package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/integrations/slack"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type slackIntakeRule struct {
	DisableOnly       bool   `json:"disable_only,omitempty"`
	ComposerID        string `json:"composer_id"`
	Audience          string `json:"audience"`
	StyleInstructions string `json:"style_instructions"`
	ID                string `json:"id"`
	WorkspaceID       string `json:"workspace_id"`
	InstallationID    string `json:"installation_id"`
	ChannelID         string `json:"channel_id"`
	ChannelName       string `json:"channel_name"`
	Workflow          string `json:"workflow"`
	ServiceUserID     string `json:"service_user_id"`
	CoordinatorID     string `json:"coordinator_id"`
	AnalystID         string `json:"analyst_id"`
	EscalationUserID  string `json:"escalation_user_id"`
	Enabled           bool   `json:"enabled"`
	Revision          int64  `json:"revision"`
	TimeoutMinutes    int    `json:"timeout_minutes"`
}

var slackChannelID = regexp.MustCompile(`^[CG][A-Z0-9]{5,}$`)
var slackPersonID = regexp.MustCompile(`^[UW][A-Z0-9]{5,}$`)

func (h *Handler) SlackIntakeRules(w http.ResponseWriter, r *http.Request) {
	ws := workspaceIDFromURL(r, "id")
	member, ok := h.requireWorkspaceMember(w, r, ws, "workspace not found")
	if !ok {
		return
	}
	if member.Role != "owner" && member.Role != "admin" {
		writeError(w, 403, "workspace administrator required")
		return
	}
	if r.Method == http.MethodPut {
		if r.Header.Get("X-Actor-Source") != "" {
			writeError(w, 403, "human administrator required")
			return
		}
		var v slackIntakeRule
		r.Body = http.MaxBytesReader(w, r.Body, 8000)
		if json.NewDecoder(r.Body).Decode(&v) != nil || v.Workflow != "support_diagnosis" || v.Revision < 0 || v.TimeoutMinutes < 5 || v.TimeoutMinutes > 1440 || !slackChannelID.MatchString(v.ChannelID) || !slackPersonID.MatchString(v.EscalationUserID) {
			writeError(w, 400, "provide a support workflow, Slack channel/member IDs and a timeout of 5–1440 minutes")
			return
		}
		// Disabling an existing rule must work even when Slack is unavailable.
		if v.ID != "" && !v.Enabled && v.DisableOnly {
			ruleID, valid := parseUUIDOrBadRequest(w, v.ID, "id")
			if !valid {
				return
			}
			tag, err := h.DB.Exec(r.Context(), `UPDATE slack_intake_rule SET enabled=false,revision=revision+1,updated_at=now() WHERE id=$1 AND workspace_id=$2 AND revision=$3`, ruleID, parseUUID(ws), v.Revision)
			if err != nil || tag.RowsAffected() != 1 {
				writeError(w, 409, "workflow changed; reload before disabling")
				return
			}
			r.Method = http.MethodGet
			h.SlackIntakeRules(w, r)
			return
		}
		instID, ok := parseUUIDOrBadRequest(w, v.InstallationID, "installation_id")
		if !ok {
			return
		}
		coord, ok := parseUUIDOrBadRequest(w, v.CoordinatorID, "coordinator_id")
		if !ok {
			return
		}
		analyst, ok := parseUUIDOrBadRequest(w, v.AnalystID, "analyst_id")
		if !ok {
			return
		}
		if coord == analyst {
			writeError(w, 400, "coordinator and support analyst must be different agents")
			return
		}
		var composer pgtype.UUID
		agentIDs := []pgtype.UUID{coord, analyst}
		if v.ComposerID != "" {
			var valid bool
			composer, valid = parseUUIDOrBadRequest(w, v.ComposerID, "composer_id")
			if !valid {
				return
			}
			if composer == coord || composer == analyst {
				writeError(w, 400, "choose a separate communications agent")
				return
			}
			agentIDs = append(agentIDs, composer)
		}
		if v.Audience == "" {
			v.Audience = "customer_support"
		}
		if (v.Audience != "customer_support" && v.Audience != "sales" && v.Audience != "internal") || len(v.StyleInstructions) > 3000 {
			writeError(w, 400, "choose a supported audience and keep style instructions under 3000 characters")
			return
		}
		inst, e := h.Queries.GetChannelInstallation(r.Context(), db.GetChannelInstallationParams{ID: instID, ChannelType: "slack"})
		if e != nil || inst.WorkspaceID != parseUUID(ws) || inst.ChannelType != "slack" || inst.Status != "active" {
			writeError(w, 404, "active Slack installation not found")
			return
		}
		if h.SlackSupport == nil {
			writeError(w, 409, "Connect the Slack app first")
			return
		}
		name, e := h.SlackSupport.Validate(r.Context(), inst.Config, v.ChannelID, v.EscalationUserID)
		if e != nil {
			writeError(w, 400, e.Error())
			return
		}
		for _, id := range agentIDs {
			a, e := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{ID: id, WorkspaceID: parseUUID(ws)})
			if e != nil || a.ArchivedAt.Valid {
				writeError(w, 400, "select active workspace agents")
				return
			}
		}
		tx, e := h.TxStarter.Begin(r.Context())
		if e != nil {
			writeError(w, 500, "could not save workflow")
			return
		}
		defer tx.Rollback(r.Context())
		// One explicit, non-login automation identity per workspace. No personal
		// credentials or memberships are copied to it.
		if _, e = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtext($1))`, "slack-service:"+ws); e != nil {
			writeError(w, 500, "could not lock workflow")
			return
		}
		var serviceID pgtype.UUID
		e = tx.QueryRow(r.Context(), `SELECT user_id FROM automation_account WHERE workspace_id=$1`, parseUUID(ws)).Scan(&serviceID)
		if errors.Is(e, pgx.ErrNoRows) {
			serviceID = dbid.NewV7()
			_, e = tx.Exec(r.Context(), `INSERT INTO "user"(id,name,email) VALUES($1,'FS Caddie',$2)`, serviceID, uuidToString(serviceID)+"@automation.multica.invalid")
			if e == nil {
				_, e = tx.Exec(r.Context(), `INSERT INTO automation_account(user_id,workspace_id,created_by) VALUES($1,$2,$3)`, serviceID, parseUUID(ws), member.UserID)
			}
			if e == nil {
				_, e = tx.Exec(r.Context(), `INSERT INTO member(workspace_id,user_id,role) VALUES($1,$2,'member')`, parseUUID(ws), serviceID)
			}
		}
		if e != nil {
			writeError(w, 500, "could not provision FS Caddie")
			return
		}
		// Do not restore revoked membership or silently add per-agent grants.
		var access bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM member WHERE workspace_id=$1 AND user_id=$2)`, parseUUID(ws), serviceID).Scan(&access)
		if e != nil || !access {
			writeError(w, 409, "FS Caddie needs workspace membership")
			return
		}
		for _, id := range agentIDs {
			a, _ := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{ID: id, WorkspaceID: parseUUID(ws)})
			if !h.canInvokeAgent(r.Context(), a, "system", "", uuidToString(serviceID), ws) {
				writeError(w, 409, "FS Caddie needs access to all selected agents")
				return
			}
		}
		id := dbid.NewV7()
		e = tx.QueryRow(r.Context(), `INSERT INTO slack_intake_rule(id,workspace_id,installation_id,channel_id,channel_name,service_user_id,coordinator_id,analyst_id,escalation_user_id,enabled,timeout_minutes,composer_id,audience,style_instructions)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$13,$14,$15 WHERE $12::bigint=0 OR EXISTS(SELECT 1 FROM slack_intake_rule WHERE installation_id=$3 AND channel_id=$4 AND workspace_id=$2)
 ON CONFLICT(installation_id,channel_id) DO UPDATE SET composer_id=EXCLUDED.composer_id,audience=EXCLUDED.audience,style_instructions=EXCLUDED.style_instructions,channel_name=EXCLUDED.channel_name,coordinator_id=EXCLUDED.coordinator_id,analyst_id=EXCLUDED.analyst_id,escalation_user_id=EXCLUDED.escalation_user_id,enabled=EXCLUDED.enabled,timeout_minutes=EXCLUDED.timeout_minutes,revision=slack_intake_rule.revision+1,updated_at=now()
 WHERE slack_intake_rule.revision=$12 AND slack_intake_rule.workspace_id=$2 RETURNING id`, id, parseUUID(ws), instID, v.ChannelID, name, serviceID, coord, analyst, v.EscalationUserID, v.Enabled, v.TimeoutMinutes, v.Revision, composer, v.Audience, v.StyleInstructions).Scan(&id)
		if e != nil {
			writeError(w, 409, "workflow changed; reload before saving")
			return
		}
		if e = tx.Commit(r.Context()); e != nil {
			writeError(w, 500, "could not save workflow")
			return
		}
	}
	rows, e := h.DB.Query(r.Context(), `SELECT row_to_json(s) FROM slack_intake_rule s WHERE workspace_id=$1 ORDER BY channel_name`, parseUUID(ws))
	if e != nil {
		writeError(w, 500, "could not load workflows")
		return
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b json.RawMessage
		if rows.Scan(&b) != nil {
			writeError(w, 500, "could not read workflows")
			return
		}
		out = append(out, b)
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not read workflows")
		return
	}
	writeJSON(w, 200, out)
}

// Capture is called before acknowledging the Socket Mode envelope. Only the
// allowlisted channel is diverted; all other integrations remain unchanged.
func (h *Handler) CaptureSlackIntake(ctx context.Context, msg channel.InboundMessage) (bool, error) {
	if msg.Source.ChatType != channel.ChatTypeGroup {
		return false, nil
	}
	app, team := slack.SupportSource(msg)
	if app == "" || team == "" {
		return false, nil
	}
	var ruleID, ws pgtype.UUID
	var enabled bool
	e := h.DB.QueryRow(ctx, `SELECT r.id,r.workspace_id,r.enabled FROM slack_intake_rule r JOIN channel_installation i ON i.id=r.installation_id AND i.workspace_id=r.workspace_id WHERE i.channel_type='slack' AND i.status='active' AND i.config->>'app_id'=$1 AND i.config->>'team_id'=$2 AND r.channel_id=$3`, app, team, msg.Source.ChatID).Scan(&ruleID, &ws, &enabled)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if !enabled {
		return true, nil
	}
	if len(msg.Text) > 200000 || len(msg.Raw) > 2000000 {
		return true, fmt.Errorf("Slack intake message exceeds size limit")
	}
	root := msg.Source.ThreadID
	if root == "" {
		root = msg.MessageID
	}
	tx, e := h.TxStarter.Begin(ctx)
	if e != nil {
		return true, e
	}
	defer tx.Rollback(ctx)
	var thread pgtype.UUID
	if root == msg.MessageID {
		e = tx.QueryRow(ctx, `INSERT INTO slack_intake_thread(id,rule_id,workspace_id,root_ts) VALUES($1,$2,$3,$4) ON CONFLICT(rule_id,root_ts) DO UPDATE SET rule_id=EXCLUDED.rule_id RETURNING id`, dbid.NewV7(), ruleID, ws, root).Scan(&thread)
	} else {
		e = tx.QueryRow(ctx, `SELECT id FROM slack_intake_thread WHERE rule_id=$1 AND root_ts=$2 FOR UPDATE`, ruleID, root).Scan(&thread)
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return true, nil
	}
	if e != nil {
		return true, e
	}
	var state string
	if e = tx.QueryRow(ctx, `SELECT state FROM slack_intake_thread WHERE id=$1`, thread).Scan(&state); e != nil {
		return true, e
	}
	if state == "deleted" {
		return true, nil
	}
	tag, e := tx.Exec(ctx, `INSERT INTO slack_intake_message(id,thread_id,rule_id,message_ts,sender_id,body,raw) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(rule_id,message_ts) DO NOTHING`, dbid.NewV7(), thread, ruleID, msg.MessageID, msg.Source.SenderID, strings.TrimSpace(msg.Text), msg.Raw)
	if e != nil {
		return true, e
	}
	if tag.RowsAffected() > 0 {
		_, e = tx.Exec(ctx, `UPDATE slack_intake_thread SET revision=revision+1,state='pending',updated_at=now(),last_error='' WHERE id=$1`, thread)
		if e != nil {
			return true, e
		}
	}
	return true, tx.Commit(ctx)
}
func (h *Handler) SlackSupportDirectory(w http.ResponseWriter, r *http.Request) {
	ws := workspaceIDFromURL(r, "id")
	m, ok := h.requireWorkspaceMember(w, r, ws, "workspace not found")
	if !ok {
		return
	}
	if m.Role != "admin" && m.Role != "owner" {
		writeError(w, 403, "administrator required")
		return
	}
	if h.SlackSupport == nil {
		writeError(w, 409, "connect Slack first")
		return
	}
	id, ok := parseUUIDOrBadRequest(w, r.URL.Query().Get("installation_id"), "installation_id")
	if !ok {
		return
	}
	inst, e := h.Queries.GetChannelInstallation(r.Context(), db.GetChannelInstallationParams{ID: id, ChannelType: "slack"})
	if e != nil || inst.WorkspaceID != parseUUID(ws) || inst.Status != "active" {
		writeError(w, 404, "installation not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, e := h.SlackSupport.Directory(ctx, inst.Config)
	if e != nil {
		writeError(w, 400, e.Error())
		return
	}
	writeJSON(w, 200, out)
}
func (h *Handler) SlackSupportActivity(w http.ResponseWriter, r *http.Request) {
	ws := workspaceIDFromURL(r, "id")
	m, ok := h.requireWorkspaceMember(w, r, ws, "workspace not found")
	if !ok {
		return
	}
	if m.Role != "admin" && m.Role != "owner" {
		writeError(w, 403, "administrator required")
		return
	}
	rows, e := h.DB.Query(r.Context(), `SELECT t.id::text,COALESCE(t.issue_id::text,''),r.channel_name,t.state,t.last_error,t.updated_at FROM slack_intake_thread t JOIN slack_intake_rule r ON r.id=t.rule_id WHERE t.workspace_id=$1 ORDER BY t.updated_at DESC LIMIT 50`, parseUUID(ws))
	if e != nil {
		writeError(w, 500, "could not load intake activity")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, issue, ch, state, err string
		var updated time.Time
		if rows.Scan(&id, &issue, &ch, &state, &err, &updated) != nil {
			writeError(w, 500, "could not read activity")
			return
		}
		out = append(out, map[string]any{"id": id, "issue_id": issue, "channel_name": ch, "state": state, "error": err, "updated_at": updated})
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not read activity")
		return
	}
	writeJSON(w, 200, out)
}
