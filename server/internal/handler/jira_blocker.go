package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func jiraBlockerID(task string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("multica:jira-connection:"+task)).String()
}

// The inbox receipt is durable even after read/archive. Only an explicit resume
// resolves it. Its identity deduplicates repeated MCP calls from the same task.
func (h *Handler) recordJiraBlocker(ctx context.Context, task, user string) error {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id pgtype.UUID
	err = tx.QueryRow(ctx, `INSERT INTO inbox_item(id,workspace_id,recipient_type,recipient_id,type,severity,issue_id,title,body,actor_type,actor_id,details)
 SELECT $1,a.workspace_id,'member',t.originator_user_id,'agent_blocked','attention',t.issue_id,
 'Connect Jira to continue', CASE WHEN t.issue_id IS NOT NULL THEN 'Open this issue and select Connect Jira. Sign in with your own Atlassian account, then return and select Resume.' ELSE 'Open Settings > Account > Jira connection and connect your own Atlassian account. Then return to the conversation and retry your request.' END,
 'agent',t.agent_id,jsonb_build_object('reason','jira_connection_required','task_id',t.id::text)
 FROM agent_task_queue t JOIN agent a ON a.id=t.agent_id
 JOIN member m ON m.workspace_id=a.workspace_id AND m.user_id=t.originator_user_id
 WHERE t.id=$2 AND t.originator_user_id=$3 AND t.status='running'
 ON CONFLICT(id) DO NOTHING RETURNING id`, jiraBlockerID(task), task, user).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	item, err := h.Queries.WithTx(tx).GetInboxItem(ctx, id)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if h.Bus != nil {
		raw, _ := json.Marshal(h.enrichInboxResponse(ctx, inboxToResponse(item), item.IssueID))
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		h.Bus.Publish(events.Event{Type: protocol.EventInboxNew, WorkspaceID: uuidToString(item.WorkspaceID), ActorType: "agent", ActorID: uuidToString(item.ActorID), Payload: map[string]any{"item": payload}})
	}
	return nil
}

type jiraBlockerResponse struct {
	TaskID    string `json:"task_id"`
	AgentName string `json:"agent_name"`
	CanResume bool   `json:"can_resume"`
}

func (h *Handler) ListIssueJiraBlockers(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	user, ok := requireUserID(w, r)
	if !ok {
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT t.id::text,a.name,t.status IN ('completed','failed','cancelled')
 FROM inbox_item i JOIN agent_task_queue t ON t.id::text=i.details->>'task_id'
 JOIN agent a ON a.id=t.agent_id
 WHERE i.issue_id=$1 AND i.workspace_id=$2 AND i.recipient_type='member' AND i.recipient_id=$3
 AND t.originator_user_id=$3 AND i.details->>'reason'='jira_connection_required'
 AND NOT (i.details ? 'resumed_task_id')
 AND NOT EXISTS(SELECT 1 FROM agent_task_queue child WHERE child.rerun_of_task_id=t.id AND child.originator_user_id=$3)
 ORDER BY i.created_at DESC LIMIT 50`, issue.ID, issue.WorkspaceID, user)
	if err != nil {
		writeError(w, 500, "Could not load Jira blockers")
		return
	}
	defer rows.Close()
	out := []jiraBlockerResponse{}
	for rows.Next() {
		var b jiraBlockerResponse
		if err = rows.Scan(&b.TaskID, &b.AgentName, &b.CanResume); err != nil {
			writeError(w, 500, "Could not load Jira blockers")
			return
		}
		out = append(out, b)
	}
	if rows.Err() != nil {
		writeError(w, 500, "Could not load Jira blockers")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, out)
}

// Keep the receipt locked across the normal rerun service. That service owns its
// own transactions. If the process dies after enqueue, the persisted rerun lineage
// recovers the result on retry instead of creating another run.
func (h *Handler) ResumeIssueJiraBlocker(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	user, ok := requireUserID(w, r)
	if !ok {
		return
	}
	taskID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "taskId"), "task_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not resume Jira work")
		return
	}
	defer tx.Rollback(r.Context())
	var details []byte
	err = tx.QueryRow(r.Context(), `SELECT details FROM inbox_item WHERE id=$1 AND issue_id=$2 AND workspace_id=$3 AND recipient_type='member' AND recipient_id=$4 AND details->>'reason'='jira_connection_required' FOR UPDATE`, jiraBlockerID(uuidToString(taskID)), issue.ID, issue.WorkspaceID, user).Scan(&details)
	if err != nil {
		writeError(w, 404, "Jira blocker not found")
		return
	}
	source, err := h.Queries.GetAgentTask(r.Context(), taskID)
	if err != nil || uuidToString(source.OriginatorUserID) != user || source.IssueID != issue.ID {
		writeError(w, 403, "Only the requester can resume this Jira work")
		return
	}
	agent, err := h.Queries.GetAgent(r.Context(), source.AgentID)
	if err != nil || !h.canInvokeAgent(r.Context(), agent, "member", user, user, uuidToString(issue.WorkspaceID)) {
		writeError(w, 403, "You cannot invoke this agent")
		return
	}
	var resumed pgtype.UUID
	err = tx.QueryRow(r.Context(), `SELECT id FROM agent_task_queue WHERE rerun_of_task_id=$1 AND originator_user_id=$2 ORDER BY created_at LIMIT 1`, taskID, user).Scan(&resumed)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "Could not check resumed work")
		return
	}
	if !resumed.Valid {
		if source.Status != "completed" && source.Status != "failed" && source.Status != "cancelled" {
			writeError(w, 409, "Wait for the current run to finish before resuming")
			return
		}
		if h.JiraMCP == nil {
			writeError(w, 503, "Jira connection is not configured")
			return
		}
		if _, err = h.jiraAccessToken(r.Context(), user); err != nil {
			if errors.Is(err, errJiraReconnect) {
				writeError(w, 409, "Jira authorization is no longer valid. Connect Jira again, then resume.")
			} else {
				writeError(w, 503, "Could not verify Jira connection. Try Resume again shortly.")
			}
			return
		}
		canInvoke := func(a db.Agent) bool {
			return h.canInvokeAgent(r.Context(), a, "member", user, user, uuidToString(issue.WorkspaceID))
		}
		task, err := h.TaskService.RerunIssue(r.Context(), issue.ID, taskID, pgtype.UUID{}, parseUUID(user), canInvoke)
		if err != nil {
			writeError(w, 409, err.Error())
			return
		}
		resumed = task.ID
	}
	_, err = tx.Exec(r.Context(), `UPDATE inbox_item SET details=details||jsonb_build_object('resumed_task_id',$2::text),read=true,title='Jira work resumed',body='A new agent run has been queued for this issue.',severity='info' WHERE id=$1`, jiraBlockerID(uuidToString(taskID)), uuidToString(resumed))
	if err != nil {
		writeError(w, 500, "Could not record resumed work")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not record resumed work")
		return
	}
	writeJSON(w, 200, map[string]string{"task_id": uuidToString(resumed)})
}

// Ordinary per-run retries also resolve a receipt when the same requester
// deliberately starts the successor. Do not leave an obsolete action pinned.
func (h *Handler) settleJiraBlockerForRerun(ctx context.Context, source, successor pgtype.UUID, user string) {
	if !source.Valid || user == "" {
		return
	}
	_, err := h.DB.Exec(ctx, `UPDATE inbox_item SET details=details||jsonb_build_object('resumed_task_id',$2::text),read=true,title='Jira work resumed',body='A new agent run has been queued for this issue.',severity='info' WHERE id=$1 AND recipient_type='member' AND recipient_id=$3 AND details->>'reason'='jira_connection_required' AND NOT(details ? 'resumed_task_id')`, jiraBlockerID(uuidToString(source)), uuidToString(successor), user)
	if err != nil {
		slog.Warn("could not settle Jira blocker after rerun", "task_id", uuidToString(source), "error", err)
	}
}

// An unauthenticated gateway still initializes successfully. Tool discovery alone
// must never block unrelated work or notify a user. The explicit tool invocation
// is the evidence that this task needs Jira.
func (h *Handler) jiraConnectionRequired(w http.ResponseWriter, r *http.Request, body []byte, user string) {
	var rpc struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if json.Unmarshal(body, &rpc) != nil {
		writeError(w, 400, "Invalid MCP request")
		return
	}
	var result any
	switch rpc.Method {
	case "initialize":
		result = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "multica-jira", "version": "1.0"}}
	case "notifications/initialized", "notifications/cancelled":
		w.WriteHeader(http.StatusAccepted)
		return
	case "ping":
		result = map[string]any{}
	case "tools/list":
		result = map[string]any{"tools": []any{map[string]any{"name": "jira_connection_required", "description": "Call when this task needs Jira. Notifies the requesting user to connect their personal Jira account and enables resuming from the issue. Do not call for work that does not need Jira.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}}}
	case "tools/call":
		if !jiraMCPToolAllowed(rpc.Params.Name) {
			writeError(w, 403, "This connection exposes Jira tools only")
			return
		}
		if err := h.recordJiraBlocker(r.Context(), r.Header.Get("X-Task-ID"), user); err != nil {
			writeError(w, 500, "Could not notify the Jira requester; retry")
			return
		}
		result = map[string]any{"isError": true, "content": []any{map[string]string{"type": "text", "text": "Jira work is blocked. The requester must connect their personal Jira account from the issue or Settings > Account > Jira connection, then select Resume on the issue. Stop Jira work and report the blocker; do not substitute another account."}}}
	default:
		writeError(w, 403, "MCP method is not available for Jira")
		return
	}
	writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result})
}
