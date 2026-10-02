package handler

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"net/http"
	"slices"
	"time"
)

func (h *Handler) DaemonWorkspaceRepositories(w http.ResponseWriter, r *http.Request) {
	rt, ok := h.requireDaemonRuntimeAccess(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	daemonID := middleware.DaemonIDFromContext(r.Context())
	if !rt.OwnerID.Valid || !rt.DaemonID.Valid || (daemonID != "" && daemonID != rt.DaemonID.String) || (daemonID == "" && requestUserID(r) != uuidToString(rt.OwnerID)) {
		writeError(w, 403, "only this machine may prepare its repositories")
		return
	}
	workspaces, err := h.Queries.ListRuntimeRepositoryWorkspaces(r.Context(), rt.ID)
	if err != nil {
		writeError(w, 500, "could not load workspace repositories")
		return
	}
	plans := make([]workspacerepo.Plan, 0, len(workspaces))
	for _, id := range workspaces {
		plan, e := workspacerepo.BuildPlan(r.Context(), h.Queries, rt, uuidToString(id))
		if e != nil {
			writeError(w, 500, "could not load workspace repository")
			return
		}
		plans = append(plans, plan)
	}
	if r.Method == http.MethodPost {
		var report workspacerepo.Readiness
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&report) != nil || !slices.Contains([]string{"preparing", "cloning", "ready", "unavailable"}, report.State) || len(report.Message) > 512 {
			writeError(w, 400, "invalid readiness report")
			return
		}
		index := slices.IndexFunc(plans, func(p workspacerepo.Plan) bool {
			return p.WorkspaceID == report.WorkspaceID && p.Fingerprint == report.Fingerprint
		})
		if index < 0 {
			writeError(w, 409, "workspace repository configuration changed or access removed")
			return
		}
		report.UpdatedAt = time.Now().UTC()
		report.Usable = report.State == "ready"
		raw, _ := json.Marshal(report)
		if err = h.Queries.PutRepositoryReadiness(r.Context(), db.PutRepositoryReadinessParams{Subject: workspacerepo.StatusKey(rt, report.WorkspaceID), Config: raw}); err != nil {
			writeError(w, 500, "could not save readiness")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	writeJSON(w, 200, plans)
}
func (h *Handler) WorkspaceRepositoryReadiness(w http.ResponseWriter, r *http.Request) {
	rt, _, ok := h.requireRuntimeReadAccess(w, r, obsmetrics.RuntimeLookupSourceRuntimeAPI, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	workspace := chi.URLParam(r, "workspaceId")
	if _, ok = h.requireWorkspaceMember(w, r, workspace, "workspace not found"); !ok {
		return
	}
	allowed, err := h.Queries.ListRuntimeRepositoryWorkspaces(r.Context(), rt.ID)
	if err != nil {
		writeError(w, 500, "could not load readiness")
		return
	}
	if !slices.ContainsFunc(allowed, func(id pgtype.UUID) bool { return uuidToString(id) == workspace }) {
		writeError(w, 403, "runtime cannot serve this workspace")
		return
	}
	status, err := workspacerepo.Status(r.Context(), h.Queries, rt, workspace)
	if err != nil {
		writeError(w, 500, "could not load readiness")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, status)
}
