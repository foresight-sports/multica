package handler

import (
	"encoding/json"
	"errors"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/execution"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWorkspaceRepositoryReadinessLifecycle(t *testing.T) {
	workspace := dbfx.Workspace(t, "Readiness", "readiness-test")
	dbfx.Member(t, workspace, testUserID, "owner")
	runtimeID := dbfx.Runtime(t, "Readiness runtime", testutil.Cols{"workspace_id": workspace, "daemon_id": "readiness-test", "owner_id": testUserID, "status": "online", "last_seen_at": time.Now(), "metadata": `{"workspace_repository_version":2,"execution_version":1,"execution_models":[{"id":"test-model"}]}`})
	rt, err := testHandler.Queries.GetAgentRuntime(t.Context(), parseUUID(runtimeID))
	if err != nil {
		t.Fatal(err)
	}
	q := testHandler.Queries
	agentID := dbfx.Agent(t, "Repository routing", runtimeID, testutil.Cols{"workspace_id": workspace, "runtime_id": runtimeID})
	agent, err := q.GetAgent(t.Context(), parseUUID(agentID))
	if err != nil {
		t.Fatal(err)
	}
	policy := execution.Policy{Profiles: []execution.Profile{{ID: "default", Provider: rt.Provider, Model: "test-model"}}}
	cleanup := func() {
		testPool.Exec(t.Context(), "DELETE FROM repository_configuration WHERE subject=$1 OR subject=$2 OR subject=$3", workspace, workspacerepo.MachineKey(rt), workspacerepo.StatusKey(rt, workspace))
	}
	defer cleanup()
	if err := workspacerepo.RequireConfigured(t.Context(), q, workspace); !errors.Is(err, workspacerepo.ErrConfigurationRequired) {
		t.Fatal(err)
	}
	_, err = testHandler.IssueService.Create(t.Context(), service.IssueCreateParams{WorkspaceID: parseUUID(workspace), Title: "blocked"}, service.IssueCreateOpts{})
	if !errors.Is(err, workspacerepo.ErrConfigurationRequired) {
		t.Fatalf("unconfigured workspace accepted: %v", err)
	}
	_, err = q.SaveRepositoryConfiguration(t.Context(), db.SaveRepositoryConfigurationParams{Scope: "workspace", Subject: workspace, Config: []byte(`{"folder":"local","mode":"in_place"}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.SaveRepositoryConfiguration(t.Context(), db.SaveRepositoryConfigurationParams{Scope: "machine", Subject: workspacerepo.MachineKey(rt), Config: []byte(`{"root":"D:/GitHub"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := workspacerepo.RequireConfigured(t.Context(), q, workspace); err != nil {
		t.Fatal(err)
	}
	p, err := workspacerepo.BuildPlan(t.Context(), q, rt, workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"cloning", "ready", "unavailable"} {
		raw, _ := json.Marshal(workspacerepo.Readiness{WorkspaceID: workspace, Fingerprint: p.Fingerprint, State: state, UpdatedAt: time.Now(), Message: state})
		if err = q.PutRepositoryReadiness(t.Context(), db.PutRepositoryReadinessParams{Subject: workspacerepo.StatusKey(rt, workspace), Config: raw}); err != nil {
			t.Fatal(err)
		}
		status, e := workspacerepo.Status(t.Context(), q, rt, workspace)
		if e != nil || status.State != state || status.Usable != (state == "ready") {
			t.Fatal(status, e)
		}
		candidates := testHandler.TaskService.ExecutionCandidates(t.Context(), agent, policy)
		if len(candidates) != 1 || candidates[0].Eligible != (state == "ready") {
			t.Fatalf("router readiness %s: %+v", state, candidates)
		}
	}
	raw, _ := json.Marshal(workspacerepo.Readiness{Fingerprint: "old-config", State: "ready", UpdatedAt: time.Now()})
	q.PutRepositoryReadiness(t.Context(), db.PutRepositoryReadinessParams{Subject: workspacerepo.StatusKey(rt, workspace), Config: raw})
	status, err := workspacerepo.Status(t.Context(), q, rt, workspace)
	if err != nil || status.Usable {
		t.Fatal(status, err)
	}
	// A report for a workspace this runtime cannot serve must not create readiness.
	req := withURLParam(newRequest("POST", "/workspace-repositories", map[string]any{"workspace_id": testWorkspaceID, "fingerprint": "unrelated", "state": "ready"}), "runtimeId", runtimeID)
	w := httptest.NewRecorder()
	testHandler.DaemonWorkspaceRepositories(w, req)
	if w.Code != 409 {
		t.Fatalf("foreign report=%d %s", w.Code, w.Body.String())
	}
}
