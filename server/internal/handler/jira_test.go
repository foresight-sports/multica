package handler

import (
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/execution"
	"strings"
	"testing"
)

func TestJiraRequirementBlocksAndReleasesWork(t *testing.T) {
	aid, iid, rt, _, policy := executionFixture(t)
	q := testHandler.Queries
	ctx := t.Context()
	prop, err := q.GetJiraProperty(ctx, parseUUID(testWorkspaceID))
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), prop.Config...)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "UPDATE issue_property SET config=$2 WHERE id=$1", prop.ID, original)
	})
	var cfg jiraSettings
	json.Unmarshal(prop.Config, &cfg)
	_, err = q.SaveJiraRequirement(ctx, db.SaveJiraRequirementParams{WorkspaceID: parseUUID(testWorkspaceID), Required: true, Revision: cfg.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.SaveJiraRequirement(ctx, db.SaveJiraRequirementParams{WorkspaceID: parseUUID(testWorkspaceID), Required: false, Revision: cfg.Revision}); err == nil {
		t.Fatal("stale settings accepted")
	}
	taskID := dbfx.Task(t, aid, testutil.Cols{"runtime_id": rt, "issue_id": iid})
	task, _ := q.GetAgentTask(ctx, parseUUID(taskID))
	agent, _ := q.GetAgent(ctx, parseUUID(aid))
	candidates := testHandler.TaskService.ConstrainExecutionToTask(ctx, task, testHandler.TaskService.ExecutionCandidates(ctx, agent, policy))
	if _, _, err = execution.Select(policy, execution.Request{}, candidates, ""); err == nil {
		t.Fatal("missing JIRA routed")
	}
	if _, err = testPool.Exec(ctx, "UPDATE agent_task_queue SET status='dispatched' WHERE id=$1", taskID); err == nil {
		t.Fatal("missing JIRA bypassed dispatch gate")
	}
	key, err := normalizeJiraKey(" fs-123 ")
	if err != nil || key != "FS-123" {
		t.Fatal(key, err)
	}
	raw, _ := json.Marshal(key)
	if _, err = q.SetIssuePropertyValue(ctx, db.SetIssuePropertyValueParams{ID: parseUUID(iid), WorkspaceID: parseUUID(testWorkspaceID), Key: uuidToString(prop.ID), Value: raw}); err != nil {
		t.Fatal(err)
	}
	var childID string
	if err = testPool.QueryRow(ctx, `INSERT INTO issue(workspace_id,title,status,parent_issue_id,creator_type,creator_id) VALUES($1,'JIRA child','backlog',$2,'member',$3) RETURNING id`, testWorkspaceID, iid, testUserID).Scan(&childID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), "DELETE FROM issue WHERE id=$1", childID) })
	if inherited, e := q.GetIssueJiraKey(ctx, parseUUID(childID)); e != nil || inherited != "FS-123" {
		t.Fatal("child did not inherit", inherited, e)
	}
	if _, err = testPool.Exec(ctx, `UPDATE issue SET properties=jsonb_set(properties,ARRAY[$2::text],'"bad key"') WHERE id=$1`, childID, uuidToString(prop.ID)); err == nil {
		t.Fatal("database accepted malformed key")
	}
	allowed, err := q.TaskJiraAllowed(ctx, db.TaskJiraAllowedParams{Column1: task.IssueID, Column2: task.AgentID, Column3: task.Context})
	if err != nil || !allowed {
		t.Fatal("valid key remained blocked", err)
	}
	runtime, _ := q.GetAgentRuntime(ctx, parseUUID(rt))
	profile := policy.Profiles[0]
	profile.RuntimeID = rt
	selection, _ := json.Marshal(execution.Selection{State: "selected", Profile: profile})
	testPool.Exec(ctx, "UPDATE agent_task_queue SET execution_selection=$2 WHERE id=$1", taskID, selection)
	task, _ = q.GetAgentTask(ctx, parseUUID(taskID))
	req := newRequest("POST", "/claim", nil)
	req.Header.Set("X-Client-Capabilities", "workspace-repository-v1,jira-ticket-v1")
	resp, _, _, _, failure := testHandler.buildClaimedTaskResponse(req, &task, runtime, rt, testWorkspaceID)
	if failure != nil || resp.JiraTicketID != "FS-123" {
		t.Fatalf("claim context: %s %+v", resp.JiraTicketID, failure)
	}
	if _, err = testPool.Exec(ctx, "UPDATE agent_task_queue SET status='dispatched' WHERE id=$1", taskID); err != nil {
		t.Fatal(err)
	}
	if _, err = q.DeleteIssuePropertyValue(ctx, db.DeleteIssuePropertyValueParams{ID: parseUUID(iid), WorkspaceID: parseUUID(testWorkspaceID), Key: uuidToString(prop.ID)}); err == nil {
		t.Fatal("changed tracking key while dispatched")
	}
	// A setting changed between dispatch and start is checked again.
	testPool.Exec(ctx, "UPDATE agent_task_queue SET status='queued' WHERE id=$1", taskID)
	q.DeleteIssuePropertyValue(ctx, db.DeleteIssuePropertyValueParams{ID: parseUUID(iid), WorkspaceID: parseUUID(testWorkspaceID), Key: uuidToString(prop.ID)})
	if _, err = testHandler.TaskService.StartTask(ctx, parseUUID(taskID)); err == nil || !strings.Contains(err.Error(), "JIRA") {
		t.Fatal("start did not reject missing JIRA", err)
	}
}

func TestJiraKeyValidation(t *testing.T) {
	for _, bad := range []string{"", "123", "FS-0", "FS-12; echo bad", "https://jira/FS-12"} {
		if _, err := normalizeJiraKey(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestJiraCreationIncludesKeyBeforeIntake(t *testing.T) {
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
