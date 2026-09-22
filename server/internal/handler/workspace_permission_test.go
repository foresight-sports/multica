package handler

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestDaemonRegister_UserPolicy(t *testing.T) {
	h := *testHandler
	h.cfg.RuntimeRegistrationRestricted = true
	for _, role := range []string{"member", "admin", "owner"} {
		for _, allowed := range []bool{false, true} {
			label := role + "/denied"
			if allowed {
				label = role + "/allowed"
			}
			t.Run(label, func(t *testing.T) {
				dbfx.Exec(t, `UPDATE member SET role = $1 WHERE user_id = $2 AND workspace_id = $3`, role, testUserID, testWorkspaceID)
				t.Cleanup(func() {
					dbfx.Exec(t, `UPDATE member SET role = 'owner' WHERE user_id = $1 AND workspace_id = $2`, testUserID, testWorkspaceID)
				})
				h.cfg.RuntimeRegistrationAllowedEmails = []string{"someone-else@foresightsports.com"}
				if allowed {
					h.cfg.RuntimeRegistrationAllowedEmails = []string{" HANDLER-TEST@MULTICA.AI "}
				}
				daemonID := "policy-" + uuid.NewString()
				body := map[string]any{"workspace_id": testWorkspaceID, "daemon_id": daemonID,
					"runtimes": []map[string]any{{"name": "policy-test", "type": "codex"}}}
				want := http.StatusForbidden
				if allowed {
					want = http.StatusOK
				}
				testutil.Call(t, h.DaemonRegister, newRequest(http.MethodPost, "/api/daemon/register", body)).Want(want)
				var count int
				if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM agent_runtime WHERE workspace_id = $1 AND daemon_id = $2`, testWorkspaceID, daemonID).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if !allowed && count != 0 {
					t.Fatal("denied registration wrote a runtime")
				}
				if allowed && count != 1 {
					t.Fatalf("authorized registration wrote %d runtimes", count)
				}
				dbfx.Exec(t, `DELETE FROM agent_runtime WHERE workspace_id = $1 AND daemon_id = $2`, testWorkspaceID, daemonID)
			})
		}
	}
	h.cfg.RuntimeRegistrationAllowedEmails = []string{handlerTestEmail}
	body := map[string]any{"workspace_id": testWorkspaceID, "daemon_id": "blocked-machine",
		"runtimes": []map[string]any{{"name": "blocked", "type": "codex"}}}
	t.Run("daemon token cannot spoof allowed user", func(t *testing.T) {
		req := newDaemonTokenRequest(http.MethodPost, "/api/daemon/register", body, testWorkspaceID, "blocked-machine")
		req.Header.Set("X-User-ID", testUserID)
		testutil.Call(t, h.DaemonRegister, req).Want(http.StatusForbidden)
	})
	t.Run("nonmember cannot enroll", func(t *testing.T) {
		other := map[string]any{"workspace_id": uuid.NewString(), "daemon_id": "wrong-workspace",
			"runtimes": []map[string]any{{"name": "blocked", "type": "codex"}}}
		testutil.Call(t, h.DaemonRegister, newRequest(http.MethodPost, "/api/daemon/register", other)).Want(http.StatusNotFound)
	})
	t.Run("empty list denies owner", func(t *testing.T) {
		h.cfg.RuntimeRegistrationAllowedEmails = nil
		testutil.Call(t, h.DaemonRegister, newRequest(http.MethodPost, "/api/daemon/register", body)).Want(http.StatusForbidden)
	})
}

func TestGetMeRuntimePermission(t *testing.T) {
	for _, tc := range []struct {
		name       string
		restricted bool
		emails     []string
		allowed    bool
	}{
		{"unrestricted", false, nil, true},
		{"allowed", true, []string{" HANDLER-TEST@MULTICA.AI "}, true},
		{"denied owner", true, []string{"test@foresightsports.com"}, false},
		{"empty list", true, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := *testHandler
			h.cfg.RuntimeRegistrationRestricted = tc.restricted
			h.cfg.RuntimeRegistrationAllowedEmails = tc.emails
			var response UserResponse
			testutil.Call(t, h.GetMe, newRequest(http.MethodGet, "/api/me", nil)).Want(http.StatusOK).JSON(&response)
			if response.Permissions.RegisterRuntimes != tc.allowed {
				t.Fatalf("register_runtimes = %v, want %v", response.Permissions.RegisterRuntimes, tc.allowed)
			}
		})
	}
}
