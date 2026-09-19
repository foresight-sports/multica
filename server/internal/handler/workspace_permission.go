package handler

import (
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
	var allowedEmails []string
	switch permission {
	case permissionRegisterRuntime:
		allowedEmails = h.cfg.RuntimeRegistrationAllowedEmails
	default:
		writeError(w, http.StatusForbidden, "unknown workspace permission")
		return db.Member{}, false
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
	for _, email := range allowedEmails {
		if strings.TrimSpace(email) != "" && strings.EqualFold(strings.TrimSpace(email), user.Email) {
			return member, true
		}
	}
	writeError(w, http.StatusForbidden, "you are not allowed to register runtimes; contact your administrator")
	return db.Member{}, false
}
