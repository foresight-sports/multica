package handler

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/execution"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"strings"
	"testing"
	"time"
)

func executionFixture(t *testing.T) (string, string, string, string, execution.Policy) {
	t.Helper()
	aRT := createClaimReclaimRuntime(t, t.Context(), "Execution default")
	bRT := createClaimReclaimRuntime(t, t.Context(), "Execution alternate")
	agentID, issueID := createClaimReclaimAgentAndIssue(t, context.Background(), aRT, "Execution agent")
	p := execution.Policy{Revision: 1, Mode: "automatic", Preference: "quality", DefaultProfile: "default", AllowFallback: true, Profiles: []execution.Profile{{ID: "default", Name: "Default", Provider: "codex", Model: "m1", Quality: 1, Speed: 1, Cost: 1}, {ID: "alternate", Name: "Alternate", Provider: "claude", Model: "m2", Quality: 5, Speed: 3, Cost: 3}}}
	raw, _ := json.Marshal(p)
	if _, e := testPool.Exec(t.Context(), "UPDATE agent SET execution_policy=$2 WHERE id=$1", agentID, raw); e != nil {
		t.Fatal(e)
	}
	for i, rt := range []string{aRT, bRT} {
		metadata, _ := json.Marshal(map[string]any{"execution_version": 1, "execution_observed_at": time.Now(), "workspace_repository_version": 2, "os": "linux", "tools": []string{"git"}, "execution_catalog_source": "discovered", "execution_catalog_observed_at": time.Now(), "execution_models": []any{map[string]any{"id": p.Profiles[i].Model, "thinking": map[string]any{"supported_levels": []any{map[string]string{"value": "high"}}}}}})
		if _, e := testPool.Exec(t.Context(), `UPDATE agent_runtime SET provider=$2,status='online',last_seen_at=now(),metadata=$3,daemon_id=id::text WHERE id=$1`, rt, p.Profiles[i].Provider, metadata); e != nil {
			t.Fatal(e)
		}
		readyExecutionMachine(t, rt, testWorkspaceID)
	}
	return agentID, issueID, aRT, bRT, p
}
func TestExecutionRoutesAcrossRuntimesAndPinsExplicit(t *testing.T) {
	agentID, issueID, aRT, bRT, p := executionFixture(t)
	taskID := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": aRT, "issue_id": issueID})
	if err := testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(bRT)}); err != nil {
		t.Fatal(err)
	}
	task, e := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
	if e != nil {
		t.Fatal(e)
	}
	selected := execution.ParseSelection(task.ExecutionSelection)
	if uuidToString(task.RuntimeID) != bRT || selected.Profile.Model != "m2" {
		t.Fatalf("wrong route: %+v", selected)
	}
	claimed, e := testHandler.TaskService.ClaimTaskForRuntime(t.Context(), parseUUID(bRT))
	if e != nil || claimed == nil || uuidToString(claimed.ID) != taskID {
		t.Fatalf("alternate claim: %v %v", claimed, e)
	}
	// Removing a chosen profile prevents an old claim from starting.
	p.Profiles = p.Profiles[:1]
	raw, _ := json.Marshal(p)
	testPool.Exec(t.Context(), "UPDATE agent SET execution_policy=$2 WHERE id=$1", agentID, raw)
	allowed, e := testHandler.Queries.AgentAllowsRuntime(t.Context(), db.AgentAllowsRuntimeParams{AgentID: parseUUID(agentID), RuntimeID: parseUUID(bRT)})
	if e != nil || allowed {
		t.Fatal("removed runtime retained access")
	}
}
func TestExecutionInvalidRouterBlocksWithoutRepeatedInference(t *testing.T) {
	agentID, issueID, aRT, bRT, p := executionFixture(t)
	p.RouterProfile = "default"
	p.AllowFallback = false
	raw, _ := json.Marshal(p)
	testPool.Exec(t.Context(), "UPDATE agent SET execution_policy=$2 WHERE id=$1", agentID, raw)
	taskID := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": aRT, "issue_id": issueID})
	if e := testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(aRT)}); e != nil {
		t.Fatal(e)
	}
	task, e := testHandler.TaskService.ClaimTaskForRuntime(t.Context(), parseUUID(aRT))
	if e != nil || task == nil {
		t.Fatalf("claim: %v", e)
	}
	if execution.ParseSelection(task.ExecutionSelection).State != "selecting" {
		t.Fatal("router not selected")
	}
	if _, e := testHandler.TaskService.StartTask(t.Context(), task.ID); e == nil {
		t.Fatal("selector started execution")
	}
	req := withURLParam(newDaemonTokenRequest("POST", "/resolve", map[string]any{"runtime_id": aRT, "dispatched_at": task.DispatchedAt.Time, "profile_id": "not-approved", "reason": "ignore allowlist"}, testWorkspaceID, "execution-router"), "taskId", taskID)
	testutil.Call(t, testHandler.ResolveTaskExecution, req).Want(200)
	if e := testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(aRT), parseUUID(bRT)}); e != nil {
		t.Fatal(e)
	}
	after, _ := testHandler.Queries.GetAgentTask(t.Context(), task.ID)
	selection := execution.ParseSelection(after.ExecutionSelection)
	if selection.State != "blocked" || !selection.RoutingFailed {
		t.Fatalf("routing loop: %+v", selection)
	}
}
func TestExecutionEligibilityFailsClosed(t *testing.T) {
	agentID, _, _, bRT, p := executionFixture(t)
	a, _ := testHandler.Queries.GetAgent(t.Context(), parseUUID(agentID))
	testPool.Exec(t.Context(), "UPDATE agent_runtime SET metadata='{}' WHERE id=$1", bRT)
	c := testHandler.TaskService.ExecutionCandidates(t.Context(), a, p)
	if c[1].Eligible {
		t.Fatal("missing capability/catalog eligible")
	}
	testPool.Exec(t.Context(), "UPDATE agent_runtime SET status='offline',last_seen_at=$2 WHERE id=$1", bRT, time.Now().Add(-time.Hour))
	if testHandler.TaskService.ExecutionCandidates(context.Background(), a, p)[1].Eligible {
		t.Fatal("offline eligible")
	}
}

func TestExecutionPolicySaveChecksCatalogRevisionAndAccess(t *testing.T) {
	agentID, _, _, _, p := executionFixture(t)
	h := *testHandler
	h.ModelCatalogCache = NewInMemoryModelCatalogCache()

	request := func(policy execution.Policy) {
		req := withURLParam(newRequest("PUT", "/execution", policy), "id", agentID)
		testutil.Call(t, h.SaveAgentExecution, req).Want(200)
	}
	request(p)
	req := withURLParam(newRequest("PUT", "/execution", p), "id", agentID)
	testutil.Call(t, h.SaveAgentExecution, req).Want(409)
	p.Revision++
	p.Profiles[0].Model = "unapproved"
	req = withURLParam(newRequest("PUT", "/execution", p), "id", agentID)
	testutil.Call(t, h.SaveAgentExecution, req).Want(400)
}
func TestExecutionConversationPinAndFreshOverride(t *testing.T) {
	agentID, issueID, aRT, bRT, policy := executionFixture(t)
	policy.Profiles[0].ThinkingLevel = "high"
	policyRaw, _ := json.Marshal(policy)
	testPool.Exec(t.Context(), "UPDATE agent SET execution_policy=$2 WHERE id=$1", agentID, policyRaw)
	prior := execution.Selection{State: "selected", Profile: execution.Profile{ID: "default", RuntimeID: aRT, Model: "m1"}}
	raw, _ := json.Marshal(prior)
	dbfx.Task(t, agentID, testutil.Cols{"runtime_id": aRT, "issue_id": issueID, "status": "completed", "started_at": time.Now().Add(-time.Minute), "completed_at": time.Now(), "execution_selection": raw})
	next := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": aRT, "issue_id": issueID})
	testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(bRT)})
	task, _ := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(next))
	if uuidToString(task.RuntimeID) != aRT || task.ForceFreshSession || execution.ParseSelection(task.ExecutionSelection).Profile.ThinkingLevel != "high" {
		t.Fatal("conversation was not pinned")
	}
	testPool.Exec(t.Context(), "UPDATE agent_task_queue SET status='cancelled' WHERE id=$1", next)
	explicit := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": aRT, "issue_id": issueID, "execution_request": []byte(`{"profile_id":"alternate"}`)})
	testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(bRT)})
	task, _ = testHandler.Queries.GetAgentTask(t.Context(), parseUUID(explicit))
	if uuidToString(task.RuntimeID) != bRT || !task.ForceFreshSession {
		t.Fatal("explicit override reused a previous session")
	}
}

func TestExecutionStartedRetryDoesNotMove(t *testing.T) {
	agentID, issueID, aRT, bRT, p := executionFixture(t)
	raw, _ := json.Marshal(execution.Selection{State: "selected", Profile: p.Profiles[0], PolicyRevision: 1})
	parent := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": aRT, "issue_id": issueID, "status": "failed", "started_at": time.Now().Add(-time.Minute), "execution_selection": raw})
	child := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": aRT, "issue_id": issueID, "retry_of_task_id": parent})
	testPool.Exec(t.Context(), "UPDATE agent_runtime SET status='offline' WHERE id=$1", aRT)
	if e := testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(bRT)}); e != nil {
		t.Fatal(e)
	}
	task, _ := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(child))
	if !execution.ParseSelection(task.ExecutionSelection).Locked || uuidToString(task.RuntimeID) != aRT {
		t.Fatal("started retry moved to another runtime")
	}
}
func TestExecutionRouterDecisionAndStaleProposal(t *testing.T) {
	agentID, issueID, aRT, bRT, p := executionFixture(t)
	p.RouterProfile = "default"
	raw, _ := json.Marshal(p)
	testPool.Exec(t.Context(), "UPDATE agent SET execution_policy=$2 WHERE id=$1", agentID, raw)
	taskID := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": aRT, "issue_id": issueID})
	testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(aRT)})
	task, e := testHandler.TaskService.ClaimTaskForRuntime(t.Context(), parseUUID(aRT))
	if e != nil || task == nil {
		t.Fatal(e)
	}
	payload := map[string]any{"runtime_id": aRT, "dispatched_at": task.DispatchedAt.Time, "profile_id": "alternate", "reason": "task needs careful reasoning"}
	req := withURLParam(newDaemonTokenRequest("POST", "/resolve", payload, testWorkspaceID, "execution-router"), "taskId", taskID)
	testutil.Call(t, testHandler.ResolveTaskExecution, req).Want(200)
	next, _ := testHandler.Queries.GetAgentTask(t.Context(), task.ID)
	if next.Status != "queued" || uuidToString(next.RuntimeID) != bRT {
		t.Fatal("router decision not applied")
	}
	req = withURLParam(newDaemonTokenRequest("POST", "/resolve", payload, testWorkspaceID, "execution-router"), "taskId", taskID)
	testutil.Call(t, testHandler.ResolveTaskExecution, req).Want(409)
}

func TestCreateAgentWithExecutionProfiles(t *testing.T) {
	_, _, rt, _, policy := executionFixture(t)
	h := *testHandler
	h.ModelCatalogCache = NewInMemoryModelCatalogCache()

	policy.Revision = 999
	name := "Creation execution " + rt
	var created AgentResponse
	testutil.Call(t, h.CreateAgent, newRequest("POST", "/agents", map[string]any{"name": name, "runtime_id": rt, "execution_policy": policy})).Want(201).JSON(&created)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM agent_invocation_target WHERE agent_id=$1", created.ID)
		testPool.Exec(context.Background(), "DELETE FROM agent WHERE id=$1", created.ID)
	})
	saved, err := h.Queries.GetAgent(context.Background(), parseUUID(created.ID))
	if err != nil {
		t.Fatal(err)
	}
	var actual execution.Policy
	if err = json.Unmarshal(saved.ExecutionPolicy, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Revision != 1 || len(actual.Profiles) != 2 || actual.DefaultProfile != policy.DefaultProfile {
		t.Fatalf("policy not saved: %+v", actual)
	}
	policy.Profiles[0].Model = "unavailable"
	invalidName := name + " invalid"
	testutil.Call(t, h.CreateAgent, newRequest("POST", "/agents", map[string]any{"name": invalidName, "runtime_id": rt, "execution_policy": policy})).Want(400)
	var count int
	if err = testPool.QueryRow(t.Context(), "SELECT count(*) FROM agent WHERE name=$1", invalidName).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid policy created an agent: %d %v", count, err)
	}
}

func TestExistingBuilderClaimUsesCurrentConfigurationInstructions(t *testing.T) {
	agentID, issueID, rt, _, _ := executionFixture(t)
	if _, err := testPool.Exec(t.Context(), "UPDATE agent SET kind='system', system_key='agent_builder:test', instructions='obsolete builder prompt',execution_policy='{}',runtime_id=$2 WHERE id=$1", agentID, rt); err != nil {
		t.Fatal(err)
	}
	taskID := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": rt, "issue_id": issueID})
	task, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := testHandler.Queries.GetAgentRuntime(t.Context(), parseUUID(rt))
	if err != nil {
		t.Fatal(err)
	}
	request := newRequest("POST", "/claim", nil)
	request.Header.Set("X-Client-Capabilities", "workspace-repository-v1")
	resp, _, _, _, _, failure := testHandler.buildClaimedTaskResponse(request, &task, runtime, rt, testWorkspaceID)
	if failure != nil {
		t.Fatalf("claim failed: %+v", failure)
	}
	if !strings.Contains(resp.Agent.Instructions, "execution_policy") || strings.Contains(resp.Agent.Instructions, "obsolete builder prompt") {
		t.Fatal("existing builder used stale instructions")
	}
}

func readyExecutionMachine(t *testing.T, runtimeID, ws string) {
	t.Helper()
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_runtime SET metadata=COALESCE(metadata,'{}') || '{"workspace_repository_version":2}'::jsonb, daemon_id=COALESCE(NULLIF(daemon_id,''),id::text) WHERE id=$1`, runtimeID); err != nil {
		t.Fatal(err)
	}
	q := testHandler.Queries
	rt, e := q.GetAgentRuntime(t.Context(), parseUUID(runtimeID))
	if e != nil {
		t.Fatal(e)
	}
	for _, config := range []struct{ scope, subject, value string }{{"workspace", ws, `{"folder":"portable-tests","mode":"in_place"}`}, {"machine", workspacerepo.MachineKey(rt), `{"root":"/tmp/repos"}`}} {
		tag, err := testPool.Exec(t.Context(), `INSERT INTO repository_configuration(scope,subject,config,revision) VALUES($1,$2,$3,1) ON CONFLICT(scope,subject) DO NOTHING`, config.scope, config.subject, config.value)
		if err != nil {
			t.Fatal(err)
		}
		if tag.RowsAffected() == 1 {
			t.Cleanup(func() {
				testPool.Exec(context.Background(), "DELETE FROM repository_configuration WHERE scope=$1 AND subject=$2", config.scope, config.subject)
			})
		}
	}
	plan, e := workspacerepo.BuildPlan(t.Context(), q, rt, ws)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(workspacerepo.Readiness{WorkspaceID: ws, Fingerprint: plan.Fingerprint, State: "ready", UpdatedAt: time.Now()})
	if e = q.PutRepositoryReadiness(t.Context(), db.PutRepositoryReadinessParams{Subject: workspacerepo.StatusKey(rt, ws), Config: raw}); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM repository_configuration WHERE subject=$1 OR subject=$2", workspacerepo.MachineKey(rt), workspacerepo.StatusKey(rt, ws))
	})
}

func TestPortableExecutionQueuesWithoutMachineAndFailsOverSameProfile(t *testing.T) {
	aid, iid, r1, r2, p := executionFixture(t)
	p.Profiles = p.Profiles[:1]
	p.AllowFallback = false
	p.Profiles[0].RequiredTools = []string{"git"}
	raw, _ := json.Marshal(p)
	if _, e := testPool.Exec(t.Context(), "UPDATE agent SET execution_policy=$2 WHERE id=$1", aid, raw); e != nil {
		t.Fatal(e)
	}
	if _, e := testPool.Exec(t.Context(), "UPDATE agent_runtime SET provider='codex',metadata=(SELECT metadata FROM agent_runtime WHERE id=$2) WHERE id=$1", r2, r1); e != nil {
		t.Fatal(e)
	}
	a, e := testHandler.Queries.GetAgent(t.Context(), parseUUID(aid))
	if e != nil || a.RuntimeID.Valid {
		t.Fatal("portable agent retained binding", e)
	}
	taskID := dbfx.Task(t, aid, testutil.Cols{"runtime_id": nil, "issue_id": iid, "execution_request": []byte(`{"profile_id":"default"}`)})
	route := func() db.AgentTaskQueue {
		t.Helper()
		if e := testHandler.TaskService.RouteExecutionTasks(t.Context(), []pgtype.UUID{parseUUID(r1), parseUUID(r2)}); e != nil {
			t.Fatal(e)
		}
		task, e := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
		if e != nil {
			t.Fatal(e)
		}
		return task
	}
	first := route()
	if !first.RuntimeID.Valid {
		t.Fatal("no eligible machine selected", string(first.ExecutionSelection))
	}
	testPool.Exec(t.Context(), "UPDATE agent_runtime SET status='offline' WHERE id=$1", first.RuntimeID)
	second := route()
	if second.RuntimeID == first.RuntimeID || execution.ParseSelection(second.ExecutionSelection).Profile.ID != "default" {
		t.Fatal("same profile did not move machines")
	}
	if !second.ForceFreshSession {
		t.Fatal("cross-machine session was reused")
	}
	claimed, e := testHandler.TaskService.ClaimTaskForRuntime(t.Context(), second.RuntimeID)
	if e != nil || claimed == nil {
		t.Fatal("claim", e)
	}
	// A machine losing a tool after selection cannot begin the task.
	testPool.Exec(t.Context(), `UPDATE agent_runtime SET metadata=jsonb_set(metadata,'{tools}','[]') WHERE id=$1`, second.RuntimeID)
	if _, e := testHandler.TaskService.StartTask(t.Context(), claimed.ID); e == nil {
		t.Fatal("started without required tools")
	}
	next := route()
	if execution.ParseSelection(next.ExecutionSelection).State != "blocked" {
		t.Fatal("no capable machine must block")
	}
}

func TestPortableExecutionRejectsPrivateAndUnreadyMachines(t *testing.T) {
	aid, _, r1, r2, p := executionFixture(t)
	p.Profiles = p.Profiles[:1]
	raw, _ := json.Marshal(p)
	testPool.Exec(t.Context(), "UPDATE agent SET execution_policy=$2 WHERE id=$1", aid, raw)
	other := dbfx.User(t, "Other machine owner", r2+"@example.test")
	testPool.Exec(t.Context(), "UPDATE agent_runtime SET provider='codex',owner_id=$2,visibility='private' WHERE id=$1", r2, other)
	a, _ := testHandler.Queries.GetAgent(t.Context(), parseUUID(aid))
	for _, c := range testHandler.TaskService.ExecutionCandidates(t.Context(), a, p) {
		if c.Profile.RuntimeID == r2 {
			t.Fatal("another owner's private machine was exposed")
		}
	}
	rt, _ := testHandler.Queries.GetAgentRuntime(t.Context(), parseUUID(r1))
	testPool.Exec(t.Context(), "DELETE FROM repository_configuration WHERE scope='readiness' AND subject=$1", workspacerepo.StatusKey(rt, testWorkspaceID))
	for _, c := range testHandler.TaskService.ExecutionCandidates(t.Context(), a, p) {
		if c.Eligible {
			t.Fatal("unprepared machine was eligible")
		}
	}
	readyExecutionMachine(t, r1, testWorkspaceID)
	p.Profiles[0].RequiredTools = []string{"git"}
	testPool.Exec(t.Context(), `UPDATE agent_runtime SET metadata=jsonb_set(metadata,'{execution_observed_at}',to_jsonb($2::text)) WHERE id=$1`, r1, time.Now().Add(-time.Hour).Format(time.RFC3339))
	for _, c := range testHandler.TaskService.ExecutionCandidates(t.Context(), a, p) {
		if c.Eligible {
			t.Fatal("stale inventory was eligible")
		}
	}
}

func TestPortableInstanceBindingUsesSourceRequirementsAndWorkspaceMachines(t *testing.T) {
	_, _, r1, r2, p := executionFixture(t)
	other := dbfx.Workspace(t, "Portable target", "portable-instance-"+r1)
	dbfx.Member(t, other, testUserID, "owner")
	h := *testHandler
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	var source AgentResponse
	testutil.Call(t, h.CreateAgent, newRequest("POST", "/agents", map[string]any{"name": "Portable instance", "instance_agent": true, "execution_policy": p})).Want(201).JSON(&source)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM agent WHERE instance_agent_id=$1", source.InstanceAgentID)
		testPool.Exec(context.Background(), "DELETE FROM instance_agent WHERE id=$1", source.InstanceAgentID)
	})
	bindings, e := h.Queries.ListAgents(t.Context(), parseUUID(other))
	if e != nil || len(bindings) != 1 {
		t.Fatal("binding", e)
	}
	binding := bindings[0]
	if binding.RuntimeID.Valid || !service.HasExecutionProfiles(binding) {
		t.Fatal("binding did not inherit portable policy")
	}
	testPool.Exec(t.Context(), "UPDATE agent_runtime SET workspace_id=$2 WHERE id=$1", r2, other)
	readyExecutionMachine(t, r1, other)
	readyExecutionMachine(t, r2, other)
	cs := h.TaskService.ExecutionCandidates(t.Context(), binding, p)
	found := map[string]bool{}
	for _, c := range cs {
		if c.Eligible {
			found[c.Profile.RuntimeID] = true
		}
	}
	if !found[r1] || !found[r2] {
		t.Fatalf("binding lost authorized source/local machines: %+v", cs)
	}
	testPool.Exec(t.Context(), "UPDATE agent SET archived_at=now() WHERE id=$1", binding.ID)
	cs = h.TaskService.ExecutionCandidates(t.Context(), binding, p)
	for _, c := range cs {
		if c.Eligible {
			t.Fatal("workspace opt-out was ignored")
		}
	}
}

func TestPortableRuntimeRemovalReleasesQueuedWork(t *testing.T) {
	aid, iid, r1, _, p := executionFixture(t)
	selected := p.Profiles[0]
	selected.RuntimeID = r1
	raw, _ := json.Marshal(execution.Selection{State: "selected", Profile: selected, PolicyRevision: 1})
	tid := dbfx.Task(t, aid, testutil.Cols{"runtime_id": r1, "issue_id": iid, "execution_selection": raw})
	tx, e := testHandler.TxStarter.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(t.Context())
	q := testHandler.Queries.WithTx(tx)
	if _, e = service.TeardownRuntime(t.Context(), q, parseUUID(r1), service.RuntimeTeardownOptions{CancelNonTerminalTasks: true}); e != nil {
		t.Fatal(e)
	}
	task, e := q.GetAgentTask(t.Context(), parseUUID(tid))
	if e != nil || task.Status != "queued" || task.RuntimeID.Valid {
		t.Fatal("portable work was lost", task, e)
	}
	a, e := q.GetAgent(t.Context(), parseUUID(aid))
	if e != nil || !service.HasExecutionProfiles(a) {
		t.Fatal("agent requirements were lost", e)
	}
}
