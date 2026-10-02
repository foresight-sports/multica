package handler

import (
	"encoding/json"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/execution"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestPortableQuickCreateUsesEligibleMachine(t *testing.T) {
	agentID, _, aRT, bRT, p := executionFixture(t)
	a, err := testHandler.Queries.GetAgent(t.Context(), parseUUID(agentID))
	if err != nil || a.RuntimeID.Valid {
		t.Fatalf("fixture must be portable: %v", err)
	}
	setVersion := func(id, version string, capabilities []string) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"cli_version": version, "capabilities": capabilities})
		if _, err := testPool.Exec(t.Context(), `UPDATE agent_runtime SET metadata=metadata || $2::jsonb WHERE id=$1`, id, raw); err != nil {
			t.Fatal(err)
		}
	}
	setVersion(aRT, "0.2.0", nil)
	setVersion(bRT, "0.4.44", nil)
	if status, body := testHandler.checkPortableQuickCreate(t.Context(), a, true, false); status != 0 {
		t.Fatalf("eligible alternate rejected: %d %+v", status, body)
	}
	if status, _ := testHandler.checkPortableQuickCreate(t.Context(), a, true, true); status != 422 {
		t.Fatal("captured context accepted without capability")
	}
	setVersion(bRT, "0.4.44", []string{protocol.DaemonCapabilitySourceContextQuickCreateV1})
	if status, body := testHandler.checkPortableQuickCreate(t.Context(), a, true, true); status != 0 {
		t.Fatalf("source context rejected: %d %+v", status, body)
	}
	var response QuickCreateIssueResponse
	testutil.Call(t, testHandler.QuickCreateIssue, newRequest("POST", "/api/issues/quick-create", map[string]any{"agent_id": agentID, "prompt": "Create a ticket for the regression fixture", "priority": "high"})).Want(202).JSON(&response)
	t.Cleanup(func() { testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id=$1`, response.TaskID) })
	task, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(response.TaskID))
	if err != nil {
		t.Fatal(err)
	}
	candidates := testHandler.TaskService.ConstrainExecutionToTask(t.Context(), task, testHandler.TaskService.ExecutionCandidates(t.Context(), a, p))
	profile, _, err := execution.Select(p, execution.Request{}, candidates, "")
	if err != nil || profile.RuntimeID != bRT {
		t.Fatalf("task must select compatible alternate: %+v %v", profile, err)
	}
	setVersion(bRT, "0.2.0", nil)
	if status, _ := testHandler.checkPortableQuickCreate(t.Context(), a, true, false); status != 422 {
		t.Fatal("accepted with only obsolete machines")
	}
	raw, _ := json.Marshal(service.QuickCreateContext{Type: service.QuickCreateContextType, Priority: "high"})
	candidates = testHandler.TaskService.ConstrainExecutionToTask(t.Context(), db.AgentTaskQueue{AgentID: a.ID, Context: raw}, testHandler.TaskService.ExecutionCandidates(t.Context(), a, p))
	for _, c := range candidates {
		if c.Eligible {
			t.Fatal("routing failed to recheck daemon version")
		}
	}
}
