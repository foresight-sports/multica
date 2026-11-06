package handler

import (
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/execution"
	"net/http/httptest"
	"testing"
)

func TestJiraRequirementOnlyBlocksPullRequests(t *testing.T) {
	aid, iid, rt, _, policy := executionFixture(t)
	q := testHandler.Queries
	ctx := t.Context()
	prop, err := q.GetJiraProperty(ctx, parseUUID(testWorkspaceID))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "UPDATE issue_property SET config=$2 WHERE id=$1", prop.ID, prop.Config)
	})
	var cfg jiraSettings
	json.Unmarshal(prop.Config, &cfg)
	_, err = q.SaveJiraRequirement(ctx, db.SaveJiraRequirementParams{WorkspaceID: parseUUID(testWorkspaceID), Required: true, Revision: cfg.Revision})
	if err != nil {
		t.Fatal(err)
	}
	taskID := dbfx.Task(t, aid, testutil.Cols{"runtime_id": rt, "issue_id": iid})
	task, _ := q.GetAgentTask(ctx, parseUUID(taskID))
	agent, _ := q.GetAgent(ctx, parseUUID(aid))
	candidates := testHandler.TaskService.ConstrainExecutionToTask(ctx, task, testHandler.TaskService.ExecutionCandidates(ctx, agent, policy))
	if _, _, err = execution.Select(policy, execution.Request{}, candidates, ""); err != nil {
		t.Fatal("investigation blocked", err)
	}
	if _, err = testPool.Exec(ctx, "UPDATE agent_task_queue SET status='dispatched' WHERE id=$1", taskID); err != nil {
		t.Fatal("dispatch blocked", err)
	}
	check := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.CheckIssuePullRequest(w, withURLParam(newRequest("GET", "/pull-request-check", nil), "id", iid))
		if w.Code != want {
			t.Fatalf("PR check %d: %s", w.Code, w.Body.String())
		}
	}
	check(409)
	// The first key can be supplied while investigation is active.
	_, err = q.SetIssuePropertyValue(ctx, db.SetIssuePropertyValueParams{ID: parseUUID(iid), WorkspaceID: parseUUID(testWorkspaceID), Key: uuidToString(prop.ID), Value: []byte(`"FS-123"`)})
	if err != nil {
		t.Fatal(err)
	}
	check(200)
	if _, err = q.DeleteIssuePropertyValue(ctx, db.DeleteIssuePropertyValueParams{ID: parseUUID(iid), WorkspaceID: parseUUID(testWorkspaceID), Key: uuidToString(prop.ID)}); err == nil {
		t.Fatal("removed existing key during run")
	}
	testPool.Exec(ctx, "UPDATE agent_task_queue SET status='queued' WHERE id=$1", taskID)
	q.DeleteIssuePropertyValue(ctx, db.DeleteIssuePropertyValueParams{ID: parseUUID(iid), WorkspaceID: parseUUID(testWorkspaceID), Key: uuidToString(prop.ID)})
	testPool.Exec(ctx, "UPDATE issue_property SET config=jsonb_set(config,'{required}','false') WHERE id=$1", prop.ID)
	check(200)
}

func TestJiraKeyValidation(t *testing.T) {
	for _, bad := range []string{"", "123", "FS-0", "FS-12; echo bad", "https://jira/FS-12"} {
		if _, err := normalizeJiraKey(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestJiraCreationIncludesKeyBeforeIntake(t *testing.T) {
	dbfx.InsertNoID(t, "repository_configuration", testutil.Cols{"scope": "workspace", "subject": testWorkspaceID, "config": testutil.Raw(`'{"repository":"example/repo","folder":"repo","mode":"worktree"}'::jsonb`)}, "scope='workspace' AND subject=$1", testWorkspaceID)
	_, squad := setupIntakeTest(t)
	saveIntake(t, map[string]any{"revision": 0, "default_squad_id": squad}, 200)
	result, err := testHandler.IssueService.Create(t.Context(), service.IssueCreateParams{
		WorkspaceID: parseUUID(testWorkspaceID), Title: "Tracked intake", Status: "todo",
		Priority: "medium", CreatorType: "member", CreatorID: parseUUID(testUserID), JiraTicketID: "FS-321",
	}, service.IssueCreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM agent_task_queue WHERE issue_id=$1", result.Issue.ID)
		testPool.Exec(context.Background(), "DELETE FROM issue WHERE id=$1", result.Issue.ID)
	})
	if !result.AssignedTaskID.Valid {
		t.Fatal("missing intake task")
	}
	key, err := testHandler.Queries.GetIssueJiraKey(t.Context(), result.Issue.ID)
	if err != nil || key != "FS-321" {
		t.Fatal("creation lost JIRA key", key, err)
	}
}
