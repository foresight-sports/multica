package handler

import (
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/workflow"
)

func workRequest(method, issueID, recordID, agentID, taskID string, body any) *http.Request {
	r := withURLParam(newRequest(method, "/work-records", body), "id", issueID)
	if recordID != "" {
		chi.RouteContext(r.Context()).URLParams.Add("recordId", recordID)
	}
	if agentID != "" {
		r.Header.Set("X-Actor-Source", "task_token")
		r.Header.Set("X-Agent-ID", agentID)
		r.Header.Set("X-Task-ID", taskID)
	}
	return r
}

func TestWorkInstallationApprovalMachineAffinity(t *testing.T) {
	aid, iid, rt, other, _ := executionFixture(t)
	taskID := dbfx.Task(t, aid, testutil.Cols{"runtime_id": rt, "issue_id": iid, "status": "running"})
	t.Cleanup(func() { testPool.Exec(context.Background(), "DELETE FROM work_record WHERE issue_id=$1", iid) })
	var record struct {
		ID                 string
		Revision           int64
		State              string
		Data               workflow.Data
		ContinuationTaskID string `json:"continuation_task_id"`
	}
	body := workRecordRequest{Kind: "installation", Data: workflow.Data{Tool: "fvm", Version: "3.2.1", Source: "https://pub.dev/packages/fvm", Scope: "user", Reason: "Build this checkout"}}
	testutil.Call(t, testHandler.WorkRecords, workRequest("POST", iid, "", aid, taskID, body)).Want(201).JSON(&record)
	if record.Data.Provider != "codex" || record.Data.Tool != "fvm" {
		t.Fatalf("lost structured machine context: %+v", record)
	}
	allowed := func(runtime string) bool {
		v, e := testHandler.Queries.TaskWorkflowAllowed(t.Context(), db.TaskWorkflowAllowedParams{AgentID: parseUUID(aid), IssueID: parseUUID(iid), RuntimeID: parseUUID(runtime)})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	if allowed(rt) || allowed(other) {
		t.Fatal("unapproved installation dispatched")
	}
	decision := workRecordRequest{Action: "approve", Revision: record.Revision}
	testutil.Call(t, testHandler.WorkRecords, workRequest("PUT", iid, record.ID, aid, taskID, decision)).Want(403)
	// Original turn can finish before the human responds; offline approval must
	// enqueue durable work without requiring any currently eligible machine.
	testPool.Exec(t.Context(), "UPDATE agent_task_queue SET status='completed',completed_at=now() WHERE id=$1", taskID)
	testPool.Exec(t.Context(), "UPDATE agent_runtime SET status='offline',last_seen_at=now()-interval '3 days' WHERE id=$1", rt)
	testutil.Call(t, testHandler.WorkRecords, workRequest("PUT", iid, record.ID, "", "", decision)).Want(200).JSON(&record)
	if record.State != "approved" || record.ContinuationTaskID == "" {
		t.Fatalf("no durable continuation: %+v", record)
	}
	if !allowed(rt) || allowed(other) {
		t.Fatal("approval did not bind the original machine")
	}
	testutil.Call(t, testHandler.WorkRecords, workRequest("PUT", iid, record.ID, "", "", decision)).Want(409)
	queued, e := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(record.ContinuationTaskID))
	if e != nil {
		t.Fatal(e)
	}
	if queued.TriggerCommentID.Valid || !strings.Contains(queued.HandoffNote.String, "installation approval") {
		t.Fatalf("approval replayed old trigger: %s", queued.Context)
	}
	testPool.Exec(t.Context(), "UPDATE agent_task_queue SET created_at=now()-interval '3 days',execution_selection='{}' WHERE id=$1", record.ContinuationTaskID)
	if _, e = testHandler.Queries.ExpireStaleQueuedTasks(t.Context(), db.ExpireStaleQueuedTasksParams{ReconnectGraceSecs: 1, MaxPerTick: 100}); e != nil {
		t.Fatal(e)
	}
	queued, _ = testHandler.Queries.GetAgentTask(t.Context(), queued.ID)
	if queued.Status != "queued" {
		t.Fatal("offline continuation expired", queued.Status)
	}
	// A new runtime registration with the same owner + daemon identity works.
	same := createClaimReclaimRuntime(t, t.Context(), "Re-registered machine")
	testPool.Exec(t.Context(), "UPDATE agent_runtime SET daemon_id=daemon_id||'-retired' WHERE id=$1", rt)
	if _, e := testPool.Exec(t.Context(), "UPDATE agent_runtime SET daemon_id=$2,provider='codex' WHERE id=$1", same, rt); e != nil {
		t.Fatal(e)
	}
	if !allowed(same) {
		t.Fatal("stable machine identity was lost on re-registration")
	}
	if _, e := testPool.Exec(t.Context(), "UPDATE agent_task_queue SET status='running',runtime_id=$2 WHERE id=$1", queued.ID, same); e != nil {
		t.Fatal(e)
	}
	testutil.Call(t, testHandler.WorkRecords, workRequest("PUT", iid, record.ID, aid, record.ContinuationTaskID, workRecordRequest{Action: "installed", Revision: record.Revision, Data: workflow.Data{Verification: "Current machine executable and version verified"}})).Want(200).JSON(&record)
	if allowed(other) {
		t.Fatal("installation verification released dependent work too early")
	}
	if _, e := testPool.Exec(t.Context(), "UPDATE agent_task_queue SET status='completed',completed_at=now() WHERE id=$1", queued.ID); e != nil {
		t.Fatal(e)
	}
	if e := testHandler.Queries.FinishInstalledWorkRecords(t.Context(), queued.ID); e != nil {
		t.Fatal(e)
	}
	if !allowed(other) {
		t.Fatal("finished installation continuation did not release affinity")
	}
}

func TestWorkCheckpointSingleWriterAndCAS(t *testing.T) {
	aid, iid, rt, _, _ := executionFixture(t)
	task := dbfx.Task(t, aid, testutil.Cols{"runtime_id": rt, "issue_id": iid, "status": "running"})
	other := dbfx.Task(t, aid, testutil.Cols{"runtime_id": rt, "issue_id": iid, "status": "running"})
	t.Cleanup(func() { testPool.Exec(context.Background(), "DELETE FROM work_record WHERE issue_id=$1", iid) })
	data := workflow.Data{Repository: "example/repo", Branch: "work/APP-3"}
	var record struct {
		ID       string
		Revision int64
		State    string
	}
	testutil.Call(t, testHandler.WorkRecords, workRequest("POST", iid, "", aid, task, workRecordRequest{Kind: "checkpoint", Data: data})).Want(201).JSON(&record)
	testutil.Call(t, testHandler.WorkRecords, workRequest("PUT", iid, record.ID, aid, other, workRecordRequest{Action: "claim", Revision: record.Revision})).Want(409)
	data.Commit = strings.Repeat("a", 40)
	data.NextStep = "Continue integration"
	testutil.Call(t, testHandler.WorkRecords, workRequest("PUT", iid, record.ID, aid, task, workRecordRequest{Action: "save", Revision: record.Revision, Data: data})).Want(200).JSON(&record)
	testutil.Call(t, testHandler.WorkRecords, workRequest("PUT", iid, record.ID, aid, other, workRecordRequest{Action: "claim", Revision: record.Revision - 1})).Want(409)
	testutil.Call(t, testHandler.WorkRecords, workRequest("PUT", iid, record.ID, aid, other, workRecordRequest{Action: "claim", Revision: record.Revision})).Want(200)
}

func TestWorkContextCursorPreservesSameTimestampComments(t *testing.T) {
	_, iid, _, _, _ := executionFixture(t)
	for i := 0; i < 55; i++ {
		dbfx.Insert(t, "comment", testutil.Cols{"workspace_id": testWorkspaceID, "issue_id": iid, "author_type": "member", "author_id": testUserID, "content": "bounded context", "created_at": "2026-01-01T00:00:00Z"})
	}
	var first struct {
		Comments  []json.RawMessage
		Next      string `json:"next_cursor"`
		Truncated map[string]bool
	}
	r := withURLParam(newRequest("GET", "/context?since=1970-01-01T00:00:00Z", nil), "id", iid)
	testutil.Call(t, testHandler.IssueContext, r).Want(200).JSON(&first)
	if len(first.Comments) != 50 || !first.Truncated["comments"] {
		t.Fatalf("unexpected first page: %+v", first)
	}
	r = withURLParam(newRequest("GET", "/context", nil), "id", iid)
	q := r.URL.Query()
	q.Set("since", first.Next)
	r.URL.RawQuery = q.Encode()
	var next struct {
		Comments  []json.RawMessage
		Truncated map[string]bool
	}
	testutil.Call(t, testHandler.IssueContext, r).Want(200).JSON(&next)
	if len(next.Comments) != 5 || next.Truncated["comments"] {
		t.Fatalf("cursor skipped comments: %+v", next)
	}
}

func TestWorkMessagePagesDoNotSkipEarlierMessages(t *testing.T) {
	aid, iid, rt, _, _ := executionFixture(t)
	task := dbfx.Task(t, aid, testutil.Cols{"runtime_id": rt, "issue_id": iid})
	for seq := 1; seq <= 5; seq++ {
		dbfx.InsertNoID(t, "task_message", testutil.Cols{"task_id": task, "seq": seq, "type": "text", "content": "message"}, "task_id=$1 AND seq=$2", task, seq)
	}
	for _, tc := range []struct {
		query string
		want  []int
	}{{"?tail=2", []int{4, 5}}, {"?tail=2&since=0", []int{1, 2}}, {"?tail=2&since=2", []int{3, 4}}} {
		var rows []struct{ Seq int }
		req := withURLParam(newRequest("GET", "/messages"+tc.query, nil), "taskId", task)
		testutil.Call(t, testHandler.ListTaskMessagesByUser, req.WithContext(middleware.SetMemberContext(req.Context(), testWorkspaceID, db.Member{}))).Want(200).JSON(&rows)
		if len(rows) != 2 || rows[0].Seq != tc.want[0] || rows[1].Seq != tc.want[1] {
			t.Fatalf("%s: %v", tc.query, rows)
		}
	}
	req := withURLParam(newRequest("GET", "/messages?tail=2&since=bogus", nil), "taskId", task)
	testutil.Call(t, testHandler.ListTaskMessagesByUser, req.WithContext(middleware.SetMemberContext(req.Context(), testWorkspaceID, db.Member{}))).Want(400)
}
