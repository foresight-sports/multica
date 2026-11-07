package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/integrations/slack"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func (h *Handler) SlackRelayPreflight(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.slackRelayAdmin(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "installationId"), "installation id")
	if !ok {
		return
	}
	inst, err := h.Queries.GetChannelInstallation(r.Context(), db.GetChannelInstallationParams{ID: id, ChannelType: "slack"})
	if err != nil || uuidToString(inst.WorkspaceID) != ws || inst.Status != "active" {
		writeError(w, 404, "active Slack installation not found")
		return
	}
	if h.SlackSupport == nil {
		writeError(w, 503, "Slack support client unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err = h.SlackSupport.VerifyIdentity(ctx, inst.Config); err != nil {
		writeError(w, 409, err.Error())
		return
	}
	rows, err := h.DB.Query(ctx, `SELECT channel_id,enabled FROM slack_intake_rule WHERE installation_id=$1 AND workspace_id=$2 ORDER BY channel_id LIMIT 201`, inst.ID, inst.WorkspaceID)
	if err != nil {
		writeError(w, 503, "workflow configuration unavailable")
		return
	}
	type route struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
		Name    string `json:"name"`
		Private bool   `json:"private"`
	}
	routes := []route{}
	for rows.Next() {
		var ch route
		if rows.Scan(&ch.ID, &ch.Enabled) != nil {
			rows.Close()
			writeError(w, 503, "workflow configuration unavailable")
			return
		}
		routes = append(routes, ch)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(routes) > 200 {
		writeError(w, 409, "preflight requires at most 200 configured channels")
		return
	}
	for i := range routes {
		routes[i].Name, routes[i].Private, err = h.SlackSupport.VerifyChannel(ctx, inst.Config, routes[i].ID)
		if err != nil {
			writeError(w, 409, err.Error())
			return
		}
	}
	info := slack.DecodePublicConfig(inst.Config)
	writeJSON(w, 200, map[string]any{"installation_id": uuidToString(inst.ID), "workspace_id": ws, "app_id": info.AppID, "team_id": info.TeamID, "bot_user_id": info.BotUserID, "mode": info.InboundMode, "channels": routes})
}

func (h *Handler) slackRelayAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	ws := workspaceIDFromURL(r, "id")
	member, ok := h.requireWorkspaceMember(w, r, ws, "workspace not found")
	if !ok {
		return "", false
	}
	if r.Header.Get("X-Actor-Source") != "" || (member.Role != "owner" && member.Role != "admin") {
		writeError(w, 403, "human workspace administrator required")
		return "", false
	}
	return ws, true
}

func (h *Handler) SetSlackInboundMode(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.slackRelayAdmin(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "installationId"), "installation id")
	if !ok {
		return
	}
	var body struct {
		Mode         string `json:"mode"`
		ExpectedMode string `json:"expected_mode"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1000)).Decode(&body) != nil || (body.Mode != "relay" && body.Mode != "socket_mode") {
		writeError(w, 400, "choose relay or socket_mode")
		return
	}
	inst, err := h.Queries.GetChannelInstallation(r.Context(), db.GetChannelInstallationParams{ID: id, ChannelType: "slack"})
	if err != nil || uuidToString(inst.WorkspaceID) != ws || inst.Status != "active" {
		writeError(w, 404, "active Slack installation not found")
		return
	}
	info := slack.DecodePublicConfig(inst.Config)
	if body.Mode == "relay" {
		c, exists := h.SlackRelayCredentials[uuidToString(id)]
		if !exists || c.WorkspaceID != ws || c.AppID != info.AppID || c.TeamID != info.TeamID {
			writeError(w, 409, "configure the server relay credential with this installation, workspace, app and team first")
			return
		}
	}
	// CAS and pending-work check are one write. Returning to Socket Mode is an
	// explicit operator action after draining Onyx and Multica, never a fallback.
	tag, err := h.DB.Exec(r.Context(), `UPDATE channel_installation SET config=jsonb_set(config,'{inbound_mode}',to_jsonb($3::text)),updated_at=now() WHERE id=$1 AND workspace_id=$2 AND COALESCE(config->>'inbound_mode','socket_mode')=$4 AND ($3='relay' OR NOT EXISTS(SELECT 1 FROM slack_relay_receipt WHERE installation_id=$1 AND state IN ('pending','waiting_root','held_disabled','failed')))`, id, inst.WorkspaceID, body.Mode, body.ExpectedMode)
	if err != nil || tag.RowsAffected() != 1 {
		writeError(w, 409, "mode changed or relay receipts still need attention; reload and drain before switching")
		return
	}
	writeJSON(w, 200, map[string]string{"mode": body.Mode})
}

func (h *Handler) SlackRelayStatus(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.slackRelayAdmin(w, r)
	if !ok {
		return
	}
	// Only metadata. Never return payload, raw files, sender IDs or credentials.
	rows, err := h.DB.Query(r.Context(), `SELECT jsonb_build_object('id',i.id,'app_id',i.config->>'app_id','team_id',i.config->>'team_id','mode',COALESCE(i.config->>'inbound_mode','socket_mode'),'active',i.status='active','last_seen_at',h.last_seen_at,'contact_recent',COALESCE(h.last_seen_at>now()-interval '90 seconds',false)) FROM channel_installation i LEFT JOIN slack_relay_health h ON h.installation_id=i.id AND h.workspace_id=i.workspace_id WHERE i.workspace_id=$1 AND i.channel_type='slack' ORDER BY i.id`, parseUUID(ws))
	if err != nil {
		writeError(w, 503, "relay status unavailable")
		return
	}
	installations := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil {
			rows.Close()
			writeError(w, 503, "relay status unavailable")
			return
		}
		var item map[string]any
		if json.Unmarshal(raw, &item) != nil {
			rows.Close()
			writeError(w, 503, "relay status unavailable")
			return
		}
		id, _ := item["id"].(string)
		c, exists := h.SlackRelayCredentials[id]
		item["credential_configured"] = exists && c.WorkspaceID == ws && c.AppID == item["app_id"] && c.TeamID == item["team_id"]
		installations = append(installations, item)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		writeError(w, 503, "relay status unavailable")
		return
	}
	counts := map[string]int{}
	rows, err = h.DB.Query(r.Context(), `SELECT state,count(*) FROM slack_relay_receipt WHERE workspace_id=$1 GROUP BY state`, parseUUID(ws))
	if err != nil {
		writeError(w, 503, "relay status unavailable")
		return
	}
	for rows.Next() {
		var state string
		var n int
		if rows.Scan(&state, &n) != nil {
			rows.Close()
			writeError(w, 503, "relay status unavailable")
			return
		}
		counts[state] = n
	}
	rowErr = rows.Err()
	rows.Close()
	if rowErr != nil {
		writeError(w, 503, "relay status unavailable")
		return
	}
	rows, err = h.DB.Query(r.Context(), `SELECT jsonb_build_object('id',id,'installation_id',installation_id,'channel_id',channel_id,'message_ts',message_ts,'owner',owner,'state',state,'attempts',attempts,'last_error',last_error,'received_at',last_received_at,'updated_at',updated_at) FROM slack_relay_receipt WHERE workspace_id=$1 ORDER BY last_received_at DESC LIMIT 50`, parseUUID(ws))
	if err != nil {
		writeError(w, 503, "relay status unavailable")
		return
	}
	defer rows.Close()
	recent := []json.RawMessage{}
	for rows.Next() {
		var raw json.RawMessage
		if rows.Scan(&raw) != nil {
			writeError(w, 503, "relay status unavailable")
			return
		}
		recent = append(recent, raw)
	}
	if rows.Err() != nil {
		writeError(w, 503, "relay status unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"installations": installations, "counts": counts, "recent": recent})
}

func (h *Handler) ReplaySlackRelay(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.slackRelayAdmin(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "receiptId"), "receipt id")
	if !ok {
		return
	}
	tag, err := h.DB.Exec(r.Context(), `UPDATE slack_relay_receipt s SET state='pending',attempts=0,next_attempt_at=now(),last_error='',updated_at=now() FROM slack_intake_rule r JOIN channel_installation i ON i.id=r.installation_id AND i.workspace_id=r.workspace_id WHERE s.id=$1 AND s.workspace_id=$2 AND s.rule_id=r.id AND r.workspace_id=s.workspace_id AND r.enabled AND i.status='active' AND i.config->>'inbound_mode'='relay' AND s.owner='multica' AND s.state IN ('failed','held_disabled','waiting_root')`, id, parseUUID(ws))
	if err != nil || tag.RowsAffected() != 1 {
		writeError(w, 409, "enable the original workflow and relay installation before replay; delivered receipts cannot be replayed")
		return
	}
	writeJSON(w, 200, map[string]string{"state": "pending"})
}
