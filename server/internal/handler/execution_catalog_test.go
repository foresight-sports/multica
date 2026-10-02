package handler

import (
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/testutil"
	"testing"
	"time"
)

func TestExecutionCatalogAcceptsReportedChoicesAfterCacheLoss(t *testing.T) {
	agentID, _, rt, _, p := executionFixture(t)
	p.Profiles = p.Profiles[:1]
	p.DefaultProfile = p.Profiles[0].ID
	for _, source := range []string{"discovered", "fallback"} {
		t.Run(source, func(t *testing.T) {
			metadata, _ := json.Marshal(map[string]any{"execution_models": []ModelEntry{{ID: "m1"}}, "execution_catalog_source": source, "execution_catalog_observed_at": time.Now().UTC()})
			if _, e := testPool.Exec(t.Context(), "UPDATE agent_runtime SET metadata=$2 WHERE id=$1", rt, metadata); e != nil {
				t.Fatal(e)
			}
			h := *testHandler
			h.ModelCatalogCache = NewInMemoryModelCatalogCache()
			req := withURLParam(newRequest("PUT", "/execution", p), "id", agentID)
			testutil.Call(t, h.SaveAgentExecution, req).Want(200)
			p.Revision++
		})
	}
}
func TestExecutionCatalogRejectsExpiredAndUnreportedChoices(t *testing.T) {
	for _, at := range []time.Time{{}, time.Now().Add(-25 * time.Hour)} {
		raw, _ := json.Marshal(map[string]any{"execution_models": []ModelEntry{{ID: "m"}}, "execution_catalog_source": "fallback", "execution_catalog_observed_at": at})
		if reportedExecutionCatalog(raw, time.Now()) != nil {
			t.Fatal("unverified catalog accepted")
		}
	}
}

func TestReportedFallbackChoicesPersistWithoutDiscoveryCache(t *testing.T) {
	_, _, rt, _, _ := executionFixture(t)
	h := *testHandler
	h.ModelCatalogCache = nil
	h.ModelListStore = NewInMemoryModelListStore()
	pending, e := h.ModelListStore.Create(t.Context(), rt)
	if e != nil {
		t.Fatal(e)
	}
	req := newDaemonTokenRequest("POST", "/models/result", map[string]any{"status": "completed", "supported": true, "fallback": true, "models": []ModelEntry{{ID: "claude-choice"}}}, testWorkspaceID, "catalog-test")
	route := chi.NewRouteContext()
	route.URLParams.Add("runtimeId", rt)
	route.URLParams.Add("requestId", pending.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	testutil.Call(t, h.ReportModelListResult, req).Want(200)
	completed, e := h.ModelListStore.Get(t.Context(), pending.ID)
	if e != nil || completed.Status != ModelListCompleted {
		t.Fatal("not completed", e)
	}
	runtime, e := h.Queries.GetAgentRuntime(t.Context(), parseUUID(rt))
	if e != nil {
		t.Fatal(e)
	}
	catalog := reportedExecutionCatalog(runtime.Metadata, time.Now())
	if catalog == nil || catalog.Models[0].ID != "claude-choice" {
		t.Fatal("completed without durable choices")
	}
}
