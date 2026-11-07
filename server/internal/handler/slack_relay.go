package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/integrations/slack"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// Relay endpoints use only the dedicated server credential, never session,
// agent, task, installer, or Jira credentials.
func (h *Handler) authenticateSlackRelay(w http.ResponseWriter, r *http.Request) (slack.RelayCredential, db.ChannelInstallation, []byte, bool) {
	c, ok := h.SlackRelayCredentials[chi.URLParam(r, "installationId")]
	if !ok {
		writeError(w, 401, "relay authentication failed")
		return c, db.ChannelInstallation{}, nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, slack.RelayMaxBytes))
	if err != nil {
		writeError(w, 413, "relay payload exceeds limit")
		return c, db.ChannelInstallation{}, nil, false
	}
	if !slack.VerifyRelaySignature(c, r, body, time.Now()) {
		writeError(w, 401, "relay authentication failed")
		return c, db.ChannelInstallation{}, nil, false
	}
	inst, err := h.Queries.GetChannelInstallation(r.Context(), db.GetChannelInstallationParams{ID: parseUUID(c.InstallationID), ChannelType: "slack"})
	info := slack.DecodePublicConfig(inst.Config)
	if err != nil || inst.Status != "active" || uuidToString(inst.WorkspaceID) != c.WorkspaceID || info.AppID != c.AppID || info.TeamID != c.TeamID || info.InboundMode != "relay" {
		writeError(w, 409, "relay installation binding or mode unavailable")
		return c, inst, nil, false
	}
	tag, err := h.DB.Exec(r.Context(), `INSERT INTO slack_relay_nonce(installation_id,nonce) VALUES($1,$2) ON CONFLICT DO NOTHING`, inst.ID, r.Header.Get("X-Relay-Nonce"))
	if err != nil {
		writeError(w, 503, "relay storage unavailable")
		return c, inst, nil, false
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 409, "relay nonce already used; retry with a new signed nonce")
		return c, inst, nil, false
	}
	_, err = h.DB.Exec(r.Context(), `INSERT INTO slack_relay_health(installation_id,workspace_id) VALUES($1,$2) ON CONFLICT(installation_id) DO UPDATE SET last_seen_at=now(),workspace_id=EXCLUDED.workspace_id`, inst.ID, inst.WorkspaceID)
	if err != nil {
		writeError(w, 503, "relay storage unavailable")
		return c, inst, nil, false
	}
	return c, inst, body, true
}

func (h *Handler) ReceiveSlackRelay(w http.ResponseWriter, r *http.Request) {
	c, inst, body, ok := h.authenticateSlackRelay(w, r)
	if !ok {
		return
	}
	event, err := slack.ParseRelayEvent(body, c, slack.DecodePublicConfig(inst.Config).BotUserID)
	if err != nil {
		writeError(w, 400, "invalid relay event identity or payload")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, 503, "relay storage unavailable")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL synchronous_commit = on`); err != nil {
		writeError(w, 503, "relay storage unavailable")
		return
	}
	// Serialize message/app_mention variants and rule edits at the ownership
	// boundary. Once recorded, an owner never changes on replay or disable.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "slack-relay:"+c.InstallationID+":"+event.ChannelID+":"+event.MessageTS); err != nil {
		writeError(w, 503, "relay storage unavailable")
		return
	}
	var id, rule pgtype.UUID
	var owner, state string
	var stored []byte
	err = tx.QueryRow(ctx, `SELECT id,rule_id,owner,state,payload FROM slack_relay_receipt WHERE installation_id=$1 AND channel_id=$2 AND message_ts=$3 FOR UPDATE`, inst.ID, event.ChannelID, event.MessageTS).Scan(&id, &rule, &owner, &state, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		enabled := false
		err = tx.QueryRow(ctx, `SELECT id,enabled FROM slack_intake_rule WHERE installation_id=$1 AND workspace_id=$2 AND channel_id=$3 FOR SHARE`, inst.ID, inst.WorkspaceID, event.ChannelID).Scan(&rule, &enabled)
		owner, state, stored = "onyx", "onyx", []byte(`{}`)
		if err == nil {
			owner, state = "multica", "pending"
			if !event.Eligible {
				state = "ignored"
			} else if !enabled {
				state = "held_disabled"
			}
			stored, err = json.Marshal(event.Message)
		} else if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
		if err == nil {
			id = dbid.NewV7()
			_, err = tx.Exec(ctx, `INSERT INTO slack_relay_receipt(id,installation_id,workspace_id,channel_id,message_ts,rule_id,owner,state,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, inst.ID, inst.WorkspaceID, event.ChannelID, event.MessageTS, rule, owner, state, stored)
		}
	} else if err == nil && owner == "multica" && event.Eligible && state != "ignored" {
		var original channel.InboundMessage
		if json.Unmarshal(stored, &original) != nil {
			writeError(w, 503, "relay receipt unreadable")
			return
		}
		enriched, changed := slack.EnrichRelayMessage(original, event.Message)
		if changed {
			stored, err = json.Marshal(enriched)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE slack_relay_receipt SET payload=$2 WHERE id=$1`, id, stored)
			}
			// Enrichment never queues another investigation/reply for the same Slack
			// message. Preserve the metadata for support-context reads.
			if err == nil && state == "delivered" {
				_, err = tx.Exec(ctx, `UPDATE slack_intake_message SET raw=$3 WHERE rule_id=$1 AND message_ts=$2`, rule, event.MessageTS, enriched.Raw)
			}
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE slack_relay_receipt SET last_received_at=now() WHERE id=$1`, id)
	}
	if err != nil || tx.Commit(ctx) != nil {
		writeError(w, 503, "relay storage unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"version": 1, "installation_id": c.InstallationID, "workspace_id": c.WorkspaceID, "app_id": c.AppID, "team_id": c.TeamID, "channel_id": event.ChannelID, "message_ts": event.MessageTS, "owner": owner, "state": state})
}

// Routing is diagnostic/preflight only. Event ownership is decided durably by
// ReceiveSlackRelay; clients must never cache a negative route decision.
func (h *Handler) SlackRelayRoutes(w http.ResponseWriter, r *http.Request) {
	c, inst, _, ok := h.authenticateSlackRelay(w, r)
	if !ok {
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT channel_id,enabled,revision FROM slack_intake_rule WHERE installation_id=$1 AND workspace_id=$2 ORDER BY channel_id`, inst.ID, inst.WorkspaceID)
	if err != nil {
		writeError(w, 503, "routing unavailable")
		return
	}
	defer rows.Close()
	routes := []map[string]any{}
	for rows.Next() {
		var ch string
		var enabled bool
		var revision int64
		if rows.Scan(&ch, &enabled, &revision) != nil {
			writeError(w, 503, "routing unavailable")
			return
		}
		routes = append(routes, map[string]any{"channel_id": ch, "enabled": enabled, "revision": revision})
	}
	if rows.Err() != nil {
		writeError(w, 503, "routing unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"version": 1, "installation_id": c.InstallationID, "workspace_id": c.WorkspaceID, "app_id": c.AppID, "team_id": c.TeamID, "mode": "relay", "routes": routes})
}

func (h *Handler) processSlackRelayReceipt(ctx context.Context, id pgtype.UUID) error {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var raw, installConfig []byte
	var rule, ws, installation pgtype.UUID
	var channelID string
	var attempts int
	err = tx.QueryRow(ctx, `SELECT payload,rule_id,workspace_id,attempts,installation_id,channel_id FROM slack_relay_receipt WHERE id=$1 AND state IN ('pending','waiting_root') AND next_attempt_at<=now() FOR UPDATE SKIP LOCKED`, id).Scan(&raw, &rule, &ws, &attempts, &installation, &channelID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	attempts++
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT r.enabled,i.config FROM slack_intake_rule r JOIN channel_installation i ON i.id=r.installation_id AND i.workspace_id=r.workspace_id WHERE r.id=$1 AND r.workspace_id=$2 AND r.installation_id=$3 AND r.channel_id=$4 AND i.status='active' AND i.config->>'inbound_mode'='relay' FOR SHARE OF r,i`, rule, ws, installation, channelID).Scan(&enabled, &installConfig)
	state, reason := "delivered", ""
	if errors.Is(err, pgx.ErrNoRows) {
		state, reason, err = "failed", "Installation or workflow unavailable; restore configuration before replay", nil
	} else if err == nil && !enabled {
		state, reason = "held_disabled", "Workflow disabled; enable and explicitly replay this receipt"
	} else if err == nil {
		var msg channel.InboundMessage
		if json.Unmarshal(raw, &msg) != nil {
			state, reason = "failed", "Invalid stored receipt; operator inspection required"
		} else {
			app, team := slack.SupportSource(msg)
			info := slack.DecodePublicConfig(installConfig)
			if info.AppID != app || info.TeamID != team || msg.Source.ChatID != channelID {
				state, reason = "failed", "Slack installation identity changed; restore the original binding before replay"
			} else {
				err = captureSlackIntakeTx(ctx, tx, msg, rule, ws)
			}
			if errors.Is(err, pgx.ErrNoRows) {
				state, reason, err = "waiting_root", "Root message has not arrived; replay the root first", nil
				if attempts >= 12 {
					state = "failed"
				}
			}
		}
	}
	if err != nil {
		return err
	}
	delay := min(900, 1<<min(attempts, 10))
	_, err = tx.Exec(ctx, `UPDATE slack_relay_receipt SET state=$2,last_error=$3,attempts=$4,next_attempt_at=now()+$5*interval '1 second',updated_at=now() WHERE id=$1`, id, state, reason, attempts, delay)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *Handler) drainSlackRelay(ctx context.Context) {
	// Ten minutes retains nonces beyond the entire +/-5 minute signature window.
	h.DB.Exec(ctx, `DELETE FROM slack_relay_nonce WHERE created_at<now()-interval '11 minutes'`)
	rows, err := h.DB.Query(ctx, `SELECT id FROM slack_relay_receipt WHERE state IN ('pending','waiting_root') AND next_attempt_at<=now() ORDER BY message_ts,created_at LIMIT 100`)
	if err != nil {
		return
	}
	ids := []pgtype.UUID{}
	for rows.Next() {
		var id pgtype.UUID
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if h.processSlackRelayReceipt(ctx, id) != nil {
			// No error objects here: DB errors may include message text or file URLs.
			h.DB.Exec(ctx, `UPDATE slack_relay_receipt SET attempts=attempts+1,state=CASE WHEN attempts>=11 THEN 'failed' ELSE state END,last_error='Intake storage failed; check database and replay',next_attempt_at=now()+interval '1 minute',updated_at=now() WHERE id=$1 AND state IN ('pending','waiting_root')`, id)
		}
	}
}
