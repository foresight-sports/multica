package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestInstanceSharedRuntimeClaimAndTaskScope(t *testing.T) {
	h := *testHandler
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	other := dbfx.Workspace(t, "Shared runtime target", "instance-shared-runtime-target")
	otherUser := dbfx.User(t, "Target owner", "instance-target-owner@example.test")
	dbfx.Member(t, other, otherUser, "owner")
	runtimeID := createClaimReclaimRuntime(t, t.Context(), "Shared instance runtime")
	var created AgentResponse
	testutil.Call(t, h.CreateAgent, newRequest("POST", "/api/agents", CreateAgentRequest{InstanceAgent: true, Name: "Shared runtime agent", RuntimeID: runtimeID, Visibility: "workspace", MaxConcurrentTasks: 2})).Want(201).JSON(&created)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM agent WHERE instance_agent_id=$1", created.InstanceAgentID)
		testPool.Exec(context.Background(), "DELETE FROM instance_agent WHERE id=$1", created.InstanceAgentID)
	})
	agents, err := h.Queries.ListAgents(t.Context(), parseUUID(other))
	if err != nil || len(agents) != 1 {
		t.Fatalf("missing shared binding: %v", err)
	}
	binding := agents[0]
	issueID := dbfx.Issue(t, "Target workspace issue", testutil.Cols{"workspace_id": other, "creator_id": otherUser})
	taskID := dbfx.Task(t, uuidToString(binding.ID), testutil.Cols{"runtime_id": runtimeID, "issue_id": issueID})
	claim := withURLParam(newDaemonTokenRequest("POST", "/claim", nil, testWorkspaceID, "instance-runtime"), "runtimeId", runtimeID)
	var response struct {
		Task *AgentTaskResponse `json:"task"`
	}
	testutil.Call(t, h.ClaimTaskByRuntime, claim).Want(200).JSON(&response)
	if response.Task == nil || response.Task.ID != taskID || response.Task.WorkspaceID != other || response.Task.AgentID != uuidToString(binding.ID) {
		t.Fatalf("wrong shared task claim: %+v", response.Task)
	}
	start := withURLParam(newDaemonTokenRequest("POST", "/start", nil, testWorkspaceID, "instance-runtime"), "taskId", taskID)
	testutil.Call(t, h.StartTask, start).Want(200)
	// The task token works without enrolling the runtime owner in this workspace.
	tokenRequest := newRequest("GET", "/api/issues", nil)
	tokenRequest.Header.Set("X-Actor-Source", "task_token")
	tokenRequest.Header.Set("X-Task-ID", taskID)
	tokenRequest.Header.Set("X-Agent-ID", uuidToString(binding.ID))
	tokenRequest.Header.Set("X-Workspace-ID", other)
	member, ok := middleware.InstanceTaskMember(tokenRequest, h.Queries, other)
	if !ok || member.Role != "member" {
		t.Fatal("instance task needs scoped member access")
	}
	if _, ok := middleware.InstanceTaskMember(tokenRequest, h.Queries, testWorkspaceID); ok {
		t.Fatal("task token escaped its target workspace")
	}
	tokenRequest.Header.Set("X-Agent-ID", created.ID)
	if _, ok := middleware.InstanceTaskMember(tokenRequest, h.Queries, other); ok {
		t.Fatal("mismatched task agent was accepted")
	}
	// An unrelated cross-workspace agent never receives the runtime grant.
	ordinaryID := dbfx.Agent(t, "Unrelated runtime binding", runtimeID, testutil.Cols{"workspace_id": other})
	ordinaryTask := dbfx.Task(t, ordinaryID, testutil.Cols{"runtime_id": runtimeID, "issue_id": issueID})
	ordinary, _ := h.Queries.GetAgentTask(t.Context(), parseUUID(ordinaryTask))
	if h.instanceTaskWorkspace(t.Context(), ordinary) != "" {
		t.Fatal("ordinary agent received an instance grant")
	}
	if _, err := testPool.Exec(t.Context(), "DELETE FROM agent WHERE id=$1", ordinaryID); err != nil {
		t.Fatal(err)
	}
	tx, err := h.TxStarter.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	_, err = service.TeardownRuntime(t.Context(), h.Queries.WithTx(tx), parseUUID(runtimeID), service.RuntimeTeardownOptions{CancelNonTerminalTasks: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	unbound, err := h.Queries.GetAgent(t.Context(), binding.ID)
	if err != nil || unbound.RuntimeID.Valid {
		t.Fatalf("shared runtime teardown did not unbind remote agent: %+v %v", unbound, err)
	}
	statusRequest := newDaemonTokenRequest("GET", "/task-status", nil, testWorkspaceID, "instance-runtime")
	// Cancellation/status reporting must survive removal of runtime_id.
	task, ws, ok := h.requireDaemonTaskAccessWithWorkspace(httptest.NewRecorder(), statusRequest, taskID)
	if !ok || ws != other || task.Status != "cancelled" {
		t.Fatal("shared task lifecycle access was lost during teardown")
	}
}

func TestInstanceAgentNormalCreation(t *testing.T) {
	h := *testHandler
	other := dbfx.Workspace(t, "Normal instance creation", "instance-normal-create")
	dbfx.Member(t, other, testUserID, "owner")
	body := CreateAgentRequest{InstanceAgent: true, Name: "Normal instance agent", RuntimeID: testRuntimeID,
		Description: "Shared description", Instructions: "Shared instructions", Visibility: "private", MaxConcurrentTasks: 2,
		CustomEnv:            map[string]string{"WORKSPACE_ONLY": "private-value"},
		ConversationStarters: []AgentConversationStarter{{Label: "Start", Prompt: "Review this"}},
	}
	testutil.Call(t, h.CreateAgent, newRequest("POST", "/api/agents", body)).Want(403)
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	agentRequest := newRequest("POST", "/api/agents", body)
	agentRequest.Header.Set("X-Actor-Source", "task_token")
	agentRequest.Header.Set("X-Agent-ID", testUserID)
	testutil.Call(t, h.CreateAgent, agentRequest).Want(403)
	var created AgentResponse
	testutil.Call(t, h.CreateAgent, newRequest("POST", "/api/agents", body)).Want(201).JSON(&created)
	if created.InstanceAgentID == "" {
		t.Fatal("normal create did not return instance identity")
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM agent_invocation_target WHERE agent_id IN (SELECT id FROM agent WHERE instance_agent_id=$1)", created.InstanceAgentID)
		testPool.Exec(context.Background(), "DELETE FROM agent WHERE instance_agent_id=$1", created.InstanceAgentID)
		testPool.Exec(context.Background(), "DELETE FROM instance_agent WHERE id=$1", created.InstanceAgentID)
	})
	local, err := h.Queries.GetAgent(t.Context(), parseUUID(created.ID))
	if err != nil || local.RuntimeID != parseUUID(testRuntimeID) || local.MaxConcurrentTasks != 2 || local.PermissionMode != "private" || !strings.Contains(string(local.CustomEnv), "private-value") || !strings.Contains(string(local.ConversationStarters), "Review this") {
		t.Fatalf("source workspace setup was not preserved: %+v %v", local, err)
	}
	remote, err := h.Queries.ListAgents(t.Context(), parseUUID(other))
	if err != nil || len(remote) != 1 || remote[0].InstanceAgentID != local.InstanceAgentID || remote[0].RuntimeID != local.RuntimeID || remote[0].ArchivedAt.Valid || remote[0].PermissionMode != "private" || !strings.Contains(string(remote[0].CustomEnv), "private-value") {
		t.Fatalf("other workspace must inherit the central setup: %+v %v", remote, err)
	}
	remoteID := uuidToString(remote[0].ID)
	remoteUpdate := withURLParam(newRequest("PUT", "/api/agents/"+remoteID, map[string]any{"runtime_id": testRuntimeID}), "id", remoteID)
	remoteUpdate.Header.Set("X-Workspace-ID", other)
	testutil.Call(t, h.UpdateAgent, remoteUpdate).Want(403)
	// Updating the central setup propagates without any workspace action.
	testutil.Call(t, h.UpdateAgent, withURLParam(newRequest("PUT", "/api/agents/"+created.ID, map[string]any{"max_concurrent_tasks": 3, "visibility": "workspace"}), "id", created.ID)).Want(200)
	remoteAgent, err := h.Queries.GetAgent(t.Context(), remote[0].ID)
	if err != nil || remoteAgent.MaxConcurrentTasks != 3 || remoteAgent.PermissionMode != "public_to" {
		t.Fatalf("central update not propagated: %+v %v", remoteAgent, err)
	}
	targets, err := h.Queries.ListAgentInvocationTargets(t.Context(), remoteAgent.ID)
	if err != nil || len(targets) != 1 || targets[0].TargetID != parseUUID(other) {
		t.Fatalf("shared workspace access was not mapped: %+v %v", targets, err)
	}
	skillID := dbfx.Insert(t, "skill", testutil.Cols{"workspace_id": testWorkspaceID, "name": "Shared instance skill", "description": "", "content": "Use this skill", "created_by": testUserID})
	if err := h.Queries.AddAgentSkill(t.Context(), db.AddAgentSkillParams{AgentID: local.ID, SkillID: parseUUID(skillID)}); err != nil {
		t.Fatal(err)
	}
	skills, err := h.Queries.ListAgentSkills(t.Context(), remoteAgent.ID)
	if err != nil || len(skills) != 1 || skills[0].ID != parseUUID(skillID) {
		t.Fatalf("central skills not shared: %+v %v", skills, err)
	}
	if err := h.Queries.RemoveAllAgentSkills(t.Context(), local.ID); err != nil {
		t.Fatal(err)
	}
	skills, err = h.Queries.ListAgentSkills(t.Context(), remoteAgent.ID)
	if err != nil || len(skills) != 0 {
		t.Fatalf("central skill removal not propagated: %+v %v", skills, err)
	}
	// A global name conflict must roll back the ordinary agent too.
	testutil.Call(t, h.CreateAgent, newRequest("POST", "/api/agents", body)).Want(409)
	var leaked int
	if err := testPool.QueryRow(t.Context(), "SELECT count(*) FROM agent WHERE workspace_id=$1 AND name=$2", testWorkspaceID, body.Name).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("failed instance creation left a workspace agent: %d %v", leaked, err)
	}
}

func TestInstanceConfigurationPermissionsAndRevision(t *testing.T) {
	h := *testHandler
	old, err := h.Queries.GetInstanceConfiguration(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "UPDATE instance_configuration SET instructions=$1, revision=$2 WHERE singleton", old.Instructions, old.Revision)
	})
	req := InstanceConfigurationResponse{Instructions: "Always report test results.", Revision: old.Revision}
	testutil.Call(t, h.UpdateInstanceConfiguration, newRequest("PUT", "/api/instance/configuration", req)).Want(403)
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	var saved InstanceConfigurationResponse
	testutil.Call(t, h.UpdateInstanceConfiguration, newRequest("PUT", "/api/instance/configuration", req)).Want(200).JSON(&saved)
	if saved.Revision != old.Revision+1 || saved.Instructions != req.Instructions {
		t.Fatalf("unexpected save: %+v", saved)
	}
	testutil.Call(t, h.UpdateInstanceConfiguration, newRequest("PUT", "/api/instance/configuration", req)).Want(409)
	fresh := *testHandler
	var got InstanceConfigurationResponse
	testutil.Call(t, fresh.GetInstanceConfiguration, newRequest("GET", "/api/instance/configuration", nil)).Want(200).JSON(&got)
	if got != saved {
		t.Fatalf("configuration not durable: %+v", got)
	}
	req.Instructions = ""
	req.Revision = saved.Revision
	testutil.Call(t, h.UpdateInstanceConfiguration, newRequest("PUT", "/api/instance/configuration", req)).Want(200)
}

func TestInstanceAgentWorkspaceBindingsAndOptOut(t *testing.T) {
	h := *testHandler
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	other := dbfx.Workspace(t, "Instance agent second workspace", "instance-agent-second")
	dbfx.Member(t, other, testUserID, "owner")
	body := saveInstanceAgentRequest{Name: "Instance reviewer", Description: "Shared reviewer", Instructions: "Review carefully."}
	denied := *testHandler
	testutil.Call(t, denied.SaveInstanceAgent, newRequest("POST", "/api/instance/agents", body)).Want(403)
	var saved map[string]string
	testutil.Call(t, h.SaveInstanceAgent, newRequest("POST", "/api/instance/agents", body)).Want(201).JSON(&saved)
	id := saved["id"]
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM agent_invocation_target WHERE agent_id IN (SELECT id FROM agent WHERE instance_agent_id=$1)", id)
		testPool.Exec(context.Background(), "DELETE FROM agent WHERE instance_agent_id=$1", id)
		testPool.Exec(context.Background(), "DELETE FROM instance_agent WHERE id=$1", id)
	})
	var list []InstanceAgentResponse
	testutil.Call(t, h.ListInstanceAgents, newRequest("GET", "/api/instance/agents", nil)).Want(200).JSON(&list)
	var current InstanceAgentResponse
	for _, a := range list {
		if a.ID == id {
			current = a
		}
	}
	if !current.Enabled || current.RuntimeBound || current.AgentID == "" {
		t.Fatalf("new binding should be enabled and unbound: %+v", current)
	}
	var second []InstanceAgentResponse
	r := newRequest("GET", "/api/instance/agents", nil)
	r.Header.Set("X-Workspace-ID", other)
	testutil.Call(t, h.ListInstanceAgents, r).Want(200).JSON(&second)
	if len(second) != 1 || second[0].ID != id || !second[0].Enabled || second[0].AgentID == current.AgentID {
		t.Fatalf("expected same definition and separate workspace binding: %+v", second)
	}
	binding, err := h.Queries.GetAgent(t.Context(), parseUUID(current.AgentID))
	if err != nil {
		t.Fatal(err)
	}
	if !h.canInvokeAgent(t.Context(), binding, "member", testUserID, "", testWorkspaceID) {
		t.Fatal("new instance agent must be invocable by workspace members")
	}
	// Workspace clients cannot override the shared identity, including managers
	// who must go through the revision-checked instance endpoint.
	testutil.Call(t, h.UpdateAgent, withURLParam(newRequest("PUT", "/api/agents/"+current.AgentID, map[string]any{"instructions": "local override"}), "id", current.AgentID)).Want(403)
	testutil.Call(t, h.ArchiveAgent, withURLParam(newRequest("POST", "/api/agents/"+current.AgentID+"/archive", nil), "id", current.AgentID)).Want(200)
	body.Revision = current.Revision
	body.Instructions = "Updated centrally."
	testutil.Call(t, h.SaveInstanceAgent, withURLParam(newRequest("PUT", "/api/instance/agents/"+id, body), "id", id)).Want(200)
	testutil.Call(t, h.SaveInstanceAgent, withURLParam(newRequest("PUT", "/api/instance/agents/"+id, body), "id", id)).Want(409)
	testutil.Call(t, h.ListInstanceAgents, newRequest("GET", "/api/instance/agents", nil)).Want(200).JSON(&list)
	for _, a := range list {
		if a.ID == id && (a.Enabled || a.Instructions != body.Instructions) {
			t.Fatalf("global edit must preserve opt-out: %+v", a)
		}
	}
	remote, err := h.Queries.GetAgent(t.Context(), parseUUID(second[0].AgentID))
	if err != nil || remote.Instructions != body.Instructions || remote.ArchivedAt.Valid {
		t.Fatalf("other workspace must receive edit and stay enabled: %+v %v", remote, err)
	}
	testutil.Call(t, h.RestoreAgent, withURLParam(newRequest("POST", "/api/agents/"+current.AgentID+"/restore", nil), "id", current.AgentID)).Want(200)
	// New workspaces use the same default. Reconciliation is idempotent.
	future := dbfx.Workspace(t, "Future workspace", "instance-agent-future")
	dbfx.Member(t, future, testUserID, "owner")
	if err := h.ensureInstanceAgents(t.Context(), parseUUID(future)); err != nil {
		t.Fatal(err)
	}
	if err := h.ensureInstanceAgents(t.Context(), parseUUID(future)); err != nil {
		t.Fatal(err)
	}
	futureAgents, err := h.Queries.ListAgents(t.Context(), parseUUID(future))
	if err != nil || len(futureAgents) != 1 || futureAgents[0].InstanceAgentID != parseUUID(id) {
		t.Fatalf("future workspace: %+v %v", futureAgents, err)
	}
	// The ordinary workspace detail loader still refuses a foreign binding.
	testutil.Call(t, h.GetAgent, withURLParam(newRequest("GET", "/api/agents/"+second[0].AgentID, nil), "id", second[0].AgentID)).Want(http.StatusNotFound)
}

func TestInstanceInstructionsReachClaim(t *testing.T) {
	old, err := testHandler.Queries.GetInstanceConfiguration(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "UPDATE instance_configuration SET instructions=$1, revision=$2 WHERE singleton", old.Instructions, old.Revision)
	})
	for index, text := range []string{"Instance policy first revision.", "Instance policy second revision."} {
		_, err := testHandler.Queries.UpdateInstanceConfiguration(t.Context(), db.UpdateInstanceConfigurationParams{
			Instructions: text, UpdatedBy: parseUUID(testUserID), ExpectedRevision: old.Revision,
		})
		if err != nil {
			t.Fatal(err)
		}
		old.Revision++
		runtimeID := createClaimReclaimRuntime(t, t.Context(), "Instance claim runtime")
		agentID, issueID := createClaimReclaimAgentAndIssue(t, t.Context(), runtimeID, fmt.Sprintf("Instance claim agent %d", index))
		createDispatchedClaimFixtureTask(t, t.Context(), agentID, runtimeID, issueID, "120 seconds", false)
		req := newDaemonTokenRequest("POST", "/api/daemon/runtimes/"+runtimeID+"/tasks/claim", nil, testWorkspaceID, "instance-claim")
		req = withURLParam(req, "runtimeId", runtimeID)
		var response struct {
			Task *AgentTaskResponse `json:"task"`
		}
		testutil.Call(t, testHandler.ClaimTaskByRuntime, req).Want(200).JSON(&response)
		if response.Task == nil || response.Task.Agent == nil || !strings.Contains(response.Task.Agent.Instructions, text) {
			t.Fatalf("missing instance instructions: %+v", response.Task)
		}
	}
	if got := composeInstanceInstructions("", "local instructions"); got != "local instructions" {
		t.Fatalf("empty instance prompt altered local instructions: %q", got)
	}
}

func TestInstanceInstructionsMissingPreventLaunch(t *testing.T) {
	old, err := testHandler.Queries.GetInstanceConfiguration(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	runtimeID := createClaimReclaimRuntime(t, t.Context(), "Instance missing runtime")
	agentID, issueID := createClaimReclaimAgentAndIssue(t, t.Context(), runtimeID, "Instance missing agent")
	taskID := createDispatchedClaimFixtureTask(t, t.Context(), agentID, runtimeID, issueID, "120 seconds", false)
	dbfx.Exec(t, "DELETE FROM instance_configuration WHERE singleton")
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "INSERT INTO instance_configuration (singleton, instructions, revision) VALUES (true,$1,$2)", old.Instructions, old.Revision)
	})
	req := newDaemonTokenRequest("POST", "/api/daemon/runtimes/"+runtimeID+"/tasks/claim", nil, testWorkspaceID, "instance-missing")
	testutil.Call(t, testHandler.ClaimTaskByRuntime, withURLParam(req, "runtimeId", runtimeID)).Want(http.StatusServiceUnavailable)
	var status string
	dbfx.QueryRow(t, "SELECT status FROM agent_task_queue WHERE id=$1", taskID).Scan(&status)
	if status != "queued" {
		t.Fatalf("missing policy must requeue, got %q", status)
	}
}
