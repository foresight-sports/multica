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
	var previous []byte
	dbfx.QueryRow(t, `SELECT issue_intake FROM workspace WHERE id=$1`, testWorkspaceID).Scan(&previous)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE workspace SET issue_intake=$2 WHERE id=$1`, testWorkspaceID, previous)
	})
	squad := dbfx.Squad(t, "Support intake", f.coord)
	dbfx.Exec(t, `UPDATE workspace SET issue_intake=jsonb_build_object('revision',1,'default_squad_id',$2::text) WHERE id=$1`, testWorkspaceID, squad)
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
func TestSlackSupportCoordinatorChoosesDelegationAndWaits(t *testing.T) {
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
	payload := map[string]any{"installation_id": f.install, "channel_id": "CNEW001", "coordinator_id": f.writer, "analyst_id": "invalid-legacy-value", "composer_id": "invalid-legacy-value", "workflow": "support_diagnosis", "escalation_user_id": "UKODY01", "enabled": true, "revision": 0, "timeout_minutes": 60}
	call := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.SlackIntakeRules(w, withURLParam(newRequest("PUT", "/intake", payload), "id", testWorkspaceID))
		if w.Code != want {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	call(200)
	var routed string
	dbfx.QueryRow(t, `SELECT coordinator_id::text FROM slack_intake_rule WHERE workspace_id=$1 AND channel_id='CNEW001'`, testWorkspaceID).Scan(&routed)
	if routed != f.coord {
		t.Fatal("Slack payload overrode workspace intake coordinator")
	}
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
	task := dbfx.Task(t, f.coord, testutil.Cols{"issue_id": f.issue, "status": "completed", "runtime_id": testRuntimeID, "originator_user_id": testUserID, "accountable_user_id": testUserID})
	reply := dbfx.Insert(t, "slack_intake_reply", testutil.Cols{"task_id": task, "id": uuidToString(dbid.NewV7()), "thread_id": f.thread, "revision": 1, "outcome": "answer", "confidence": .95, "body": "Finding", "evidence": "Evidence"})
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

func TestSlackSupportNoFixedAnalystWriterOrAudience(t *testing.T) {
	f := newSupportFixture(t)
	// Legacy selections can be unavailable: the coordinator owns replacement/delegation decisions.
	dbfx.Exec(t, `UPDATE agent SET archived_at=now() WHERE id IN($1,$2)`, f.analyst, f.writer)
	dbfx.Exec(t, `UPDATE slack_intake_rule SET coordinator_id=$2,audience='sales',style_instructions='obsolete channel wording' WHERE id=$1`, f.rule, f.writer)
	current, err := testHandler.loadSupportThread(t.Context(), parseUUID(f.thread))
	if err != nil {
		t.Fatal(err)
	}
	if current.Rule.CoordinatorID != f.coord || current.Rule.AnalystID != "" || current.Rule.ComposerID != "" || current.Rule.Audience != "" || current.Rule.StyleInstructions != "" {
		t.Fatal("legacy channel settings still control loop")
	}
	task := dbfx.Task(t, f.coord, testutil.Cols{"issue_id": f.issue, "status": "running", "runtime_id": testRuntimeID, "originator_user_id": testUserID, "accountable_user_id": testUserID})
	w := httptest.NewRecorder()
	r := withURLParam(newRequest("POST", "/support-reply", map[string]any{"revision": 1, "outcome": "clarify", "confidence": .95, "body": "Which app version reproduced the issue?", "evidence": "The report lacks its app version; coordinator identified the specific missing fact."}), "id", f.issue)
	r.Header.Set("X-Actor-Source", "task_token")
	r.Header.Set("X-Task-ID", task)
	r.Header.Set("X-Agent-ID", f.coord)
	testHandler.SupportReply(w, r)
	if w.Code != 200 {
		t.Fatalf("coordinator required fixed agents: %d %s", w.Code, w.Body.String())
	}
}
func TestSlackSupportMissingIntakeDoesNotUseLegacyAgent(t *testing.T) {
	f := newSupportFixture(t)
	dbfx.Exec(t, `UPDATE workspace SET issue_intake='{}' WHERE id=$1`, testWorkspaceID)
	if _, err := testHandler.loadSupportThread(t.Context(), parseUUID(f.thread)); err == nil {
		t.Fatal("missing coordinator fell back to channel agent")
	}
}

func TestSlackSupportPartialDefectConfidence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		score   float64
		pending bool
		want    int
		backlog bool
	}{
		{"confirmed partial finding", .95, false, 200, true},
		{"uncertain defect", .7, false, 200, false},
		{"delegation still running", .95, true, 409, false},
		{"invalid score", 1.1, false, 400, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSupportFixture(t)
			task := dbfx.Task(t, f.coord, testutil.Cols{"issue_id": f.issue, "status": "running", "originator_user_id": testUserID, "accountable_user_id": testUserID, "runtime_id": testRuntimeID})
			if tc.pending {
				dbfx.Task(t, f.analyst, testutil.Cols{"issue_id": f.issue, "status": "running", "originator_user_id": testUserID, "accountable_user_id": testUserID, "runtime_id": testRuntimeID})
			}
			call := func() {
				w := httptest.NewRecorder()
				r := withURLParam(newRequest("POST", "/support-reply", map[string]any{"revision": 1, "outcome": "defect", "confidence": .7, "defect_confidence": tc.score, "defect_evidence": "Source confirms dropped disconnect notifications; not hardware reproduced.", "body": "Connection status can remain stale. What time were the missing shots taken?", "evidence": "The foreground shot loss remains unexplained."}), "id", f.issue)
				r.Header.Set("X-Actor-Source", "task_token")
				r.Header.Set("X-Task-ID", task)
				r.Header.Set("X-Agent-ID", f.coord)
				testHandler.SupportReply(w, r)
				if w.Code != tc.want {
					t.Fatalf("want %d got %d: %s", tc.want, w.Code, w.Body.String())
				}
			}
			call()
			if tc.want != 200 {
				return
			}
			call() // An identical retry must not create a second backlog item or reply.
			var outcome, body, evidence string
			dbfx.QueryRow(t, `SELECT outcome,body,evidence FROM slack_intake_reply WHERE thread_id=$1`, f.thread).Scan(&outcome, &body, &evidence)
			if outcome != "escalate" {
				t.Fatal("unresolved diagnosis did not escalate")
			}
			var backlog string
			dbfx.QueryRow(t, `SELECT COALESCE(backlog_issue_id::text,'') FROM slack_intake_thread WHERE id=$1`, f.thread).Scan(&backlog)
			if (backlog != "") != tc.backlog {
				t.Fatalf("unexpected backlog %q", backlog)
			}
			if tc.backlog {
				t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id=$1`, backlog) })
				if !strings.Contains(body, "What time") || !strings.Contains(body, "backlog") || !strings.Contains(evidence, "Defect confidence: 0.95") {
					t.Fatal("lost reviewed findings or confidence audit")
				}
				var status, description string
				dbfx.QueryRow(t, `SELECT status,description FROM issue WHERE id=$1`, backlog).Scan(&status, &description)
				if status != "backlog" || strings.Contains(description, "foreground shot loss") {
					t.Fatal("backlog mixed uncertain symptoms with confirmed defect")
				}
				var tasks int
				dbfx.QueryRow(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id=$1`, backlog).Scan(&tasks)
				if tasks != 0 {
					t.Fatal("support started implementation")
				}
			} else if strings.Contains(body, "Connection status") {
				t.Fatal("uncertain finding published")
			}
		})
	}
}

func TestSlackSupportReviewedClarification(t *testing.T) {
	for _, score := range []float64{.95, .6} {
		t.Run(fmt.Sprint(score), func(t *testing.T) {
			f := newSupportFixture(t)
			task := dbfx.Task(t, f.coord, testutil.Cols{"issue_id": f.issue, "status": "running", "originator_user_id": testUserID, "accountable_user_id": testUserID, "runtime_id": testRuntimeID})
			w := httptest.NewRecorder()
			r := withURLParam(newRequest("POST", "/support-reply", map[string]any{"revision": 1, "outcome": "clarify", "confidence": .7, "response_confidence": score, "body": "Retry succeeded in the recorded sessions. Did the latest retry fail?", "evidence": "Five retries succeeded in logs; the latest reported failure is not captured."}), "id", f.issue)
			r.Header.Set("X-Actor-Source", "task_token")
			r.Header.Set("X-Task-ID", task)
			r.Header.Set("X-Agent-ID", f.coord)
			testHandler.SupportReply(w, r)
			if w.Code != 200 {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			var result struct {
				Outcome, Body string
				DeliveryState string `json:"delivery_state"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Outcome != "escalate" || result.DeliveryState != "pending" {
				t.Fatalf("wrong receipt: %+v", result)
			}
			if strings.Contains(result.Body, "Did the latest retry fail?") != (score >= .9) {
				t.Fatal("clarification confidence not enforced")
			}
			var body string
			dbfx.QueryRow(t, `SELECT body FROM slack_intake_reply WHERE thread_id=$1`, f.thread).Scan(&body)
			if result.Body != body {
				t.Fatal("receipt does not match actual queued wording")
			}
		})
	}
}

func TestSlackSupportDeliveryMovesIssueToReview(t *testing.T) {
	for _, tc := range []struct {
		name, status, outcome string
		newer                 bool
		want                  string
	}{
		{"escalation", "in_progress", "escalate", false, "in_review"},
		{"clarification", "in_progress", "clarify", false, "in_review"},
		{"answer awaits review", "in_progress", "answer", false, "in_review"},
		{"closed by human", "done", "escalate", false, "done"},
		{"blocked by human", "blocked", "escalate", false, "blocked"},
		{"new conversation", "in_progress", "escalate", true, "in_progress"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSupportFixture(t)
			dbfx.Exec(t, `UPDATE issue SET status=$2 WHERE id=$1`, f.issue, tc.status)
			dbfx.Insert(t, "slack_intake_reply", testutil.Cols{"id": uuidToString(dbid.NewV7()), "thread_id": f.thread, "revision": 1, "outcome": tc.outcome, "confidence": .7, "body": "Reviewed response", "evidence": "Reviewed evidence", "state": "sent"})
			current, err := testHandler.loadSupportThread(t.Context(), parseUUID(f.thread))
			if err != nil {
				t.Fatal(err)
			}
			if tc.newer {
				dbfx.Exec(t, `UPDATE slack_intake_thread SET revision=2,state='diagnosing' WHERE id=$1`, f.thread)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"ok":true}`))
			}))
			defer srv.Close()
			prior := testHandler.SlackSupport
			testHandler.SlackSupport = &slack.SupportClient{APIURL: srv.URL + "/"}
			defer func() { testHandler.SlackSupport = prior }()
			for n := 0; n < 2; n++ {
				if err := testHandler.finishSupportReaction(t.Context(), current, tc.outcome); err != nil {
					t.Fatal(err)
				}
			}
			var status, state string
			dbfx.QueryRow(t, `SELECT i.status,t.state FROM issue i JOIN slack_intake_thread t ON t.issue_id=i.id WHERE t.id=$1`, f.thread).Scan(&status, &state)
			if status != tc.want {
				t.Fatalf("want %s got %s", tc.want, status)
			}
			if tc.newer && state != "diagnosing" {
				t.Fatal("old delivery overwrote new conversation")
			}
		})
	}
}
