package handler

import (
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/execution"
	"github.com/multica-ai/multica/server/pkg/quota"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionQuotaRoutingAndMonotonicReports(t *testing.T) {
	agentID, issueID, aRT, bRT, p := executionFixture(t)
	used := 100.0
	r := quota.Report{Provider: "codex", Status: "reported", ObservedAt: time.Now().UTC(), Source: "codex_app_server", Windows: []quota.Window{{ID: "codex:0", UsedPercent: &used, AppliesAll: true, ResetsAt: time.Now().Add(time.Hour).Unix()}}}
	raw, _ := json.Marshal(r)
	if e := testHandler.Queries.SetSubscriptionQuota(t.Context(), db.SetSubscriptionQuotaParams{ID: parseUUID(bRT), Report: raw}); e != nil {
		t.Fatal(e)
	}
	r.ObservedAt = r.ObservedAt.Add(-time.Minute)
	r.Windows = nil
	old, _ := json.Marshal(r)
	if e := testHandler.Queries.SetSubscriptionQuota(t.Context(), db.SetSubscriptionQuotaParams{ID: parseUUID(bRT), Report: old}); e != nil {
		t.Fatal(e)
	}
	a, _ := testHandler.Queries.GetAgent(t.Context(), parseUUID(agentID))
	c := testHandler.TaskService.ExecutionCandidates(t.Context(), a, p)
	if c[1].Eligible || !c[0].Eligible {
		t.Fatalf("eligibility %+v", c)
	}
	taskID := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": aRT, "issue_id": issueID})
	if e := testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(aRT), parseUUID(bRT)}); e != nil {
		t.Fatal(e)
	}
	task, _ := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
	sel := execution.ParseSelection(task.ExecutionSelection)
	if sel.Profile.RuntimeID != aRT {
		t.Fatalf("exhausted route: %+v", sel)
	}
	sel.Explicit = true
	selected, _ := json.Marshal(sel)
	if _, e := testPool.Exec(t.Context(), "UPDATE agent_task_queue SET execution_selection=$2 WHERE id=$1", taskID, selected); e != nil {
		t.Fatal(e)
	}
	if e := testHandler.Queries.SetSubscriptionQuota(t.Context(), db.SetSubscriptionQuotaParams{ID: parseUUID(aRT), Report: raw}); e != nil {
		t.Fatal(e)
	}
	if e := testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(aRT)}); e != nil {
		t.Fatal(e)
	}
	task, _ = testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
	sel = execution.ParseSelection(task.ExecutionSelection)
	if sel.State != "blocked" || sel.Profile.RuntimeID != aRT {
		t.Fatalf("explicit not pinned: %+v", sel)
	}
	claimed, e := testHandler.TaskService.ClaimTaskForRuntime(t.Context(), parseUUID(aRT))
	if e != nil || claimed != nil {
		t.Fatalf("exhausted claim %v %v", claimed, e)
	}
}

func TestSubscriptionQuotaEndpointValidation(t *testing.T) {
	_, _, rt, _, _ := executionFixture(t)
	if _, e := testPool.Exec(t.Context(), "UPDATE agent_runtime SET provider='codex' WHERE id=$1", rt); e != nil {
		t.Fatal(e)
	}
	r := quota.Report{Provider: "codex", Source: "codex_app_server", Status: "reported", ObservedAt: time.Now().UTC(), Windows: []quota.Window{}}
	request := func(report quota.Report) {
		req := withURLParam(newDaemonTokenRequest("POST", "/quota", report, testWorkspaceID, "quota-test"), "runtimeId", rt)
		testutil.Call(t, testHandler.ReportSubscriptionQuota, req).Want(200)
	}
	request(r)
	r.Provider = "claude"
	r.Source = "claude_stream"
	req := withURLParam(newDaemonTokenRequest("POST", "/quota", r, testWorkspaceID, "quota-test"), "runtimeId", rt)
	testutil.Call(t, testHandler.ReportSubscriptionQuota, req).Want(400)
	req = withURLParam(newRequest("GET", "/quota", nil), "runtimeId", rt)
	var result quota.Report
	testutil.Call(t, testHandler.GetSubscriptionQuota, req).Want(200).JSON(&result)
	if result.AccountKey != "" || result.Status != "reported" {
		t.Fatalf("unexpected response %+v", result)
	}
}

func TestSubscriptionQuotaAccountSharing(t *testing.T) {
	_, _, aRT, bRT, _ := executionFixture(t)
	if _, e := testPool.Exec(t.Context(), "UPDATE agent_runtime SET provider='codex' WHERE id IN ($1,$2)", aRT, bRT); e != nil {
		t.Fatal(e)
	}
	r := quota.Report{Provider: "codex", Source: "codex_app_server", Status: "reported", AccountKey: strings.Repeat("a", 64), ObservedAt: time.Now().UTC().Add(-time.Minute), Windows: []quota.Window{}}
	write := func(id string) {
		raw, _ := json.Marshal(r)
		if e := testHandler.Queries.SetSubscriptionQuota(t.Context(), db.SetSubscriptionQuotaParams{ID: parseUUID(id), Report: raw}); e != nil {
			t.Fatal(e)
		}
	}
	write(aRT)
	r.ObservedAt = time.Now().UTC()
	r.Windows = []quota.Window{{ID: "codex:0", Exhausted: true, AppliesAll: true}}
	write(bRT)
	raw, e := testHandler.Queries.GetSubscriptionQuota(t.Context(), parseUUID(aRT))
	if e != nil || !quota.Parse(raw).Blocks("m", time.Now()) {
		t.Fatal("shared account not reconciled", e)
	}
	r.AccountKey = strings.Repeat("b", 64)
	r.ObservedAt = time.Now().UTC()
	write(bRT)
	raw, e = testHandler.Queries.GetSubscriptionQuota(t.Context(), parseUUID(aRT))
	if e != nil || quota.Parse(raw).Blocks("m", time.Now()) {
		t.Fatal("different accounts shared", e)
	}
}
