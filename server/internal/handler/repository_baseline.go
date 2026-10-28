package handler

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"net/http"
)

// Pin the first freshly fetched branch commit for an issue/config revision.
// All machines receive that same commit, even when the branch advances later.
func (h *Handler) PinTaskRepositoryBaseline(w http.ResponseWriter, r *http.Request) {
	task, ws, ok := h.requireDaemonTaskAccessWithWorkspace(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	// Preparation retains its runtime assignment while waiting for the local
	// directory lock. Provenance is saved before the start endpoint clears that
	// waiting state, so it must be accepted here as well.
	if !task.RuntimeID.Valid || (task.Status != "dispatched" && task.Status != "waiting_local_directory" && task.Status != "running") {
		writeError(w, 409, "task is not assigned for preparation")
		return
	}
	if _, ok := h.requireDaemonRuntimeAccess(w, r, uuidToString(task.RuntimeID)); !ok {
		return
	}
	var req workspacerepo.Baseline
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || !workspacerepo.ValidCommit(req.Commit) || (req.Snapshot != "" && !workspacerepo.ValidCommit(req.Snapshot)) {
		writeError(w, 400, "invalid repository baseline")
		return
	}
	cfg, err := h.readRepositorySettings(r.Context(), "workspace", ws)
	if err != nil {
		writeError(w, 500, "repository settings unavailable")
		return
	}
	if cfg.BaseBranch == "" || req.Repository != cfg.Repository || req.Branch != cfg.BaseBranch || req.Revision != cfg.Revision {
		writeError(w, 409, "repository configuration changed; prepare again")
		return
	}
	identity := uuidToString(task.IssueID)
	if !task.IssueID.Valid {
		identity = uuidToString(task.ID)
	}
	key := fmt.Sprintf("%s:%s:%d", ws, identity, cfg.Revision)
	snapshot := req.Snapshot
	req.Snapshot = ""
	raw, _ := json.Marshal(req)
	pinned, err := h.Queries.PinRepositoryBaseline(r.Context(), db.PinRepositoryBaselineParams{Subject: key, Config: raw})
	if err != nil {
		writeError(w, 500, "could not pin repository baseline")
		return
	}
	var baseline workspacerepo.Baseline
	if json.Unmarshal(pinned, &baseline) != nil {
		writeError(w, 500, "invalid saved repository baseline")
		return
	}
	if snapshot != "" {
		if req.Commit != baseline.Commit {
			writeError(w, 409, "prepared commit differs from issue baseline")
			return
		}
		baseline.Snapshot = snapshot
		raw, _ = json.Marshal(baseline)
		if err = h.Queries.PutTaskRepositorySource(r.Context(), db.PutTaskRepositorySourceParams{Subject: ws + ":" + identity + ":" + uuidToString(task.ID), Config: raw}); err != nil {
			writeError(w, 500, "could not save repository provenance")
			return
		}
	}
	writeJSON(w, 200, baseline)
}
