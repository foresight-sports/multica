package handler

import (
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIssueBaselineSharedAndProvenance(t *testing.T) {
	aid, iid, rt, other, _ := executionFixture(t)
	old, err := testHandler.Queries.GetRepositoryConfiguration(t.Context(), db.GetRepositoryConfigurationParams{Scope: "workspace", Subject: testWorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = testPool.Exec(t.Context(), `UPDATE repository_configuration SET config='{"repository":"org/repo","folder":"repo","mode":"worktree","base_branch":"dev"}' WHERE scope='workspace' AND subject=$1`, testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE repository_configuration SET config=$2 WHERE scope='workspace' AND subject=$1`, testWorkspaceID, old.Config)
		testPool.Exec(context.Background(), `DELETE FROM repository_configuration WHERE scope IN ('issue_baseline','task_source') AND subject LIKE $1`, testWorkspaceID+":"+iid+":%")
	})
	one := dbfx.Task(t, aid, testutil.Cols{"runtime_id": rt, "issue_id": iid, "status": "dispatched"})

	call := func(task string, body workspacerepo.Baseline, want int) workspacerepo.Baseline {
		t.Helper()
		r := withURLParam(newRequest("POST", "/repository-baseline", body), "taskId", task)
		w := httptest.NewRecorder()
		testHandler.PinTaskRepositoryBaseline(w, r)
		if w.Code != want {
			t.Fatalf("got %d: %s", w.Code, w.Body.String())
		}
		var got workspacerepo.Baseline
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		return got
	}
	candidate := workspacerepo.Baseline{Repository: "org/repo", Branch: "dev", Commit: strings.Repeat("a", 40), Revision: old.Revision}
	first := call(one, candidate, 200)
	_, err = testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='completed' WHERE id=$1`, one)
	if err != nil {
		t.Fatal(err)
	}
	two := dbfx.Task(t, aid, testutil.Cols{"runtime_id": other, "issue_id": iid, "status": "dispatched"})
	candidate.Commit = strings.Repeat("b", 40)
	second := call(two, candidate, 200)
	if first.Commit != second.Commit {
		t.Fatal("machines got different source commits")
	}
	candidate.Snapshot = strings.Repeat("c", 40)
	call(two, candidate, 409)
	candidate.Commit = first.Commit
	// A daemon can finish preparing while its claim is still waiting for the
	// local directory. Saving provenance precedes the task's start transition.
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='waiting_local_directory' WHERE id=$1`, two); err != nil {
		t.Fatal(err)
	}
	call(two, candidate, 200)
	rows, err := testHandler.Queries.ListIssueRepositorySources(t.Context(), testWorkspaceID+":"+iid)
	if err != nil || len(rows) != 1 {
		t.Fatalf("source records: %v %v", rows, err)
	}
	candidate.Revision++
	call(two, candidate, 409)
	candidate.Revision--
	for _, status := range []string{"queued", "completed", "failed", "cancelled"} {
		if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status=$2 WHERE id=$1`, two, status); err != nil {
			t.Fatal(err)
		}
		call(two, candidate, 409)
	}
}
