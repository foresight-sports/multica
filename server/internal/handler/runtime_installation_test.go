package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestRuntimeInstallationPermissions(t *testing.T) {
	h := *testHandler
	h.cfg.RuntimeRegistrationRestricted = true
	h.cfg.RuntimeRegistrationAllowedEmails = nil
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	h.cfg.CloudflareClientID = "sample-id"
	h.cfg.CloudflareClientSecret = "test-secret'$(never-execute)"
	dbfx.Exec(t, `DELETE FROM permission_policy WHERE action = 'runtime.register'`)
	t.Cleanup(func() { dbfx.Exec(t, `DELETE FROM permission_policy WHERE action = 'runtime.register'`) })
	path := "/api/runtime-installation"
	request := func() *http.Request { return newRequest(http.MethodGet, path, nil) }
	// Permission-management access alone must never disclose the runtime secret.
	rr := httptest.NewRecorder()
	h.GetRuntimeInstallation(rr, request())
	if rr.Code != http.StatusForbidden || strings.Contains(rr.Body.String(), "test-secret") {
		t.Fatal("unauthorized secret disclosure")
	}
	if !strings.Contains(rr.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("sensitive response may be cached")
	}
	h.cfg.RuntimeRegistrationAllowedEmails = []string{handlerTestEmail}
	var response struct {
		Command string `json:"command"`
	}
	testutil.Call(t, h.GetRuntimeInstallation, request()).Want(http.StatusOK).JSON(&response)
	if !strings.Contains(response.Command, "'test-secret''$(never-execute)'") || strings.Contains(response.Command, "Read-Host") {
		t.Fatal("credentials were not safely embedded")
	}
	h.cfg.CloudflareClientSecret = ""
	testutil.Call(t, h.GetRuntimeInstallation, request()).Want(http.StatusServiceUnavailable)
	testutil.Call(t, h.GetRuntimeInstallation, httptest.NewRequest(http.MethodGet, path, nil)).Want(http.StatusUnauthorized)
}
