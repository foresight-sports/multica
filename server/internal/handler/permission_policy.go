package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type RuntimePermissionPolicyResponse struct {
	Action        string   `json:"action"`
	AllowedEmails []string `json:"allowed_emails"`
	Restricted    bool     `json:"restricted"`
	Revision      int64    `json:"revision"`
}

// Environment values bootstrap the policy until the first explicit save.
// Persisted policy takes precedence afterward, including an empty deny-all list.
func (h *Handler) runtimePermissionPolicy(ctx context.Context) (RuntimePermissionPolicyResponse, error) {
	initial := RuntimePermissionPolicyResponse{Action: string(permissionRegisterRuntime),
		AllowedEmails: append([]string{}, h.cfg.RuntimeRegistrationAllowedEmails...),
		Restricted:    h.cfg.RuntimeRegistrationRestricted}
	if h.Queries == nil {
		return initial, fmt.Errorf("permission database unavailable")
	}
	policy, err := h.Queries.GetPermissionPolicy(ctx, string(permissionRegisterRuntime))
	if errors.Is(err, pgx.ErrNoRows) {
		return initial, nil
	}
	if err != nil {
		return RuntimePermissionPolicyResponse{}, err
	}
	return runtimePolicyResponse(policy), nil
}

func runtimePolicyResponse(policy db.PermissionPolicy) RuntimePermissionPolicyResponse {
	return RuntimePermissionPolicyResponse{Action: policy.Action, AllowedEmails: append([]string{}, policy.AllowedEmails...), Restricted: true, Revision: policy.Revision}
}

func (h *Handler) requirePermissionManager(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return pgtype.UUID{}, false
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err != nil {
		writeError(w, http.StatusForbidden, "permission management is not available for this account")
		return pgtype.UUID{}, false
	}
	if !emailAllowedForPermission(user.Email, h.cfg.PermissionManagerEmails) {
		writeError(w, http.StatusForbidden, "only configured permission managers can change access")
		return pgtype.UUID{}, false
	}
	return user.ID, true
}

func (h *Handler) GetRuntimePermissionPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requirePermissionManager(w, r); !ok {
		return
	}
	policy, err := h.runtimePermissionPolicy(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load permission access")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, policy)
}

func (h *Handler) UpdateRuntimePermissionPolicy(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requirePermissionManager(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	var req struct {
		AllowedEmails *[]string `json:"allowed_emails"`
		Revision      *int64    `json:"revision"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.AllowedEmails == nil || req.Revision == nil || *req.Revision < 0 {
		writeError(w, http.StatusBadRequest, "allowed_emails and revision are required")
		return
	}
	emails, err := normalizePermissionEmails(*req.AllowedEmails)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var saved db.PermissionPolicy
	if *req.Revision == 0 {
		saved, err = h.Queries.CreatePermissionPolicy(r.Context(), db.CreatePermissionPolicyParams{
			Action: string(permissionRegisterRuntime), AllowedEmails: emails, UpdatedBy: actor})
	} else {
		saved, err = h.Queries.UpdatePermissionPolicy(r.Context(), db.UpdatePermissionPolicyParams{
			Action: string(permissionRegisterRuntime), AllowedEmails: emails, UpdatedBy: actor, ExpectedRevision: *req.Revision})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "permission access changed; reload the latest list before saving")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save permission access")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, runtimePolicyResponse(saved))
}

var permissionEmailPattern = regexp.MustCompile(`^[a-z0-9_'+.\-]*[a-z0-9_+\-]@([a-z0-9][a-z0-9\-]*\.)+[a-z]{2,}$`)

func normalizePermissionEmails(input []string) ([]string, error) {
	if len(input) > 1000 {
		return nil, fmt.Errorf("at most 1000 email addresses are allowed")
	}
	emails := make([]string, 0, len(input))
	for _, value := range input {
		email := strings.ToLower(strings.TrimSpace(value))
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email || len(email) > 254 || !permissionEmailPattern.MatchString(email) || strings.HasPrefix(email, ".") || strings.Contains(email, "..") {
			return nil, fmt.Errorf("enter valid email addresses without display names")
		}
		emails = append(emails, email)
	}
	slices.Sort(emails)
	return slices.Compact(emails), nil
}
