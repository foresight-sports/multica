package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
)

type repositorySettings struct {
	workspacerepo.Configuration
	Revision  int64 `json:"revision"`
	CanEdit   bool  `json:"can_edit"`
	Supported bool  `json:"supported"`
}

func machineRepositoryKey(rt db.AgentRuntime) string {
	return uuidToString(rt.OwnerID) + ":" + rt.DaemonID.String
}
func (h *Handler) readRepositorySettings(ctx context.Context, scope, subject string) (repositorySettings, error) {
	var out repositorySettings
	row, e := h.Queries.GetRepositoryConfiguration(ctx, db.GetRepositoryConfigurationParams{Scope: scope, Subject: subject})
	if errors.Is(e, pgx.ErrNoRows) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	out.Revision = row.Revision
	e = json.Unmarshal(row.Config, &out.Configuration)
	if e == nil {
		e = out.Configuration.Validate(scope)
	}
	return out, e
}
func (h *Handler) RepositorySettings(w http.ResponseWriter, r *http.Request) {
	scope, subject := "workspace", workspaceIDFromURL(r, "id")
	canEdit, supported := false, true
	if raw := chi.URLParam(r, "runtimeId"); raw != "" {
		rt, _, ok := h.requireRuntimeReadAccess(w, r, obsmetrics.RuntimeLookupSourceRuntimeAPI, raw)
		if !ok {
			return
		}
		if !rt.DaemonID.Valid || !rt.OwnerID.Valid {
			writeError(w, 400, "a registered machine with an owner is required")
			return
		}
		scope, subject = "machine", machineRepositoryKey(rt)
		canEdit = requestUserID(r) == uuidToString(rt.OwnerID)
		var meta struct {
			Version int `json:"workspace_repository_version"`
		}
		_ = json.Unmarshal(rt.Metadata, &meta)
		supported = meta.Version >= 2
	} else {
		member, ok := h.requireWorkspaceMember(w, r, subject, "workspace not found")
		if !ok {
			return
		}
		canEdit = member.Role == "owner" || member.Role == "admin"
	}
	if r.Method == http.MethodPut {
		if !canEdit {
			writeError(w, 403, "only workspace administrators or the machine owner may change these settings")
			return
		}
		var req repositorySettings
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.Revision < 0 || req.Revision >= 9223372036854775807 {
			writeError(w, 400, "invalid repository settings")
			return
		}
		if e := req.Configuration.Validate(scope); e != nil {
			writeError(w, 400, e.Error())
			return
		}
		data, _ := json.Marshal(req.Configuration)
		var e error
		if req.Revision == 0 {
			_, e = h.Queries.SaveRepositoryConfiguration(r.Context(), db.SaveRepositoryConfigurationParams{Scope: scope, Subject: subject, Config: data, ExpectedRevision: 0})
		} else {
			_, e = h.Queries.UpdateRepositoryConfiguration(r.Context(), db.UpdateRepositoryConfigurationParams{Scope: scope, Subject: subject, Config: data, ExpectedRevision: req.Revision})
		}
		if errors.Is(e, pgx.ErrNoRows) {
			writeError(w, 409, "settings changed; reload before saving")
			return
		}
		if e != nil {
			writeError(w, 500, "could not save repository settings")
			return
		}
	}
	out, e := h.readRepositorySettings(r.Context(), scope, subject)
	if e != nil {
		writeError(w, 500, "could not load repository settings")
		return
	}
	out.CanEdit = canEdit
	out.Supported = supported
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, out)
}

// Workspace bindings are authoritative for every task kind and every execution profile.
func (h *Handler) applyWorkspaceRepository(ctx context.Context, resp *AgentTaskResponse, rt db.AgentRuntime, capable bool) error {
	c, e := h.readRepositorySettings(ctx, "workspace", resp.WorkspaceID)
	if e != nil {
		return e
	}
	if c.Repository == "" && c.Folder == "" {
		return workspacerepo.ErrConfigurationRequired
	}
	var meta struct {
		Version int `json:"workspace_repository_version"`
	}
	_ = json.Unmarshal(rt.Metadata, &meta)
	if meta.Version < 2 || !capable {
		return fmt.Errorf("update this machine's Multica client to support workspace repositories")
	}
	if !rt.DaemonID.Valid || !rt.OwnerID.Valid {
		return fmt.Errorf("workspace repository requires an owned machine")
	}
	machine, e := h.readRepositorySettings(ctx, "machine", machineRepositoryKey(rt))
	if e != nil {
		return e
	}
	if machine.Root == "" {
		return fmt.Errorf("set this machine's repositories folder before running workspace tasks")
	}
	localPath := workspacerepo.Join(machine.Root, c.Folder)
	ref, _ := json.Marshal(map[string]string{"daemon_id": rt.DaemonID.String, "local_path": localPath, "repositories_root": machine.Root, "expected_repository": c.Repository, "execution_mode": c.Mode, "label": c.Folder})
	// Never silently replace a project's different local directory.
	filtered := make([]ProjectResourceData, 0, len(resp.ProjectResources)+1)
	for _, res := range resp.ProjectResources {
		if res.ResourceType == "local_directory" {
			var old localDirectoryRef
			if json.Unmarshal(res.ResourceRef, &old) != nil {
				return fmt.Errorf("invalid project directory resource")
			}
			if old.DaemonID == rt.DaemonID.String {
				a := strings.TrimRight(strings.ReplaceAll(old.LocalPath, `\`, "/"), "/")
				if a != localPath {
					return fmt.Errorf("project local directory conflicts with workspace repository; remove or align it before running")
				}
				continue
			}
		}
		filtered = append(filtered, res)
	}
	resp.ProjectResources = append(filtered, ProjectResourceData{ID: resp.WorkspaceID, ResourceType: "local_directory", ResourceRef: ref, Label: c.Folder})
	resp.Repos = nil // Preparation owns cloning; task execution never chooses another directory.
	resp.WorkspaceContext += "\n\n## Workspace repository\nGitHub repository: " + c.Repository + "\nRepository directory on this machine: " + localPath + "\nUse this repository as the workspace. In isolated mode, the task's assigned Git worktree is also an authorized working directory. Other repositories under the machine's repositories folder are outside this workspace. Follow the instance filesystem instructions; this setting does not grant write access to the repositories parent folder.\n"
	// A prior session from a different configuration must never resume elsewhere.
	resp.PriorWorkDir = ""
	resp.PriorSessionID = ""
	return nil
}
