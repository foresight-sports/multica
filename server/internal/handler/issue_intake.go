package handler

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"net/http"
)

func (h *Handler) GetIssueIntake(w http.ResponseWriter, r *http.Request) {
	ws := workspaceIDFromURL(r, "id")
	if _, ok := h.requireWorkspaceMember(w, r, ws, "workspace not found"); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, ws, "workspace_id")
	if !ok {
		return
	}
	raw, e := h.Queries.GetIssueIntake(r.Context(), id)
	if e != nil {
		writeError(w, 500, "could not load intake settings")
		return
	}
	c, e := service.ParseIssueIntake(raw)
	if e != nil {
		writeError(w, 500, "invalid intake settings")
		return
	}
	writeJSON(w, 200, c)
}
func (h *Handler) UpdateIssueIntake(w http.ResponseWriter, r *http.Request) {
	ws := workspaceIDFromURL(r, "id")
	m, ok := h.requireWorkspaceMember(w, r, ws, "workspace not found")
	if !ok {
		return
	}
	if m.Role != "owner" && m.Role != "admin" {
		writeError(w, 403, "only workspace administrators can configure intake")
		return
	}
	id, ok := parseUUIDOrBadRequest(w, ws, "workspace_id")
	if !ok {
		return
	}
	var c service.IssueIntakeConfig
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	if json.NewDecoder(r.Body).Decode(&c) != nil || len(c.Projects) > 200 || c.Revision < 0 || c.Revision == 2147483647 {
		writeError(w, 400, "invalid intake settings")
		return
	}
	if c.Projects == nil {
		c.Projects = map[string]string{}
	}
	for project := range c.Projects {
		pid, ok := parseUUIDOrBadRequest(w, project, "project_id")
		if !ok {
			return
		}
		if _, e := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{ID: pid, WorkspaceID: id}); e != nil {
			writeError(w, 400, "project is not in this workspace")
			return
		}
	}
	squads := []string{c.DefaultSquadID}
	for _, s := range c.Projects {
		squads = append(squads, s)
	}
	for _, s := range squads {
		if s == "" {
			continue
		}
		sid, ok := parseUUIDOrBadRequest(w, s, "squad_id")
		if !ok {
			return
		}
		if e := service.ValidateIssueIntakeSquad(r.Context(), h.Queries, id, sid); e != nil {
			writeError(w, 400, "choose an active squad whose leader has a runtime and allows everyone in this workspace to invoke it")
			return
		}
	}
	revision := c.Revision
	c.Revision++
	raw, _ := json.Marshal(c)
	_, e := h.Queries.UpdateIssueIntake(r.Context(), db.UpdateIssueIntakeParams{WorkspaceID: id, Revision: revision, Config: raw})
	if errors.Is(e, pgx.ErrNoRows) {
		writeError(w, 409, "intake settings changed; reload before saving")
		return
	}
	if e != nil {
		writeError(w, 500, "could not save intake settings")
		return
	}
	writeJSON(w, 200, c)
}
