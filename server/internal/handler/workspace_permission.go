package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// workspacePermission names protected actions. Future lockdowns can assign
// their own user allowlists here while sharing identity and membership checks.
type workspacePermission string

const permissionRegisterRuntime workspacePermission = "runtime.register"

func (h *Handler) requireWorkspacePermission(w http.ResponseWriter, r *http.Request, workspaceID string, permission workspacePermission) (db.Member, bool) {
	if permission != permissionRegisterRuntime {
		writeError(w, http.StatusForbidden, "unknown workspace permission")
		return db.Member{}, false
	}
	policy, err := h.runtimePermissionPolicy(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check runtime registration access")
		return db.Member{}, false
	}
	if !policy.Restricted {
		return db.Member{}, true
	}
	// Machine credentials do not prove a user's identity. In particular, do
	// not trust X-User-ID supplied by the client on the daemon-token path.
	if middleware.DaemonWorkspaceIDFromContext(r.Context()) != "" {
		writeError(w, http.StatusForbidden, "runtime registration requires an allowed user's credentials")
		return db.Member{}, false
	}
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return db.Member{}, false
	}
	user, err := h.Queries.GetUser(r.Context(), member.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check workspace permission")
		return db.Member{}, false
	}
	// Use the persisted account email, never a request header or body field.
	// Empty lists deny everyone; roles never bypass an explicit user policy.
	if emailAllowedForPermission(user.Email, policy.AllowedEmails) {
		return member, true
	}
	writeError(w, http.StatusForbidden, "you are not allowed to register runtimes; contact your administrator")
	return db.Member{}, false
}

// runtimeRegistrationAllowed reports the account-level policy. Registration
// also checks membership in the requested workspace before writing anything.
func (h *Handler) runtimeRegistrationAllowed(ctx context.Context, email string) bool {
	policy, err := h.runtimePermissionPolicy(ctx)
	return err == nil && (!policy.Restricted || emailAllowedForPermission(email, policy.AllowedEmails))
}

func emailAllowedForPermission(email string, allowedEmails []string) bool {
	for _, allowed := range allowedEmails {
		if strings.TrimSpace(allowed) != "" && strings.EqualFold(strings.TrimSpace(allowed), email) {
			return true
		}
	}
	return false
}
