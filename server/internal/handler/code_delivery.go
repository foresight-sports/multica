package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"net/http"
)

func (h *Handler) readCodeDelivery(ctx context.Context, ws string) (workspacerepo.DeliverySettings, error) {
	c := workspacerepo.DeliverySettings{Projects: map[string]workspacerepo.Delivery{}}
	row, e := h.Queries.GetRepositoryConfiguration(ctx, db.GetRepositoryConfigurationParams{Scope: "code_delivery", Subject: ws})
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return c, e
	}
	if e == nil {
		if e = json.Unmarshal(row.Config, &c); e != nil {
			return c, e
		}
		c.Revision = row.Revision
	}
	if c.Projects == nil {
		c.Projects = map[string]workspacerepo.Delivery{}
	}
	if e = c.Default.Validate(); e != nil {
		return c, e
	}
	for id, p := range c.Projects {
		if e = p.Validate(); e != nil {
			return c, e
		}
		c.Projects[id] = p
	}
	return c, nil
}

func (h *Handler) CodeDeliverySettings(w http.ResponseWriter, r *http.Request) {
	ws := workspaceIDFromURL(r, "id")
	m, ok := h.requireWorkspaceMember(w, r, ws, "workspace not found")
	if !ok {
		return
	}
	canEdit := m.Role == "owner" || m.Role == "admin"
	if r.Method == http.MethodPut {
		if !canEdit {
			writeError(w, 403, "only workspace administrators can configure code delivery")
			return
		}
		var c workspacerepo.DeliverySettings
		r.Body = http.MaxBytesReader(w, r.Body, 65536)
		if json.NewDecoder(r.Body).Decode(&c) != nil || c.Revision < 0 || c.Revision >= 9223372036854775807 || len(c.Projects) > 200 {
			writeError(w, 400, "invalid delivery settings")
			return
		}
		if e := c.Default.Validate(); e != nil {
			writeError(w, 400, e.Error())
			return
		}
		for id, p := range c.Projects {
			pid, ok := parseUUIDOrBadRequest(w, id, "project_id")
			if !ok {
				return
			}
			if _, e := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{ID: pid, WorkspaceID: parseUUID(ws)}); e != nil {
				writeError(w, 400, "project is not in this workspace")
				return
			}
			if e := p.Validate(); e != nil {
				writeError(w, 400, e.Error())
				return
			}
			c.Projects[id] = p
		}
		revision := c.Revision
		c.Revision = 0
		c.CanEdit = false
		c.AutomationAvailable = false
		c.AutomationErrors = nil
		raw, _ := json.Marshal(c)
		var e error
		if revision == 0 {
			_, e = h.Queries.SaveRepositoryConfiguration(r.Context(), db.SaveRepositoryConfigurationParams{Scope: "code_delivery", Subject: ws, Config: raw, ExpectedRevision: 0})
		} else {
			_, e = h.Queries.UpdateRepositoryConfiguration(r.Context(), db.UpdateRepositoryConfigurationParams{Scope: "code_delivery", Subject: ws, Config: raw, ExpectedRevision: revision})
		}
		if errors.Is(e, pgx.ErrNoRows) {
			writeError(w, 409, "delivery settings changed; reload before saving")
			return
		}
		if e != nil {
			writeError(w, 500, "could not save delivery settings")
			return
		}
	}
	c, e := h.readCodeDelivery(r.Context(), ws)
	if e != nil {
		writeError(w, 500, "could not load delivery settings")
		return
	}
	c.CanEdit = canEdit
	c.AutomationAvailable = h.codeReviewEnabled()
	if c.AutomationAvailable {
		installations, err := h.Queries.ListGitHubInstallationsByWorkspace(r.Context(), parseUUID(ws))
		c.AutomationAvailable = err == nil && len(installations) > 0
	}
	c.AutomationErrors = []string{}
	if h.DB != nil {
		rows, err := h.DB.Query(r.Context(), `SELECT p.repo_owner || '/' || p.repo_name || ' #' || p.pr_number::text || ': ' || (c.config->>'error') FROM repository_configuration c JOIN github_pull_request p ON c.subject=p.id::text WHERE c.scope='code_review_scan' AND p.workspace_id=$1 AND p.state='open' AND c.config->>'error' <> '' ORDER BY c.config->>'checked_at' DESC LIMIT 10`, parseUUID(ws))
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var message string
				if rows.Scan(&message) == nil {
					c.AutomationErrors = append(c.AutomationErrors, message)
				}
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, c)
}
