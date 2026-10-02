package handler

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	"net/http/httptest"
	"testing"
	"time"
)

func setupIntakeTest(t *testing.T) (string, string) {
	t.Helper()
	a := seedAllowListedAgent(t, "Intake coordinator", testUserID, "public_to")
	dbfx.InsertNoID(t, "agent_invocation_target", testutil.Cols{"agent_id": a, "target_type": "workspace", "target_id": testWorkspaceID}, "agent_id=$1", a)
	s := seedSquadForBriefing(t, a, "Intake squad", "")
	raw, e := testHandler.Queries.GetIssueIntake(t.Context(), parseUUID(testWorkspaceID))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "UPDATE workspace SET issue_intake=$2 WHERE id=$1", testWorkspaceID, raw)
	})
	return a, uuidToString(s.ID)
}
func saveIntake(t *testing.T, c map[string]any, want int) {
	t.Helper()
	w := httptest.NewRecorder()
	r := withURLParam(newRequest("PUT", "/issue-intake", c), "id", testWorkspaceID)
	testHandler.UpdateIssueIntake(w, r)
	if w.Code != want {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
}
func TestIssueIntakeRoutingAndExclusions(t *testing.T) {
	a, s := setupIntakeTest(t)
	saveIntake(t, map[string]any{"revision": 0, "default_squad_id": s, "projects": map[string]string{}}, 200)
	cases := []struct {
		name, creator, status             string
		parent, explicit, deferred, route bool
	}{{"simple", "member", "todo", false, false, false, true}, {"backlog", "member", "backlog", false, false, false, true}, {"subtask", "member", "todo", true, false, false, false}, {"agent generated", "agent", "todo", false, false, false, false}, {"assigned", "member", "todo", false, true, false, false}, {"terminal", "member", "done", false, false, false, false}, {"media", "member", "todo", false, false, true, true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := service.IssueCreateParams{WorkspaceID: parseUUID(testWorkspaceID), Title: "Intake " + tc.name, Status: tc.status, Priority: "medium", CreatorType: tc.creator, CreatorID: parseUUID(testUserID)}
			if tc.creator == "agent" {
				p.CreatorID = parseUUID(a)
			}
			if tc.parent {
				p.ParentIssueID = parseUUID(dbfx.Issue(t, "Intake parent", testutil.Cols{"number": -900001}))
			}
			if tc.explicit {
				p.AssigneeType = pgtype.Text{String: "member", Valid: true}
				p.AssigneeID = parseUUID(testUserID)
			}
			opts := service.IssueCreateOpts{}
			if tc.deferred {
				opts.AssignedAgentRunFireAt = time.Now().Add(time.Minute)
			}
			result, e := testHandler.IssueService.Create(t.Context(), p, opts)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() {
				testPool.Exec(context.Background(), "DELETE FROM agent_task_queue WHERE issue_id=$1", result.Issue.ID)
				testPool.Exec(context.Background(), "DELETE FROM issue WHERE id=$1", result.Issue.ID)
			})
			routed := result.Issue.AssigneeType.String == "squad"
			if routed != tc.route {
				t.Fatalf("wrong routing: %+v", result.Issue)
			}
			if tc.route && tc.status != "backlog" {
				if !result.AssignedTaskID.Valid {
					t.Fatal("no durable task")
				}
				var task struct {
					Status string
					Leader bool
					Squad  string
				}
				e = testPool.QueryRow(t.Context(), "SELECT status,is_leader_task,squad_id::text FROM agent_task_queue WHERE id=$1", result.AssignedTaskID).Scan(&task.Status, &task.Leader, &task.Squad)
				if e != nil || !task.Leader || task.Squad != s {
					t.Fatalf("task %+v %v", task, e)
				}
				want := "queued"
				if tc.deferred {
					want = "deferred"
				}
				if task.Status != want {
					t.Fatalf("status %s want %s", task.Status, want)
				}
			}
		})
	}
}
func TestIssueIntakeSettingsValidation(t *testing.T) {
	a, s := setupIntakeTest(t)
	saveIntake(t, map[string]any{"default_squad_id": "bad"}, 400)
	saveIntake(t, map[string]any{"projects": map[string]string{"bad": ""}}, 400)
	saveIntake(t, map[string]any{"revision": 0, "default_squad_id": s}, 200)
	saveIntake(t, map[string]any{"revision": 0, "default_squad_id": ""}, 409)
	testPool.Exec(context.Background(), "UPDATE agent SET permission_mode='private' WHERE id=$1", a)
	saveIntake(t, map[string]any{"revision": 1, "default_squad_id": s}, 400)
	saveIntake(t, map[string]any{"revision": 1, "default_squad_id": ""}, 200)
}
func TestIssueIntakeProjectOverrides(t *testing.T) {
	c, e := service.ParseIssueIntake([]byte(`{"default_squad_id":"default","projects":{"one":"other","two":""}}`))
	if e != nil {
		t.Fatal(e)
	}
	for id, want := range map[string]string{"one": "other", "two": "", "three": "default"} {
		if got := c.SquadFor(id); got != want {
			t.Fatal(got, want)
		}
	}
	raw, _ := json.Marshal(c)
	if len(raw) == 0 {
		t.Fatal("empty config")
	}
}
func TestIssueIntakeAdminOnly(t *testing.T) {
	_, _, user := runtimeVisibilityFixture(t)
	w := httptest.NewRecorder()
	r := withURLParam(newRequestAs(user, "PUT", "/issue-intake", map[string]any{"revision": 0}), "id", testWorkspaceID)
	testHandler.UpdateIssueIntake(w, r)
	if w.Code != 403 {
		t.Fatalf("non-admin: %d %s", w.Code, w.Body.String())
	}
}

func TestIssueIntakeCreatePreview(t *testing.T) {
	a, s := setupIntakeTest(t)
	project := dbfx.Project(t, "Manual intake project", nil)
	parent := dbfx.Issue(t, "Preview parent", testutil.Cols{"number": -900002})
	saveIntake(t, map[string]any{"revision": 0, "default_squad_id": s, "projects": map[string]string{project: ""}}, 200)
	cases := []struct {
		name  string
		body  map[string]any
		count int
	}{
		{"default", map[string]any{"is_create": true}, 1},
		{"backlog", map[string]any{"is_create": true, "status": "backlog"}, 0},
		{"project off", map[string]any{"is_create": true, "project_id": project}, 0},
		{"subtask", map[string]any{"is_create": true, "parent_issue_id": parent}, 0},
		{"member assigned", map[string]any{"is_create": true, "assignee_type": "member", "assignee_id": testUserID}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := previewIssueTrigger(t, tc.body)
			if got.TotalCount != tc.count {
				t.Fatalf("got %+v want %d", got, tc.count)
			}
			if tc.count > 0 && got.Triggers[0].AgentID != a {
				t.Fatalf("wrong coordinator: %+v", got)
			}
		})
	}
}

func TestIssueIntakeQuickCreateAuthorCannotBypass(t *testing.T) {
	a, squad := setupIntakeTest(t)
	saveIntake(t, map[string]any{"revision": 0, "default_squad_id": squad}, 200)
	task, taskErr := testHandler.TaskService.EnqueueQuickCreateTask(t.Context(), parseUUID(testWorkspaceID), parseUUID(testUserID), parseUUID(a), parseUUID(squad), "File a ticket", "", "", pgtype.UUID{}, pgtype.UUID{}, nil)
	if taskErr != nil {
		t.Fatal(taskErr)
	}
	taskID := uuidToString(task.ID)
	t.Cleanup(func() { testPool.Exec(context.Background(), "DELETE FROM agent_task_queue WHERE id=$1", task.ID) })
	p := service.IssueCreateParams{WorkspaceID: parseUUID(testWorkspaceID), Title: "AI-authored intake", Status: "todo", Priority: "medium", CreatorType: "agent", CreatorID: parseUUID(a), OriginType: pgtype.Text{String: "quick_create", Valid: true}, OriginID: parseUUID(taskID), AssigneeType: pgtype.Text{String: "agent", Valid: true}, AssigneeID: parseUUID(a)}
	result, err := testHandler.IssueService.Create(t.Context(), p, service.IssueCreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM agent_task_queue WHERE issue_id=$1", result.Issue.ID)
		testPool.Exec(context.Background(), "DELETE FROM issue WHERE id=$1", result.Issue.ID)
	})
	if result.Issue.AssigneeType.String != "squad" || uuidToString(result.Issue.AssigneeID) != squad || !result.AssignedTaskID.Valid {
		t.Fatalf("intake bypassed: %+v", result)
	}
	if result.Issue.CreatorType != "agent" || result.Issue.CreatorID != p.CreatorID {
		t.Fatal("authorship changed")
	}
	// An unrelated agent cannot borrow this human-requested origin to route work.
	p.CreatorID = parseUUID(testUserID)
	if err := testHandler.IssueService.ResolveIssueIntake(t.Context(), testHandler.Queries, &p, p.ProjectID); err != nil {
		t.Fatal(err)
	}
	if p.AssigneeType.String != "agent" {
		t.Fatal("untrusted origin accepted")
	}
}

func TestIssueIntakeQuickCreateSelectsCoordinator(t *testing.T) {
	a, squad := setupIntakeTest(t)
	saveIntake(t, map[string]any{"revision": 0, "default_squad_id": squad}, 200)
	// The selected author is deliberately malformed. Intake replaces it before resolution.
	w := httptest.NewRecorder()
	testHandler.QuickCreateIssue(w, newRequest("POST", "/api/issues/quick-create", map[string]any{"agent_id": "bypass-author", "prompt": "File this ticket"}))
	// The fixture runtime lacks a CLI version, so the coordinator's version gate
	// must reject it rather than trying to resolve the supplied author.
	if w.Code != 422 {
		t.Fatalf("expected coordinator readiness/version gate, got %d %s (leader %s)", w.Code, w.Body.String(), a)
	}
}
