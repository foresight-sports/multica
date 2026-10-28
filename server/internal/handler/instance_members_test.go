package handler

import (
	"github.com/multica-ai/multica/server/internal/testutil"
	"net/http"
	"testing"
)

func TestInstanceMembersRequiresManager(t *testing.T) {
	h := *testHandler
	h.cfg.PermissionManagerEmails = nil
	testutil.Call(t, h.ListInstanceMembers, newRequest(http.MethodGet, "/api/instance/members", nil)).Want(http.StatusForbidden)
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	email := "instance-directory-new@example.test"
	dbfx.User(t, "New account", email)
	var response struct {
		Members   []instanceMember `json:"members"`
		Truncated bool             `json:"truncated"`
	}
	testutil.Call(t, h.ListInstanceMembers, newRequest(http.MethodGet, "/api/instance/members", nil)).Want(http.StatusOK).JSON(&response)
	for _, member := range response.Members {
		if member.Email == email {
			if len(member.Workspaces) != 0 {
				t.Fatal("new account unexpectedly has memberships")
			}
			return
		}
	}
	t.Fatal("account without memberships missing from directory")
}
