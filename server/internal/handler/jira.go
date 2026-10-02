package handler

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
	"net/http"
	"regexp"
	"strings"
)

var jiraKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[1-9][0-9]*$`)

type jiraSettings struct {
	PropertyID string `json:"property_id"`
	Required   bool   `json:"required"`
	Revision   int64  `json:"revision"`
	CanEdit    bool   `json:"can_edit"`
}

func (h *Handler) JiraSettings(w http.ResponseWriter, r *http.Request) {
	ws := workspaceIDFromURL(r, "id")
	member, ok := h.requireWorkspaceMember(w, r, ws, "workspace not found")
	if !ok {
		return
	}
	canEdit := member.Role == "owner" || member.Role == "admin"
	if r.Method == http.MethodPut {
		if !canEdit {
			writeError(w, 403, "only workspace administrators may change the JIRA requirement")
			return
		}
		var req jiraSettings
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || req.Revision < 1 {
			writeError(w, 400, "valid revision is required")
			return
		}
		_, err := h.Queries.SaveJiraRequirement(r.Context(), db.SaveJiraRequirementParams{WorkspaceID: parseUUID(ws), Required: req.Required, Revision: req.Revision})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 409, "settings changed; reload before saving")
			return
		}
		if err != nil {
			writeError(w, 500, "could not save JIRA settings")
			return
		}
	}
	prop, err := h.Queries.GetJiraProperty(r.Context(), parseUUID(ws))
	if err != nil {
		writeError(w, 500, "could not load JIRA settings")
		return
	}
	var out jiraSettings
	if json.Unmarshal(prop.Config, &out) != nil {
		writeError(w, 500, "invalid JIRA settings")
		return
	}
	out.PropertyID = uuidToString(prop.ID)
	out.CanEdit = canEdit
	if r.Method == http.MethodPut {
		h.publish(protocol.EventPropertyUpdated, ws, "member", uuidToString(member.UserID), map[string]any{"property": propertyToResponse(prop, 0)})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, out)
}

func normalizeJiraKey(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) > 128 || !jiraKeyPattern.MatchString(value) {
		return "", errors.New("JIRA ticket ID must be a key such as FS-123")
	}
	return value, nil
}
