package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/integrations/slack"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type supportFixture struct{ rule, install, coord, analyst, writer, issue, thread string }

func newSupportFixture(t *testing.T) supportFixture {
	t.Helper()
	f := supportFixture{}
	dbfx.InsertNoID(t, "repository_configuration", testutil.Cols{"scope": "workspace", "subject": testWorkspaceID, "config": testutil.Raw(`'{"repository":"example/support","folder":"support","mode":"worktree"}'::jsonb`)}, "scope='workspace' AND subject=$1", testWorkspaceID)
	for i, dest := range []*string{&f.coord, &f.analyst, &f.writer} {
		*dest = dbfx.Agent(t, fmt.Sprintf("Support test %d", i), testRuntimeID, testutil.Cols{"permission_mode": "public_to"})
		dbfx.InsertNoID(t, "agent_invocation_target", testutil.Cols{"agent_id": *dest, "target_type": "workspace", "target_id": testWorkspaceID}, "agent_id=$1", *dest)
	}
	f.install = dbfx.Insert(t, "channel_installation", testutil.Cols{"workspace_id": testWorkspaceID, "agent_id": f.coord, "installer_user_id": testUserID, "channel_type": "slack", "status": "active", "config": testutil.Raw(`'{"app_id":"ASUPPORT","team_id":"TSUPPORT","bot_token_encrypted":"eG94Yi10ZXN0"}'::jsonb`)})
	f.rule = dbfx.Insert(t, "slack_intake_rule", testutil.Cols{"id": uuidToString(dbid.NewV7()), "workspace_id": testWorkspaceID, "installation_id": f.install, "channel_id": "CSUPPORT", "channel_name": "support", "service_user_id": testUserID, "coordinator_id": f.coord, "analyst_id": f.analyst, "composer_id": f.writer, "escalation_user_id": "UKODY01", "enabled": true})
	f.issue = dbfx.Issue(t, "Slack support", testutil.Cols{"creator_id": testUserID, "creator_type": "member", "assignee_type": "agent", "assignee_id": f.coord})
	dbfx.Exec(t, `UPDATE workspace SET issue_counter=(SELECT COALESCE(max(number),0) FROM issue WHERE workspace_id=$1) WHERE id=$1`, testWorkspaceID)
	f.thread = dbfx.Insert(t, "slack_intake_thread", testutil.Cols{"id": uuidToString(dbid.NewV7()), "workspace_id": testWorkspaceID, "rule_id": f.rule, "root_ts": "100.1", "issue_id": f.issue, "revision": 1})
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM slack_intake_reply WHERE thread_id=$1`, f.thread)
		testPool.Exec(context.Background(), `DELETE FROM slack_intake_message WHERE rule_id=$1`, f.rule)
		testPool.Exec(context.Background(), `DELETE FROM slack_intake_thread WHERE rule_id=$1`, f.rule)
	})
	return f
}
func TestSlackIntakeCaptureDeduplicatesAndScopes(t *testing.T) {
	f := newSupportFixture(t)
	m := channel.InboundMessage{MessageID: "100.2", Source: channel.Source{ChatType: channel.ChatTypeGroup, ChatID: "CSUPPORT", ThreadID: "100.1", SenderID: "UCSREP"}, Text: "new evidence", Raw: json.RawMessage(`{"api_app_id":"ASUPPORT","team_id":"TSUPPORT"}`)}
	for i := 0; i < 2; i++ {
		handled, e := testHandler.CaptureSlackIntake(t.Context(), m)
		if e != nil || !handled {
			t.Fatalf("capture %v %v", handled, e)
		}
	}
	var rev, n int
	dbfx.QueryRow(t, `SELECT revision FROM slack_intake_thread WHERE id=$1`, f.thread).Scan(&rev)
	dbfx.QueryRow(t, `SELECT count(*) FROM slack_intake_message WHERE thread_id=$1`, f.thread).Scan(&n)
	if rev != 2 || n != 1 {
		t.Fatalf("duplicate receipt rev=%d count=%d", rev, n)
	}
	m.Source.ChatID = "COTHER"
	handled, e := testHandler.CaptureSlackIntake(t.Context(), m)
	if handled || e != nil {
		t.Fatal("captured another channel")
	}
	m.Source.ChatID = "CSUPPORT"
	m.Source.ThreadID = "999.1"
	m.MessageID = "999.2"
	_, e = testHandler.CaptureSlackIntake(t.Context(), m)
	if e != nil {
		t.Fatal(e)
	}
	dbfx.QueryRow(t, `SELECT count(*) FROM slack_intake_thread WHERE rule_id=$1`, f.rule).Scan(&n)
	if n != 1 {
		t.Fatal("orphan reply created issue")
	}
	dbfx.Exec(t, `UPDATE slack_intake_thread SET state='deleted' WHERE id=$1`, f.thread)
	m.Source.ThreadID = "100.1"
	m.MessageID = "100.3"
	_, e = testHandler.CaptureSlackIntake(t.Context(), m)
	if e != nil {
		t.Fatal(e)
	}
	dbfx.QueryRow(t, `SELECT revision FROM slack_intake_thread WHERE id=$1`, f.thread).Scan(&rev)
	if rev != 2 {
		t.Fatal("deleted thread resurrected")
	}
}
func TestSlackSupportWriterCannotPublishAndCoordinatorWaits(t *testing.T) {
	f := newSupportFixture(t)
	task := dbfx.Task(t, f.coord, testutil.Cols{"issue_id": f.issue, "status": "running", "originator_user_id": testUserID, "accountable_user_id": testUserID, "runtime_id": testRuntimeID})
	writerTask := dbfx.Task(t, f.writer, testutil.Cols{"issue_id": f.issue, "status": "running", "originator_user_id": testUserID, "accountable_user_id": testUserID, "runtime_id": testRuntimeID})
	call := func(taskID, agent string, rev int, confidence float64, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		r := withURLParam(newRequest("POST", "/support-reply", map[string]any{"revision": rev, "outcome": "answer", "confidence": confidence, "body": "Verified finding.", "evidence": "Reproduced against provided logs."}), "id", f.issue)
		r.Header.Set("X-Actor-Source", "task_token")
		r.Header.Set("X-Task-ID", taskID)
		r.Header.Set("X-Agent-ID", agent)
		testHandler.SupportReply(w, r)
		if w.Code != want {
			t.Fatalf("want %d got %d %s", want, w.Code, w.Body.String())
		}
	}
	call(writerTask, f.writer, 1, .95, 403)
	call(task, f.coord, 1, .95, 409)
	dbfx.Task(t, f.analyst, testutil.Cols{"issue_id": f.issue, "status": "completed", "runtime_id": testRuntimeID, "originator_user_id": testUserID, "accountable_user_id": testUserID, "result": testutil.Raw(`'{"summary":"verified evidence"}'::jsonb`)})
	call(task, f.coord, 1, .95, 409)
	dbfx.Exec(t, `UPDATE agent_task_queue SET status='completed',result='{"summary":"concise verified wording"}'::jsonb WHERE id=$1`, writerTask)
	call(task, f.coord, 2, .95, 400)
	call(task, f.coord, 1, .95, 200)
	call(task, f.coord, 1, .95, 200)
	var n int
	dbfx.QueryRow(t, `SELECT count(*) FROM slack_intake_reply WHERE thread_id=$1`, f.thread).Scan(&n)
	if n != 1 {
		t.Fatal("duplicate external response")
	}
}
func TestSlackSupportLowConfidenceEscalatesWithoutInventingAnswer(t *testing.T) {
	f := newSupportFixture(t)
	task := dbfx.Task(t, f.coord, testutil.Cols{"issue_id": f.issue, "status": "running", "originator_user_id": testUserID, "accountable_user_id": testUserID, "runtime_id": testRuntimeID})
	w := httptest.NewRecorder()
	r := withURLParam(newRequest("POST", "/support-reply", map[string]any{"revision": 1, "outcome": "answer", "confidence": .6, "body": "Unverified claim", "evidence": "Missing logs"}), "id", f.issue)
	r.Header.Set("X-Actor-Source", "task_token")
	r.Header.Set("X-Task-ID", task)
	r.Header.Set("X-Agent-ID", f.coord)
	testHandler.SupportReply(w, r)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var outcome, body string
	dbfx.QueryRow(t, `SELECT outcome,body FROM slack_intake_reply WHERE thread_id=$1`, f.thread).Scan(&outcome, &body)
	if outcome != "escalate" || body == "Unverified claim" {
		t.Fatal("unverified answer escaped")
	}
}

func TestSlackIntakeServiceIdentityAndDisableOffline(t *testing.T) {
	f := newSupportFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/conversations.info" {
			w.Write([]byte(`{"ok":true,"channel":{"id":"CNEW001","name":"new-support","is_member":true}}`))
		} else {
			w.Write([]byte(`{"ok":true,"user":{"id":"UKODY01","name":"Kody"}}`))
		}
	}))
	defer srv.Close()
	prior := testHandler.SlackSupport
	testHandler.SlackSupport = &slack.SupportClient{APIURL: srv.URL + "/"}
	t.Cleanup(func() { testHandler.SlackSupport = prior })
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM slack_intake_rule WHERE workspace_id=$1 AND channel_id='CNEW001'`, testWorkspaceID)
		testPool.Exec(context.Background(), `DELETE FROM member WHERE user_id IN(SELECT user_id FROM automation_account WHERE workspace_id=$1)`, testWorkspaceID)
		testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id IN(SELECT user_id FROM automation_account WHERE workspace_id=$1)`, testWorkspaceID)
		testPool.Exec(context.Background(), `DELETE FROM automation_account WHERE workspace_id=$1`, testWorkspaceID)
	})
	payload := map[string]any{"installation_id": f.install, "channel_id": "CNEW001", "coordinator_id": f.coord, "analyst_id": f.analyst, "composer_id": f.writer, "workflow": "support_diagnosis", "escalation_user_id": "UKODY01", "enabled": true, "revision": 0, "timeout_minutes": 60}
	call := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.SlackIntakeRules(w, withURLParam(newRequest("PUT", "/intake", payload), "id", testWorkspaceID))
		if w.Code != want {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	call(200)
	var id, serviceID, name, email string
	dbfx.QueryRow(t, `SELECT r.id::text,r.service_user_id::text,u.name,u.email FROM slack_intake_rule r JOIN "user" u ON u.id=r.service_user_id WHERE r.workspace_id=$1 AND r.channel_id='CNEW001'`, testWorkspaceID).Scan(&id, &serviceID, &name, &email)
	if serviceID == testUserID || name != "FS Caddie" || !strings.HasSuffix(email, "@automation.multica.invalid") {
		t.Fatal("not a distinct service identity")
	}
	if _, _, err := testHandler.findOrCreateUser(t.Context(), email); err == nil {
		t.Fatal("service identity can log in")
	}
	payload["id"] = id
	payload["revision"] = 1
	payload["enabled"] = false
	payload["disable_only"] = true
	testHandler.SlackSupport = nil
	call(200)
	call(409)
}
func TestSlackSupportDefectOnlyCreatesBacklog(t *testing.T) {
	f := newSupportFixture(t)
	for _, a := range []string{f.analyst, f.writer} {
		dbfx.Task(t, a, testutil.Cols{"issue_id": f.issue, "status": "completed", "runtime_id": testRuntimeID, "originator_user_id": testUserID, "accountable_user_id": testUserID, "result": testutil.Raw(`'{"summary":"Confirmed defect with reproduction"}'::jsonb`)})
	}
	task := dbfx.Task(t, f.coord, testutil.Cols{"issue_id": f.issue, "status": "running", "originator_user_id": testUserID, "accountable_user_id": testUserID, "runtime_id": testRuntimeID})
	w := httptest.NewRecorder()
	r := withURLParam(newRequest("POST", "/support-reply", map[string]any{"revision": 1, "outcome": "defect", "confidence": .95, "body": "We confirmed the issue.", "evidence": "Reproduction and logs agree."}), "id", f.issue)
	r.Header.Set("X-Actor-Source", "task_token")
	r.Header.Set("X-Task-ID", task)
	r.Header.Set("X-Agent-ID", f.coord)
	testHandler.SupportReply(w, r)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var backlog, status string
	dbfx.QueryRow(t, `SELECT i.id::text,i.status FROM slack_intake_thread t JOIN issue i ON i.id=t.backlog_issue_id WHERE t.id=$1`, f.thread).Scan(&backlog, &status)
	if status != "backlog" {
		t.Fatal("defect not in backlog")
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id=$1`, backlog) })
	var count int
	dbfx.QueryRow(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id=$1`, backlog).Scan(&count)
	if count != 0 {
		t.Fatal("support authorized implementation")
	}
}

func TestSlackSupportWorkerCreatesOneAttributedInvestigation(t *testing.T) {
	f := newSupportFixture(t)
	dbfx.Exec(t, `UPDATE slack_intake_thread SET issue_id=NULL WHERE id=$1`, f.thread)
	dbfx.Insert(t, "slack_intake_message", testutil.Cols{"id": uuidToString(dbid.NewV7()), "thread_id": f.thread, "rule_id": f.rule, "message_ts": "100.1", "sender_id": "UCSREP", "body": "App fails to connect"})
	if err := testHandler.processSupportThread(t.Context(), parseUUID(f.thread)); err != nil {
		t.Fatal(err)
	}
	var issueID, creator, assignee string
	dbfx.QueryRow(t, `SELECT i.id::text,i.creator_id::text,i.assignee_id::text FROM issue i JOIN slack_intake_thread t ON t.issue_id=i.id WHERE t.id=$1`, f.thread).Scan(&issueID, &creator, &assignee)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE issue_id=$1`, issueID)
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE id=$1`, issueID)
	})
	if creator != testUserID || assignee != f.coord {
		t.Fatal("lost explicit service attribution or coordinator")
	}
	var count int
	dbfx.QueryRow(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id=$1 AND originator_user_id=$2`, issueID, testUserID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected one coordinator task got %d", count)
	}
}

func TestSlackSupportUncertainDeliveryReconcilesWithoutReposting(t *testing.T) {
	f := newSupportFixture(t)
	reply := dbfx.Insert(t, "slack_intake_reply", testutil.Cols{"id": uuidToString(dbid.NewV7()), "thread_id": f.thread, "revision": 1, "outcome": "answer", "confidence": .95, "body": "Finding", "evidence": "Evidence"})
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/chat.postMessage":
			posts++
			w.Write([]byte(`{"ok":false,"error":"internal_error"}`))
		case "/conversations.replies":
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "messages": []any{map[string]any{"ts": "100.3", "metadata": map[string]any{"event_type": "multica_support_reply", "event_payload": map[string]any{"reply_id": reply}}}}})
		default:
			w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()
	prior := testHandler.SlackSupport
	testHandler.SlackSupport = &slack.SupportClient{APIURL: srv.URL + "/"}
	defer func() { testHandler.SlackSupport = prior }()
	if err := testHandler.processSupportThread(t.Context(), parseUUID(f.thread)); err == nil {
		t.Fatal("expected uncertain delivery")
	}
	if err := testHandler.processSupportThread(t.Context(), parseUUID(f.thread)); err != nil {
		t.Fatal(err)
	}
	var state string
	dbfx.QueryRow(t, `SELECT state FROM slack_intake_thread WHERE id=$1`, f.thread).Scan(&state)
	if posts != 1 || state != "answered" {
		t.Fatalf("posts=%d state=%s", posts, state)
	}
	dbfx.Exec(t, `UPDATE slack_intake_rule SET enabled=false WHERE id=$1`, f.rule)
	if err := testHandler.processSupportThread(t.Context(), parseUUID(f.thread)); err == nil {
		t.Fatal("disabled workflow processed")
	}
	if posts != 1 {
		t.Fatal("disabled workflow posted")
	}
}
