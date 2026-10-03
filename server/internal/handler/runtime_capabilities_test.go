package handler

import (
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/execution"
	"github.com/multica-ai/multica/server/pkg/runtimecap"
	"testing"
	"time"
)

func TestExecutionSelectsExactCapabilityMachineAndRevalidates(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "select", true: "offline"}[lost], func(t *testing.T) {
			aid, iid, r1, r2, p := executionFixture(t)
			p.Profiles = p.Profiles[:1]
			p.RouterProfile = "default"
			p.AllowFallback = false
			raw, _ := json.Marshal(p)
			testPool.Exec(t.Context(), "UPDATE agent SET execution_policy=$2 WHERE id=$1", aid, raw)
			testPool.Exec(t.Context(), "UPDATE agent_runtime SET provider='codex',metadata=(SELECT metadata FROM agent_runtime WHERE id=$2) WHERE id=$1", r2, r1)
			report := runtimecap.New("codex")
			report.Status = "reported"
			report.ObservedAt = time.Now()
			report.Add(runtimecap.Entry{Kind: "plugin", Name: "design@market", Enabled: runtimecap.Bool(true)})
			metadata, _ := json.Marshal(map[string]any{"runtime_capabilities": report})
			testPool.Exec(t.Context(), "UPDATE agent_runtime SET metadata=metadata || $2::jsonb WHERE id=$1", r2, metadata)
			tid := dbfx.Task(t, aid, testutil.Cols{"runtime_id": r1, "issue_id": iid})
			if e := testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(r1), parseUUID(r2)}); e != nil {
				t.Fatal(e)
			}
			task, _ := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(tid))
			sel := execution.ParseSelection(task.ExecutionSelection)
			if len(sel.Candidates) != 2 || sel.RuntimeCapabilities[r2] == nil {
				t.Fatal("lost per-machine inventory", sel)
			}
			routerID := uuidToString(task.RuntimeID)
			claimed, e := testHandler.TaskService.ClaimTaskForRuntime(t.Context(), task.RuntimeID)
			if e != nil || claimed == nil {
				t.Fatal("claim", e)
			}
			if lost {
				testPool.Exec(t.Context(), "UPDATE agent_runtime SET status='offline' WHERE id=$1", r2)
			}
			req := withURLParam(newDaemonTokenRequest("POST", "/resolve", map[string]any{"runtime_id": routerID, "dispatched_at": claimed.DispatchedAt.Time, "profile_id": "default", "selected_runtime_id": r2, "reason": "design plugin"}, testWorkspaceID, "capability-router"), "taskId", tid)
			testutil.Call(t, testHandler.ResolveTaskExecution, req).Want(200)
			task, _ = testHandler.Queries.GetAgentTask(t.Context(), parseUUID(tid))
			sel = execution.ParseSelection(task.ExecutionSelection)
			if lost {
				if sel.State != "blocked" {
					t.Fatal("offline choice accepted", sel)
				}
			} else if sel.State != "selected" || uuidToString(task.RuntimeID) != r2 {
				t.Fatal("runtime choice lost", sel)
			}
		})
	}
}
func TestExecutionCapabilityInventoryExcludesStaleAndCustomEnvironment(t *testing.T) {
	aid, _, r1, _, p := executionFixture(t)
	report := runtimecap.New("codex")
	report.Status = "reported"
	report.ObservedAt = time.Now()
	report.Add(runtimecap.Entry{Kind: "plugin", Name: "test"})
	put := func() {
		raw, _ := json.Marshal(map[string]any{"runtime_capabilities": report})
		testPool.Exec(t.Context(), "UPDATE agent_runtime SET metadata=metadata || $2::jsonb WHERE id=$1", r1, raw)
	}
	put()
	a, _ := testHandler.Queries.GetAgent(t.Context(), parseUUID(aid))
	check := func(want bool) {
		t.Helper()
		for _, c := range testHandler.TaskService.ExecutionCandidates(t.Context(), a, p) {
			if c.Profile.RuntimeID == r1 && (c.Capabilities != nil) != want {
				t.Fatal("inventory scope", c)
			}
		}
	}
	check(true)
	report.ObservedAt = time.Now().Add(-time.Hour)
	put()
	check(false)
	report.ObservedAt = time.Now()
	put()
	a.CustomEnv = []byte(`{"CODEX_HOME":"/other"}`)
	check(false)
}
func TestExecutionClaimGenerationRechecksMachineTools(t *testing.T) {
	aid, iid, r1, r2, p := executionFixture(t)
	p.Profiles[1].RequiredTools = []string{"git"}
	raw, _ := json.Marshal(p)
	testPool.Exec(t.Context(), "UPDATE agent SET execution_policy=$2 WHERE id=$1", aid, raw)
	tid := dbfx.Task(t, aid, testutil.Cols{"runtime_id": r1, "issue_id": iid})
	testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(r2)})
	task, e := testHandler.TaskService.ClaimTaskForRuntime(t.Context(), parseUUID(r2))
	if e != nil || task == nil || uuidToString(task.ID) != tid {
		t.Fatal("claim", e)
	}
	testPool.Exec(t.Context(), `UPDATE agent_runtime SET metadata=jsonb_set(metadata,'{tools}','[]') WHERE id=$1`, r2)
	_, e = testHandler.TaskService.StartTaskForClaim(t.Context(), db.LockAgentTaskStartClaimParams{ID: task.ID, RuntimeID: task.RuntimeID, DispatchedAt: task.DispatchedAt})
	if e == nil {
		t.Fatal("started after losing required tools")
	}
	after, _ := testHandler.Queries.GetAgentTask(t.Context(), task.ID)
	if after.Status != "queued" {
		t.Fatal("did not requeue stale machine", after.Status)
	}
}
