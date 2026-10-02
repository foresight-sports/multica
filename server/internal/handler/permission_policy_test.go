package handler

import (
	"errors"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestRuntimePermissionPolicyManagement(t *testing.T) {
	h := *testHandler
	h.cfg.RuntimeRegistrationRestricted = true
	h.cfg.RuntimeRegistrationAllowedEmails = []string{"initial@foresightsports.com"}
	h.cfg.PermissionManagerEmails = nil
	path := "/api/permission-policies/runtime-register"
	dbfx.Exec(t, `DELETE FROM permission_policy WHERE action = 'runtime.register'`)
	t.Cleanup(func() { dbfx.Exec(t, `DELETE FROM permission_policy WHERE action = 'runtime.register'`) })
	testutil.Call(t, h.GetRuntimePermissionPolicy, newRequest(http.MethodGet, path, nil)).Want(http.StatusForbidden)
	testutil.Call(t, h.UpdateRuntimePermissionPolicy, newRequest(http.MethodPut, path, map[string]any{"allowed_emails": []string{handlerTestEmail}, "revision": 0})).Want(http.StatusForbidden)
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	var initial RuntimePermissionPolicyResponse
	testutil.Call(t, h.GetRuntimePermissionPolicy, newRequest(http.MethodGet, path, nil)).Want(http.StatusOK).JSON(&initial)
	if initial.Revision != 0 || !initial.Restricted || len(initial.AllowedEmails) != 1 {
		t.Fatalf("unexpected initial policy: %+v", initial)
	}
	var saved RuntimePermissionPolicyResponse
	testutil.Call(t, h.UpdateRuntimePermissionPolicy, newRequest(http.MethodPut, path, map[string]any{
		"allowed_emails": []string{" HANDLER-TEST@MULTICA.AI ", handlerTestEmail}, "revision": 0,
	})).Want(http.StatusOK).JSON(&saved)
	if saved.Revision != 1 || len(saved.AllowedEmails) != 1 || saved.AllowedEmails[0] != handlerTestEmail {
		t.Fatalf("not normalized: %+v", saved)
	}
	if !h.runtimeRegistrationAllowed(t.Context(), handlerTestEmail) {
		t.Fatal("saved grant not enforced")
	}
	// A new handler has no policy cache and must observe durable changes.
	fresh := *testHandler
	fresh.cfg.RuntimeRegistrationRestricted = true
	if !fresh.runtimeRegistrationAllowed(t.Context(), handlerTestEmail) {
		t.Fatal("policy did not survive handler recreation")
	}
	testutil.Call(t, h.UpdateRuntimePermissionPolicy, newRequest(http.MethodPut, path, map[string]any{
		"allowed_emails": []string{"not-an-email"}, "revision": 1,
	})).Want(http.StatusBadRequest)
	testutil.Call(t, h.UpdateRuntimePermissionPolicy, newRequest(http.MethodPut, path, map[string]any{
		"allowed_emails": []string{}, "revision": 0,
	})).Want(http.StatusConflict)
	testutil.Call(t, h.UpdateRuntimePermissionPolicy, newRequest(http.MethodPut, path, map[string]any{
		"allowed_emails": []string{}, "revision": 1, "permission_managers": []string{handlerTestEmail},
	})).Want(http.StatusBadRequest)
	testutil.Call(t, h.UpdateRuntimePermissionPolicy, newRequest(http.MethodPut, path, map[string]any{
		"allowed_emails": []string{}, "revision": 1,
	})).Want(http.StatusOK)
	fresh.cfg.RuntimeRegistrationRestricted = false
	fresh.cfg.RuntimeRegistrationAllowedEmails = []string{handlerTestEmail}
	if fresh.runtimeRegistrationAllowed(t.Context(), handlerTestEmail) {
		t.Fatal("empty stored policy must deny even an environment grant")
	}
	var me UserResponse
	testutil.Call(t, h.GetMe, newRequest(http.MethodGet, "/api/me", nil)).Want(http.StatusOK).JSON(&me)
	if me.Permissions.RegisterRuntimes || !me.Permissions.ManagePermissionAccess {
		t.Fatal("manager and runtime capabilities must remain independent")
	}
	testutil.Call(t, h.UpdateRuntimePermissionPolicy, newRequest(http.MethodPut, path, map[string]any{
		"allowed_emails": []string{handlerTestEmail}, "revision": 1,
	})).Want(http.StatusConflict)
}

func TestRuntimePermissionPolicyDatabaseFailureDenies(t *testing.T) {
	h := *testHandler
	h.cfg.RuntimeRegistrationRestricted = false
	h.Queries = db.New(&mockDB{getUserErr: errors.New("database unavailable")})
	if h.runtimeRegistrationAllowed(t.Context(), handlerTestEmail) {
		t.Fatal("database failure must not allow registration")
	}
	req := newRequest(http.MethodPost, "/api/daemon/register", map[string]any{
		"workspace_id": testWorkspaceID, "daemon_id": "policy-db-error",
		"runtimes": []map[string]any{{"name": "blocked", "type": "codex"}},
	})
	testutil.Call(t, h.DaemonRegister, req).Want(http.StatusInternalServerError)
}

func TestAgentActionPolicies(t *testing.T) {
	h := *testHandler
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	actions := []workspacePermission{permissionCreateInstanceAgent, permissionCreateAgent, permissionEditAgent}
	for _, action := range actions {
		dbfx.Exec(t, "DELETE FROM permission_policy WHERE action=$1", string(action))
		t.Cleanup(func() { dbfx.Exec(t, "DELETE FROM permission_policy WHERE action=$1", string(action)) })
	}
	runtimeID := createClaimReclaimRuntime(t, t.Context(), "Permission runtime")
	var agent AgentResponse
	testutil.Call(t, h.CreateAgent, newRequest("POST", "/api/agents", CreateAgentRequest{Name: "Permission source", MaxConcurrentTasks: 1, RuntimeID: runtimeID})).Want(201).JSON(&agent)
	for _, tc := range []struct {
		action workspacePermission
		slug   string
	}{
		{permissionCreateInstanceAgent, "instance-agent-create"}, {permissionCreateAgent, "agent-create"}, {permissionEditAgent, "agent-edit"},
	} {
		t.Run(tc.slug, func(t *testing.T) {
			path := "/api/permission-policies/" + tc.slug
			request := withURLParam(newRequest("PUT", path, map[string]any{"allowed_emails": []string{}, "revision": 0}), "action", tc.slug)
			testutil.Call(t, h.UpdateRuntimePermissionPolicy, request).Want(200)
			if h.actionAllowed(t.Context(), handlerTestEmail, tc.action) {
				t.Fatal("manager bypassed explicit denial")
			}
			switch tc.action {
			case permissionCreateAgent:
				testutil.Call(t, h.CreateAgent, newRequest("POST", "/api/agents", CreateAgentRequest{Name: "Denied normal", MaxConcurrentTasks: 1, RuntimeID: runtimeID})).Want(403)
			case permissionCreateInstanceAgent:
				testutil.Call(t, h.CreateAgent, newRequest("POST", "/api/agents", CreateAgentRequest{Name: "Denied instance", MaxConcurrentTasks: 1, RuntimeID: runtimeID, InstanceAgent: true})).Want(403)
				testutil.Call(t, h.SaveInstanceAgent, newRequest("POST", "/api/instance/agents", map[string]any{"name": "Denied alternate"})).Want(403)
			case permissionEditAgent:
				testutil.Call(t, h.UpdateAgentEnv, withURLParam(newRequest("PUT", "/env", map[string]any{"custom_env": map[string]string{"KEY": "value"}}), "id", agent.ID)).Want(403)
				testutil.Call(t, h.SetAgentSkills, withURLParam(newRequest("PUT", "/skills", map[string]any{"skill_ids": []string{}}), "id", agent.ID)).Want(403)
				testutil.Call(t, h.AddAgentMcpServer, withURLParam(newRequest("POST", "/mcp", map[string]any{"server_id": "00000000-0000-0000-0000-000000000001"}), "id", agent.ID)).Want(403)
				testutil.Call(t, h.UpdateAgent, withURLParam(newRequest("PATCH", "/api/agents/"+agent.ID, map[string]any{"description": "blocked"}), "id", agent.ID)).Want(403)
				testutil.Call(t, h.ArchiveAgent, withURLParam(newRequest("POST", "/archive", nil), "id", agent.ID)).Want(403)
			}
			request = withURLParam(newRequest("PUT", path, map[string]any{"allowed_emails": []string{handlerTestEmail}, "revision": 1}), "action", tc.slug)
			testutil.Call(t, h.UpdateRuntimePermissionPolicy, request).Want(200)
			if !h.actionAllowed(t.Context(), handlerTestEmail, tc.action) {
				t.Fatal("saved grant missing")
			}
			testutil.Call(t, h.UpdateRuntimePermissionPolicy, withURLParam(newRequest("PUT", path, map[string]any{"allowed_emails": []string{}, "revision": 1}), "action", tc.slug)).Want(409)
		})
	}
	// Both creation policies denied blocks the AI builder as well.
	dbfx.Exec(t, "UPDATE permission_policy SET allowed_emails='{}' WHERE action IN ('agent.create','instance-agent.create')")
	testutil.Call(t, h.CreateAgentBuilderSession, newRequest("POST", "/builder", map[string]any{"runtime_id": runtimeID})).Want(403)
	dbfx.Exec(t, "UPDATE permission_policy SET allowed_emails=ARRAY[$1] WHERE action='instance-agent.create'", handlerTestEmail)
	// Instance creation is delegated without permission-management access.
	h.cfg.PermissionManagerEmails = nil
	var instance AgentResponse
	testutil.Call(t, h.CreateAgent, newRequest("POST", "/api/agents", CreateAgentRequest{Name: "Delegated instance", MaxConcurrentTasks: 1, RuntimeID: runtimeID, InstanceAgent: true})).Want(201).JSON(&instance)
	t.Cleanup(func() {
		dbfx.Exec(t, "DELETE FROM agent WHERE instance_agent_id=$1", instance.InstanceAgentID)
		dbfx.Exec(t, "DELETE FROM instance_agent WHERE id=$1", instance.InstanceAgentID)
	})
	testutil.Call(t, h.GetRuntimePermissionPolicy, withURLParam(newRequest("GET", "/policies", nil), "action", "agent-edit")).Want(403)
	testutil.Call(t, h.UpdateAgent, withURLParam(newRequest("PATCH", "/agents", map[string]any{"description": "allowed"}), "id", agent.ID)).Want(200)
}
