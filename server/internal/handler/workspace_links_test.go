package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func linkedFixture(t *testing.T) (string, string) {
	t.Helper()
	target := dbfx.Workspace(t, "Linked SDK", "linked-sdk-test")
	dbfx.Member(t, target, testUserID, "owner")
	runtime := dbfx.Runtime(t, "SDK runtime", testutil.Cols{"workspace_id": target})
	agent := dbfx.Agent(t, "SDK intake", runtime, testutil.Cols{"workspace_id": target, "permission_mode": "public_to"})
	dbfx.InsertNoID(t, "agent_invocation_target", testutil.Cols{"agent_id": agent, "target_type": "workspace", "target_id": target}, "agent_id=$1", agent)
	squad := dbfx.Squad(t, "SDK squad", agent, testutil.Cols{"workspace_id": target})
	dbfx.Exec(t, `UPDATE workspace SET issue_intake=jsonb_build_object('revision',1,'default_squad_id',$1::text) WHERE id=$2`, squad, target)
	dbfx.InsertNoID(t, "repository_configuration", testutil.Cols{"scope": "workspace", "subject": target, "config": testutil.Raw(`'{"repository":"example/sdk","folder":"sdk","mode":"worktree"}'::jsonb`)}, "scope='workspace' AND subject=$1", target)
	sourceAgent := dbfx.Agent(t, "App coordinator", testRuntimeID, testutil.Cols{"permission_mode": "public_to"})
	dbfx.InsertNoID(t, "agent_invocation_target", testutil.Cols{"agent_id": sourceAgent, "target_type": "workspace", "target_id": testWorkspaceID}, "agent_id=$1", sourceAgent)
	source := dbfx.Issue(t, "App needs SDK information", testutil.Cols{"assignee_type": "agent", "assignee_id": sourceAgent, "creator_id": testUserID, "creator_type": "member"})
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE issue_id IN (SELECT target_issue_id FROM workspace_request WHERE source_issue_id=$1) OR issue_id=$1`, source)
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE id IN (SELECT target_issue_id FROM workspace_request WHERE source_issue_id=$1)`, source)
		testPool.Exec(context.Background(), `DELETE FROM workspace_request WHERE source_issue_id=$1`, source)
		testPool.Exec(context.Background(), `DELETE FROM workspace_link WHERE workspace_a=$1 OR workspace_b=$1`, target)
	})
	return target, source
}
func setWorkspaceLink(t *testing.T, target string, enabled bool, want int) {
	t.Helper()
	w := httptest.NewRecorder()
	testHandler.WorkspaceLinks(w, withURLParam(newRequest("PUT", "/workspace-links", map[string]any{"target_id": target, "enabled": enabled}), "id", testWorkspaceID))
	if w.Code != want {
		t.Fatalf("link: %d %s", w.Code, w.Body.String())
	}
}
func TestWorkspaceLinksAuthorization(t *testing.T) {
	target, source := linkedFixture(t)
	setWorkspaceLink(t, target, true, 200)
	// The reciprocal relation is the same link.
	var n int
	dbfx.QueryRow(t, `SELECT count(*) FROM workspace_link WHERE enabled AND (workspace_a=$1 OR workspace_b=$1)`, target).Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	dbfx.Exec(t, `UPDATE member SET role='member' WHERE workspace_id=$1 AND user_id=$2`, target, testUserID)
	setWorkspaceLink(t, target, false, 403)
	dbfx.Exec(t, `UPDATE member SET role='owner' WHERE workspace_id=$1 AND user_id=$2`, target, testUserID)
	// An agent may not impersonate its owner to administer links.
	w := httptest.NewRecorder()
	r := withURLParam(newRequest("PUT", "/workspace-links", map[string]any{"target_id": target, "enabled": false}), "id", testWorkspaceID)
	r.Header.Set("X-Actor-Source", "task_token")
	testHandler.WorkspaceLinks(w, r)
	if w.Code != 403 {
		t.Fatalf("agent modified link: %d", w.Code)
	}
	setWorkspaceLink(t, target, false, 200)
	w = httptest.NewRecorder()
	testHandler.WorkspaceRequests(w, withURLParam(newRequest("POST", "/workspace-requests", map[string]string{"workspace_id": target, "request_key": "sdk-info", "title": "Read SDK"}), "id", source))
	if w.Code != 403 {
		t.Fatalf("unlinked create: %d %s", w.Code, w.Body.String())
	}
}
func TestWorkspaceRequestCreateWakeAndRevocation(t *testing.T) {
	target, source := linkedFixture(t)
	setWorkspaceLink(t, target, true, 200)
	call := func(method string) []workspaceRequestView {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.WorkspaceRequests(w, withURLParam(newRequest(method, "/workspace-requests", map[string]string{"workspace_id": target, "request_key": "sdk-info", "title": "Explain SDK behavior", "description": "Investigate only; no implementation requested."}), "id", source))
		if w.Code != 200 {
			t.Fatalf("request: %d %s", w.Code, w.Body.String())
		}
		var result []workspaceRequestView
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := call("POST")
	second := call("POST")
	if len(first) != 1 || len(second) != 1 || first[0].TargetIssueID != second[0].TargetIssueID {
		t.Fatal("retry created duplicate")
	}
	child, err := testHandler.Queries.GetIssue(t.Context(), parseUUID(first[0].TargetIssueID))
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentIssueID.Valid || child.WorkspaceID != parseUUID(target) || child.CreatorID != parseUUID(testUserID) || child.AssigneeType.String != "squad" {
		t.Fatalf("invalid cross-workspace routing: %+v", child)
	}
	if err = testHandler.syncWorkspaceRequest(t.Context(), parseUUID(first[0].ID)); err != nil {
		t.Fatal(err)
	}
	var count int
	dbfx.QueryRow(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id=$1`, source).Scan(&count)
	if count != 0 {
		t.Fatal("initial request woke parent")
	}
	parent, _ := testHandler.Queries.GetIssue(t.Context(), parseUUID(source))
	busy := dbfx.Task(t, uuidToString(parent.AssigneeID), testutil.Cols{"issue_id": source, "status": "running", "runtime_id": testRuntimeID})
	dbfx.Exec(t, `UPDATE issue SET status='in_review' WHERE id=$1`, child.ID)
	if err = testHandler.syncWorkspaceRequest(t.Context(), parseUUID(first[0].ID)); err == nil {
		t.Fatal("busy parent not deferred")
	}
	dbfx.Exec(t, `UPDATE agent_task_queue SET status='completed' WHERE id=$1`, busy)
	if err = testHandler.syncWorkspaceRequest(t.Context(), parseUUID(first[0].ID)); err != nil {
		t.Fatal(err)
	}
	if err = testHandler.syncWorkspaceRequest(t.Context(), parseUUID(first[0].ID)); err != nil {
		t.Fatal(err)
	}
	var queuedID, origin string
	dbfx.QueryRow(t, `SELECT id::text,originator_user_id::text FROM agent_task_queue WHERE issue_id=$1 AND status='queued'`, source).Scan(&queuedID, &origin)
	if origin != testUserID {
		t.Fatal("borrowed owner identity")
	}
	dbfx.QueryRow(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id=$1 AND status='queued'`, source).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate wake")
	}
	dbfx.Exec(t, `UPDATE agent_task_queue SET status='completed' WHERE id=$1`, queuedID)
	dbfx.Exec(t, `UPDATE issue SET status='done' WHERE id=$1`, child.ID)
	setWorkspaceLink(t, target, false, 200)
	if err = testHandler.syncWorkspaceRequest(t.Context(), parseUUID(first[0].ID)); err == nil {
		t.Fatal("unlink did not stop wake")
	}
	if len(call("GET")) != 0 {
		t.Fatal("unlinked target leaked")
	}
	setWorkspaceLink(t, target, true, 200)
	dbfx.Exec(t, `DELETE FROM member WHERE workspace_id=$1 AND user_id=$2`, target, testUserID)
	if err = testHandler.syncWorkspaceRequest(t.Context(), parseUUID(first[0].ID)); err == nil {
		t.Fatal("revoked membership did not stop wake")
	}
	if len(call("GET")) != 0 {
		t.Fatal("revoked target leaked")
	}
}
func TestWorkspaceRequestTaskOriginBound(t *testing.T) {
	target, source := linkedFixture(t)
	setWorkspaceLink(t, target, true, 200)
	parent, _ := testHandler.Queries.GetIssue(t.Context(), parseUUID(source))
	task := dbfx.Task(t, uuidToString(parent.AssigneeID), testutil.Cols{"issue_id": source, "status": "running", "originator_user_id": testUserID, "accountable_user_id": testUserID, "runtime_id": testRuntimeID})
	other := dbfx.Issue(t, "Another origin")
	w := httptest.NewRecorder()
	r := withURLParam(newRequest("POST", "/workspace-requests", map[string]string{"workspace_id": target, "request_key": "task-bound", "title": "Read SDK"}), "id", other)
	r.Header.Set("X-Actor-Source", "task_token")
	r.Header.Set("X-Task-ID", task)
	r.Header.Set("X-Agent-ID", uuidToString(parent.AssigneeID))
	testHandler.WorkspaceRequests(w, r)
	if w.Code != 403 {
		t.Fatalf("task escaped source issue: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r = withURLParam(newRequest("POST", "/workspace-requests", map[string]string{"workspace_id": target, "request_key": "task-bound", "title": "Read SDK"}), "id", source)
	r.Header.Set("X-Actor-Source", "task_token")
	r.Header.Set("X-Task-ID", task)
	r.Header.Set("X-Agent-ID", uuidToString(parent.AssigneeID))
	testHandler.WorkspaceRequests(w, r)
	if w.Code != 200 {
		t.Fatalf("delegated request: %d %s", w.Code, w.Body.String())
	}
	var result []workspaceRequestView
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result) != 1 {
		t.Fatalf("invalid response: %s", w.Body.String())
	}
	child, err := testHandler.Queries.GetIssue(t.Context(), parseUUID(result[0].TargetIssueID))
	if err != nil {
		t.Fatal(err)
	}
	if child.OriginType.String != "agent_create" || child.OriginID != parseUUID(task) || child.CreatorID != parseUUID(testUserID) {
		t.Fatalf("lost delegation attribution: %+v", child)
	}
	// Missing original requester must never fall back to the machine owner.
	dbfx.Exec(t, `UPDATE agent_task_queue SET originator_user_id=NULL,accountable_user_id=NULL WHERE id=$1`, task)
	_, err = testHandler.linkedRequester(r, &db.Issue{ID: parseUUID(source)})
	if err == nil {
		t.Fatal("owner fallback accepted")
	}
}

func TestWorkspaceRequestClosedOriginRollsBack(t *testing.T) {
	target, source := linkedFixture(t)
	setWorkspaceLink(t, target, true, 200)
	dbfx.Exec(t, `UPDATE issue SET status='done' WHERE id=$1`, source)
	w := httptest.NewRecorder()
	testHandler.WorkspaceRequests(w, withURLParam(newRequest("POST", "/workspace-requests", map[string]string{"workspace_id": target, "request_key": "closed-origin", "title": "Must roll back"}), "id", source))
	if w.Code != 409 {
		t.Fatalf("closed origin: %d %s", w.Code, w.Body.String())
	}
	var count int
	dbfx.QueryRow(t, `SELECT count(*) FROM issue WHERE workspace_id=$1`, target).Scan(&count)
	if count != 0 {
		t.Fatal("orphaned destination issue after failed dependency")
	}
	dbfx.QueryRow(t, `SELECT count(*) FROM workspace_request WHERE source_issue_id=$1`, source).Scan(&count)
	if count != 0 {
		t.Fatal("orphaned dependency")
	}
}

func TestWorkspaceLinkedContextRespectsRequester(t *testing.T) {
	target, _ := linkedFixture(t)
	setWorkspaceLink(t, target, true, 200)
	response := AgentTaskResponse{WorkspaceID: testWorkspaceID}
	testHandler.appendLinkedWorkspaceContext(t.Context(), db.AgentTaskQueue{OriginatorUserID: parseUUID(testUserID)}, &response)
	if !strings.Contains(response.WorkspaceContext, target) || !strings.Contains(response.WorkspaceContext, "multica issue linked-request") {
		t.Fatal("missing linked workspace guidance")
	}
	response.WorkspaceContext = ""
	testHandler.appendLinkedWorkspaceContext(t.Context(), db.AgentTaskQueue{}, &response)
	if response.WorkspaceContext != "" {
		t.Fatal("machine owner used as requester")
	}
}
