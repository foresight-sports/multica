package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/redact"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
)

type machineLogSnapshot struct {
	Log       string `json:"log"`
	Crash     string `json:"crash"`
	Message   string `json:"message"`
	Truncated bool   `json:"truncated"`
}

func (h *Handler) ReportMachineLogs(w http.ResponseWriter, r *http.Request) {
	rt, ok := h.requireDaemonRuntimeAccess(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	daemonID := middleware.DaemonIDFromContext(r.Context())
	if !rt.OwnerID.Valid || !rt.DaemonID.Valid || (daemonID != "" && daemonID != rt.DaemonID.String) || (daemonID == "" && requestUserID(r) != uuidToString(rt.OwnerID)) {
		writeError(w, 403, "only this machine may report its logs")
		return
	}
	var snapshot machineLogSnapshot
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024)).Decode(&snapshot) != nil || len(snapshot.Log) > 64*1024 || len(snapshot.Crash) > 16*1024 || len(snapshot.Message) > 512 {
		writeError(w, 400, "invalid log snapshot")
		return
	}
	snapshot.Log = redact.Text(snapshot.Log)
	snapshot.Crash = redact.Text(snapshot.Crash)
	snapshot.Message = redact.Text(snapshot.Message)
	raw, _ := json.Marshal(snapshot)
	if err := h.Queries.PutMachineLogs(r.Context(), db.PutMachineLogsParams{MachineKey: workspacerepo.MachineKey(rt), Snapshot: raw}); err != nil {
		writeError(w, 500, "could not store machine logs")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) GetMachineLogs(w http.ResponseWriter, r *http.Request) {
	if isMachineCredentialActor(r) {
		writeError(w, 403, "only human machine owners may read daemon logs")
		return
	}
	rt, _, ok := h.requireRuntimeReadAccess(w, r, obsmetrics.RuntimeLookupSourceRuntimeAPI, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	// Machine-wide logs may contain other workspaces' activity. Workspace admin
	// and shared runtime access do not grant access to the owner's diagnostics.
	if !rt.OwnerID.Valid || !rt.DaemonID.Valid || requestUserID(r) != uuidToString(rt.OwnerID) {
		writeError(w, 403, "only the machine owner may read daemon logs")
		return
	}
	var meta struct {
		Version int `json:"machine_logs_version"`
	}
	_ = json.Unmarshal(rt.Metadata, &meta)
	out := map[string]any{"supported": meta.Version >= 1, "log": "", "crash": "", "message": "", "truncated": false, "received_at": nil}
	row, err := h.Queries.GetMachineLogs(r.Context(), workspacerepo.MachineKey(rt))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "could not load machine logs")
		return
	}
	if err == nil {
		var snapshot machineLogSnapshot
		if json.Unmarshal(row.Snapshot, &snapshot) != nil {
			writeError(w, 500, "could not decode machine logs")
			return
		}
		out["log"] = snapshot.Log
		out["crash"] = snapshot.Crash
		out["message"] = snapshot.Message
		out["truncated"] = snapshot.Truncated
		out["received_at"] = row.ReceivedAt.Time
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, out)
}
