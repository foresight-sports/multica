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
