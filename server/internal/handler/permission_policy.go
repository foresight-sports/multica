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

	"github.com/go-chi/chi/v5"
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
	return h.actionPermissionPolicy(ctx, permissionRegisterRuntime)
}

func (h *Handler) actionPermissionPolicy(ctx context.Context, action workspacePermission) (RuntimePermissionPolicyResponse, error) {
	initial := RuntimePermissionPolicyResponse{Action: string(action),
		AllowedEmails: append([]string{}, h.cfg.RuntimeRegistrationAllowedEmails...),
		Restricted:    h.cfg.RuntimeRegistrationRestricted}
	if action != permissionRegisterRuntime {
		initial.AllowedEmails = []string{}
		initial.Restricted = false
		if action == permissionCreateInstanceAgent {
			initial.AllowedEmails = append([]string{}, h.cfg.PermissionManagerEmails...)
			initial.Restricted = true
		}
	}
	if h.Queries == nil {
		return initial, fmt.Errorf("permission database unavailable")
	}
	policy, err := h.Queries.GetPermissionPolicy(ctx, string(action))
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
	action, valid := permissionAction(r)
	if !valid {
		writeError(w, 404, "unknown permission action")
		return
	}
	policy, err := h.actionPermissionPolicy(r.Context(), action)
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
	action, valid := permissionAction(r)
	if !valid {
		writeError(w, 404, "unknown permission action")
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
			Action: string(action), AllowedEmails: emails, UpdatedBy: actor})
	} else {
		saved, err = h.Queries.UpdatePermissionPolicy(r.Context(), db.UpdatePermissionPolicyParams{
			Action: string(action), AllowedEmails: emails, UpdatedBy: actor, ExpectedRevision: *req.Revision})
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

func permissionAction(r *http.Request) (workspacePermission, bool) {
	switch chi.URLParam(r, "action") {
	case "", "runtime-register":
		return permissionRegisterRuntime, true
	case "instance-agent-create":
		return permissionCreateInstanceAgent, true
	case "agent-create":
		return permissionCreateAgent, true
	case "agent-edit":
		return permissionEditAgent, true
	default:
		return "", false
	}
}

func (h *Handler) actionAllowed(ctx context.Context, email string, action workspacePermission) bool {
	policy, err := h.actionPermissionPolicy(ctx, action)
	return err == nil && (!policy.Restricted || emailAllowedForPermission(email, policy.AllowedEmails))
}

// This policy supplements resource membership/ownership, never replaces it.
func (h *Handler) requireAgentAction(w http.ResponseWriter, r *http.Request, action workspacePermission) bool {
	userID, ok := requireUserID(w, r)
	if !ok {
		return false
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err != nil {
		writeError(w, 403, "could not verify agent permission")
		return false
	}
	policy, err := h.actionPermissionPolicy(r.Context(), action)
	if err != nil {
		writeError(w, 500, "could not check agent permission")
		return false
	}
	if policy.Restricted && !emailAllowedForPermission(user.Email, policy.AllowedEmails) {
		writeError(w, 403, "you are not allowed to perform this agent action; contact your administrator")
		return false
	}
	return true
}

// The builder drafts either scope; final creation checks the selected scope again.
func (h *Handler) requireAgentCreation(w http.ResponseWriter, r *http.Request) bool {
	userID, ok := requireUserID(w, r)
	if !ok {
		return false
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err == nil && (h.actionAllowed(r.Context(), user.Email, permissionCreateAgent) || h.actionAllowed(r.Context(), user.Email, permissionCreateInstanceAgent)) {
		return true
	}
	writeError(w, 403, "you are not allowed to create agents; contact your administrator")
	return false
}
