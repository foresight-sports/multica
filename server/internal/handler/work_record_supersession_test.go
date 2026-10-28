package handler

import (
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/workflow"
	"net/http/httptest"
	"testing"
)

func TestDecisionSupersessionAtomicAndHumanProtected(t *testing.T) {
	aid, iid, rt, _, _ := executionFixture(t)
	task := dbfx.Task(t, aid, testutil.Cols{"runtime_id": rt, "issue_id": iid, "status": "running", "is_leader_task": true})
	t.Cleanup(func() { testPool.Exec(context.Background(), "DELETE FROM work_record WHERE issue_id=$1", iid) })
	create := func(agent string, data workflow.Data, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.WorkRecords(w, workRequest("POST", iid, "", agent, task, workRecordRequest{Kind: "decision", Data: data}))
		if w.Code != want {
			t.Fatalf("got %d: %s", w.Code, w.Body.String())
		}
		var row struct{ ID string }
		_ = json.Unmarshal(w.Body.Bytes(), &row)
		return row.ID
	}
	human := create("", workflow.Data{Text: "human decision"}, 201)
	own := create(aid, workflow.Data{Text: "old diagnosis"}, 201)
	create(aid, workflow.Data{Text: "invalid replacement", Supersedes: []string{own, human}}, 403)
	var state string
	if err := testPool.QueryRow(t.Context(), "SELECT state FROM work_record WHERE id=$1", own).Scan(&state); err != nil || state != "open" {
		t.Fatal("partial supersession committed", err, state)
	}
	create(aid, workflow.Data{Text: "corrected diagnosis", Supersedes: []string{own}}, 201)
	if err := testPool.QueryRow(t.Context(), "SELECT state FROM work_record WHERE id=$1", own).Scan(&state); err != nil || state != "resolved" {
		t.Fatal("obsolete record still active", err, state)
	}
}
