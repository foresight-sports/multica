package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/pkg/protocol"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
)

// Provider authentication is a machine-owner operation, never an agent tool.
// Persist only request identity and lifecycle; CLI output and tokens stay local.
func (h *Handler) ProviderAuth(w http.ResponseWriter, r *http.Request) {
	if isMachineCredentialActor(r) {
		writeError(w, 403, "only human machine owners may configure authentication")
		return
	}
	rt, _, ok := h.requireRuntimeReadAccess(w, r, obsmetrics.RuntimeLookupSourceRuntimeAPI, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	if !rt.OwnerID.Valid || !rt.DaemonID.Valid || requestUserID(r) != uuidToString(rt.OwnerID) {
		writeError(w, 403, "only the machine owner may configure authentication")
		return
	}
	key := workspacerepo.MachineKey(rt)
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		var id, provider, status string
		err := h.DB.QueryRow(r.Context(), `SELECT id::text, provider, CASE WHEN expires_at < now() AND status IN ('pending','running') THEN 'failed' ELSE status END FROM provider_auth_requests WHERE machine_key=$1`, key).Scan(&id, &provider, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, 200, map[string]string{"status": "idle"})
			return
		}
		if err != nil {
			writeError(w, 500, "could not load authentication setup")
			return
		}
		writeJSON(w, 200, map[string]string{"id": id, "provider": provider, "status": status})
		return
	}
	var meta struct {
		Version int    `json:"provider_auth_version"`
		OS      string `json:"os"`
	}
	_ = json.Unmarshal(rt.Metadata, &meta)
	if meta.Version != 1 || meta.OS != "windows" {
		writeError(w, 409, "update the Windows daemon to enable authentication setup")
		return
	}
	var req struct {
		Provider string `json:"provider"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req) != nil || (req.Provider != "claude" && req.Provider != "codex") {
		writeError(w, 400, "provider must be claude or codex")
		return
	}
	id := randomID()
	tag, err := h.DB.Exec(r.Context(), `INSERT INTO provider_auth_requests (machine_key,id,runtime_id,provider,status) VALUES ($1,$2,$3,$4,'pending') ON CONFLICT (machine_key) DO UPDATE SET id=EXCLUDED.id,runtime_id=EXCLUDED.runtime_id,provider=EXCLUDED.provider,status='pending',created_at=now(),expires_at=now()+interval '2 minutes' WHERE provider_auth_requests.status IN ('completed','failed') OR provider_auth_requests.expires_at < now()`, key, id, rt.ID, req.Provider)
	if err != nil {
		writeError(w, 500, "could not start authentication setup")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 409, "authentication setup is already pending on this machine")
		return
	}
	writeJSON(w, 202, map[string]string{"id": id, "provider": req.Provider, "status": "pending"})
}

func (h *Handler) claimProviderAuth(ctx context.Context, runtimeID, userID, daemonID string) *protocol.DaemonHeartbeatProviderAuth {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var action protocol.DaemonHeartbeatProviderAuth
	err := h.DB.QueryRow(ctx, `UPDATE provider_auth_requests q SET status='running', expires_at=now()+interval '15 minutes' FROM agent_runtime r WHERE q.runtime_id=$1 AND r.id=q.runtime_id AND q.machine_key=r.owner_id::text || ':' || r.daemon_id AND (($3<>'' AND r.daemon_id=$3) OR ($3='' AND r.owner_id::text=$2)) AND q.status='pending' AND q.expires_at > now() RETURNING q.id::text,q.provider`, runtimeID, userID, daemonID).Scan(&action.ID, &action.Provider)
	if err != nil {
		return nil
	}
	return &action
}

func (h *Handler) ReportProviderAuth(w http.ResponseWriter, r *http.Request) {
	rt, ok := h.requireDaemonRuntimeAccess(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	daemonID := middleware.DaemonIDFromContext(r.Context())
	if !rt.OwnerID.Valid || !rt.DaemonID.Valid || (daemonID != "" && daemonID != rt.DaemonID.String) || (daemonID == "" && requestUserID(r) != uuidToString(rt.OwnerID)) {
		writeError(w, 403, "only this machine may report authentication setup")
		return
	}
	var req struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req) != nil || (req.Status != "completed" && req.Status != "failed") {
		writeError(w, 400, "invalid authentication result")
		return
	}
	id, valid := parseUUIDOrBadRequest(w, req.ID, "id")
	if !valid {
		return
	}
	_, err := h.DB.Exec(r.Context(), `UPDATE provider_auth_requests SET status=$1 WHERE machine_key=$2 AND runtime_id=$3 AND id=$4 AND status='running' AND expires_at>now()`, req.Status, workspacerepo.MachineKey(rt), rt.ID, id)
	if err != nil {
		writeError(w, 500, "could not save authentication result")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
