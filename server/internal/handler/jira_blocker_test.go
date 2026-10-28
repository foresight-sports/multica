package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestJiraBlockerDiscoveryNotificationAndOwnership(t *testing.T) {
	h := jiraTestHandler(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected Atlassian call") })
	h.Bus = events.New()
	published := 0
	h.Bus.Subscribe(protocol.EventInboxNew, func(event events.Event) {
		payload := event.Payload.(map[string]any)["item"].(map[string]any)
		if payload["recipient_id"] != testUserID {
			t.Fatal("notification recipient mismatch")
		}
		if _, err := h.Queries.GetInboxItem(t.Context(), parseUUID(payload["id"].(string))); err != nil {
			t.Fatal("notification published before commit", err)
		}
		published++
	})
	issue := dbfx.Issue(t, "Jira connection blocker")
	agent := createHandlerTestAgent(t, "Blocked Jira agent", []byte("[]"))
	task := dbfx.Task(t, agent, testutil.Cols{"issue_id": issue, "status": "running", "runtime_id": handlerTestRuntimeID(t), "originator_user_id": testUserID, "accountable_user_id": testUserID})
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM inbox_item WHERE id=$1`, jiraBlockerID(task)) })
	proxy := func(method string, tool ...string) *httptest.ResponseRecorder {
		name := "jira_connection_required"
		if len(tool) > 0 {
			name = tool[0]
		}
		r := newRequest("POST", jiraMCPPath+"/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": map[string]string{"name": name}})
		r.Header.Set("X-Actor-Source", "task_token")
		r.Header.Set("X-Task-ID", task)
		r.Header.Set("X-Agent-ID", agent)
		w := httptest.NewRecorder()
		h.JiraMCPProxy(w, r)
		return w
	}
	count := func() int {
		var n int
		if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM inbox_item WHERE id=$1`, jiraBlockerID(task)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, method := range []string{"initialize", "tools/list"} {
		w := proxy(method)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if count() != 0 {
		t.Fatal("discovery notified requester")
	}
	for range 2 {
		w := proxy("tools/call")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "isError") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if count() != 1 {
		t.Fatal("blocker was not idempotent")
	}
	if published != 1 {
		t.Fatal("expected one live notification", published)
	}
	jiraSeedConnection(t, h, testUserID, "connected-during-run", time.Now().Add(time.Hour))
	if w := proxy("tools/call", "getJiraIssue"); w.Code != 200 || !strings.Contains(w.Body.String(), "isError") {
		t.Fatal("OAuth silently resumed blocked run", w.Code, w.Body.String())
	}
	testPool.Exec(context.Background(), `DELETE FROM jira_mcp_connection WHERE user_id=$1`, testUserID)
	testPool.Exec(context.Background(), `UPDATE inbox_item SET read=true,archived=true WHERE id=$1`, jiraBlockerID(task))
	list := func(user string) *httptest.ResponseRecorder {
		r := withURLParam(newRequest("GET", "/", nil), "id", issue)
		r.Header.Set("X-User-ID", user)
		w := httptest.NewRecorder()
		h.ListIssueJiraBlockers(w, r)
		return w
	}
	if w := list(testUserID); w.Code != 200 || !strings.Contains(w.Body.String(), task) || !strings.Contains(w.Body.String(), `"can_resume":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
	other := dbfx.Insert(t, "user", testutil.Cols{"name": "Other", "email": "other-jira-blocker@example.test"})
	dbfx.Insert(t, "member", testutil.Cols{"workspace_id": testWorkspaceID, "user_id": other, "role": "member"})
	if w := list(other); w.Code != 200 || strings.Contains(w.Body.String(), task) {
		t.Fatal("blocker exposed to another member", w.Code, w.Body.String())
	}
	resume := func(user string) *httptest.ResponseRecorder {
		r := newRequest("POST", "/", nil)
		r.Header.Set("X-User-ID", user)
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", issue)
		rc.URLParams.Add("taskId", task)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
		w := httptest.NewRecorder()
		h.ResumeIssueJiraBlocker(w, r)
		return w
	}
	if w := resume(other); w.Code != 404 {
		t.Fatal("another member resumed blocker", w.Code, w.Body.String())
	}
	if w := resume(testUserID); w.Code != 409 {
		t.Fatal("active run resumed", w.Code, w.Body.String())
	}
	testPool.Exec(context.Background(), `UPDATE agent_task_queue SET status='completed' WHERE id=$1`, task)
	if w := resume(testUserID); w.Code != 409 {
		t.Fatal("missing connection resumed", w.Code, w.Body.String())
	}
	jiraSeedConnection(t, h, testUserID, "personal-access", time.Now().Add(time.Hour))
	testPool.Exec(context.Background(), `UPDATE agent SET owner_id=$2,permission_mode='private' WHERE id=$1`, agent, other)
	if w := resume(testUserID); w.Code != 403 {
		t.Fatal("revoked agent access bypassed", w.Code, w.Body.String())
	}
	testPool.Exec(context.Background(), `UPDATE agent SET owner_id=$2,permission_mode='public_to' WHERE id=$1`, agent, testUserID)
	var first, concurrent *httptest.ResponseRecorder
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); first = resume(testUserID) }()
	go func() { defer wg.Done(); concurrent = resume(testUserID) }()
	wg.Wait()
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	if concurrent.Code != 200 || concurrent.Body.String() != first.Body.String() {
		t.Fatal("concurrent resume duplicated work", concurrent.Code, concurrent.Body.String())
	}
	var result map[string]string
	json.Unmarshal(first.Body.Bytes(), &result)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE rerun_of_task_id=$1`, task)
	})
	second := resume(testUserID)
	if second.Code != 200 || second.Body.String() != first.Body.String() {
		t.Fatal("repeated resume differs", second.Code, second.Body.String())
	}
	// Simulate an interrupted response after enqueue but before receipt settlement.
	testPool.Exec(context.Background(), `UPDATE inbox_item SET details=details-'resumed_task_id' WHERE id=$1`, jiraBlockerID(task))
	if w := resume(testUserID); w.Code != 200 || w.Body.String() != first.Body.String() {
		t.Fatal("resume recovery duplicated work", w.Code, w.Body.String())
	}
	if w := list(testUserID); strings.Contains(w.Body.String(), task) {
		t.Fatal("resumed blocker remains active", w.Body.String())
	}
}

func TestJiraBlockerUpstreamUnauthorized(t *testing.T) {
	h := jiraTestHandler(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) })
	issue := dbfx.Issue(t, "Expired Jira connection")
	agent := createHandlerTestAgent(t, "Expired Jira", []byte("[]"))
	task := dbfx.Task(t, agent, testutil.Cols{"issue_id": issue, "status": "running", "runtime_id": handlerTestRuntimeID(t), "originator_user_id": testUserID, "accountable_user_id": testUserID})
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM inbox_item WHERE id=$1`, jiraBlockerID(task)) })
	jiraSeedConnection(t, h, testUserID, "expired-upstream", time.Now().Add(time.Hour))
	r := newRequest("POST", jiraMCPPath+"/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]string{"name": "getJiraIssue"}})
	r.Header.Set("X-Actor-Source", "task_token")
	r.Header.Set("X-Task-ID", task)
	r.Header.Set("X-Agent-ID", agent)
	w := httptest.NewRecorder()
	h.JiraMCPProxy(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "isError") {
		t.Fatal(w.Code, w.Body.String())
	}
	var status string
	testPool.QueryRow(context.Background(), `SELECT status FROM jira_mcp_connection WHERE user_id=$1`, testUserID).Scan(&status)
	if status != "reconnect_required" {
		t.Fatal(status)
	}
	var recipient string
	if err := testPool.QueryRow(context.Background(), `SELECT recipient_id::text FROM inbox_item WHERE id=$1`, jiraBlockerID(task)).Scan(&recipient); err != nil || recipient != testUserID {
		t.Fatal("requester was not notified", err, recipient)
	}
}
