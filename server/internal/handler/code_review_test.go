package handler

import (
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/internal/integrations/ghsnapshot"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"testing"
)

type reviewStub struct {
	requests int
	state    string
}

func (*reviewStub) Enabled() bool { return true }
func (*reviewStub) ReviewPull(context.Context, int64, string, string, int32) (ghsnapshot.ReviewPull, error) {
	p := ghsnapshot.ReviewPull{State: "open"}
	p.Head.SHA = "head"
	return p, nil
}
func (s *reviewStub) Reviews(_ context.Context, _ int64, _, _ string, _ int32, comments bool) ([]ghsnapshot.Review, error) {
	if comments {
		return nil, nil
	}
	r := ghsnapshot.Review{ID: 1, State: s.state, Body: "fix it", CommitID: "head"}
	r.User.Login = "bot"
	return []ghsnapshot.Review{r}, nil
}
func (s *reviewStub) RequestReviewers(context.Context, int64, string, string, int32, string, []string, []string, []string) error {
	s.requests++
	return nil
}
func TestCodeReviewReconciliation(t *testing.T) {
	if testHandler == nil {
		t.Skip("DB required")
	}
	ctx := t.Context()
	h := *testHandler
	stub := &reviewStub{state: "APPROVED"}
	h.ReviewClient = stub
	agent := createHandlerTestAgent(t, "Review followup", []byte("[]"))
	issue := dbfx.Issue(t, "Review delivery", testutil.Cols{"status": "in_review", "assignee_type": "agent", "assignee_id": agent, "creator_type": "member", "creator_id": testUserID})
	linkMergedGitHubPRForTest(t, issue, "review-sync")
	id := parseUUID(githubPRIDForTest(t, "review-sync", 1))
	dbfx.Exec(t, "UPDATE github_pull_request SET state='open',head_sha='head' WHERE id=$1", id)
	_, e := h.Queries.CreateGitHubInstallation(ctx, db.CreateGitHubInstallationParams{WorkspaceID: parseUUID(testWorkspaceID), InstallationID: 1, AccountLogin: "acme", AccountType: "Organization"})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM repository_configuration WHERE (scope='code_delivery' AND subject=$1) OR (scope IN ('code_review_receipt','code_review_scan') AND subject LIKE $2)", testWorkspaceID, uuidToString(id)+"%")
		testPool.Exec(context.Background(), "DELETE FROM agent_task_queue WHERE issue_id=$1", issue)
		testPool.Exec(context.Background(), "DELETE FROM github_installation WHERE workspace_id=$1 AND installation_id=1", testWorkspaceID)
	})
	policy := workspacerepo.DeliverySettings{Default: workspacerepo.Delivery{Mode: "pull_request", AIReviewers: []string{"bot"}, HumanReviewers: []string{"human"}}}
	raw, _ := json.Marshal(policy)
	_, e = h.Queries.SaveRepositoryConfiguration(ctx, db.SaveRepositoryConfigurationParams{Scope: "code_delivery", Subject: testWorkspaceID, Config: raw})
	if e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 2; n++ {
		if e = h.syncCodeReview(ctx, id); e != nil {
			t.Fatal(e)
		}
	}
	if stub.requests != 1 {
		t.Fatalf("human requests %d", stub.requests)
	}
	stub.state = "CHANGES_REQUESTED"
	_, otherOwner, _ := privateAgentTestFixture(t)
	dbfx.Exec(t, "UPDATE agent SET owner_id=$2 WHERE id=$1", agent, otherOwner)
	dbfx.Exec(t, "DELETE FROM agent_invocation_target WHERE agent_id=$1", agent)
	if e = h.syncCodeReview(ctx, id); e == nil {
		t.Fatal("review bypassed invocation permission")
	}
	dbfx.Exec(t, "UPDATE agent SET owner_id=$2 WHERE id=$1", agent, testUserID)
	dbfx.Exec(t, "INSERT INTO agent_invocation_target(agent_id,target_type,target_id) VALUES($1,'workspace',$2)", agent, testWorkspaceID)

	if e = h.syncCodeReview(ctx, id); e != nil {
		t.Fatal(e)
	}
	if e = h.syncCodeReview(ctx, id); e != nil {
		t.Fatal(e)
	}
	var count int
	dbfx.QueryRow(t, "SELECT count(*) FROM agent_task_queue WHERE issue_id=$1 AND handoff_note LIKE 'GitHub review feedback receipt %'", issue).Scan(&count)
	if count != 1 {
		t.Fatalf("feedback runs %d", count)
	}
	if e = h.syncCodeReviews(ctx); e != nil {
		t.Fatal(e)
	}
	// Reconciliation itself must not complete issues. Only the existing merge
	// transition does, and it still waits for every linked PR.
	if issueStatusForTest(t, issue) == "done" {
		t.Fatal("review completed issue")
	}
	linkMergedGitHubPRForTest(t, issue, "review-sync-sibling")
	sibling := parseUUID(githubPRIDForTest(t, "review-sync-sibling", 1))
	dbfx.Exec(t, "UPDATE github_pull_request SET state='open' WHERE id=$1", sibling)
	dbfx.Exec(t, "UPDATE github_pull_request SET state='merged' WHERE id=$1", id)
	h.maybeAutoCompleteIssue(ctx, parseUUID(testWorkspaceID), parseUUID(issue), issuestatus.NewResolver(parseUUID(testWorkspaceID)))
	if issueStatusForTest(t, issue) == "done" {
		t.Fatal("completed before sibling PR merged")
	}
	dbfx.Exec(t, "UPDATE github_pull_request SET state='merged' WHERE id=$1", sibling)
	h.maybeAutoCompleteIssue(ctx, parseUUID(testWorkspaceID), parseUUID(issue), issuestatus.NewResolver(parseUUID(testWorkspaceID)))
	if issueStatusForTest(t, issue) != "done" {
		t.Fatal("merge did not complete issue")
	}
}
