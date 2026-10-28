package handler

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/testutil"
	"net/http"
	"testing"
)

func membershipRequest(user, workspace, role string) *http.Request {
	r := newRequest(http.MethodPut, "/api/instance/members/"+user+"/workspaces/"+workspace, map[string]string{"role": role})
	rc := chi.NewRouteContext()
	rc.URLParams.Add("userId", user)
	rc.URLParams.Add("workspaceId", workspace)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

func TestInstanceMembershipLifecycle(t *testing.T) {
	h := *testHandler
	h.cfg.PermissionManagerEmails = nil
	user := dbfx.User(t, "Directory member", "directory-edit@example.test")
	ws := dbfx.Workspace(t, "Directory workspace", "directory-edit")
	dbfx.Member(t, ws, testUserID, "owner")
	testutil.Call(t, h.SetInstanceMembership, membershipRequest(user, ws, "member")).Want(403)
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	testutil.Call(t, h.SetInstanceMembership, membershipRequest(user, ws, "invalid")).Want(400)
	testutil.Call(t, h.SetInstanceMembership, membershipRequest(user, ws, "member")).Want(200)
	var role string
	var onboarded bool
	dbfx.QueryRow(t, `SELECT m.role,u.onboarded_at IS NOT NULL FROM member m JOIN "user" u ON u.id=m.user_id WHERE m.user_id=$1 AND m.workspace_id=$2`, user, ws).Scan(&role, &onboarded)
	if role != "member" || !onboarded {
		t.Fatalf("membership or onboarding not saved: %s %v", role, onboarded)
	}
	testutil.Call(t, h.SetInstanceMembership, membershipRequest(user, ws, "admin")).Want(200)
	testutil.Call(t, h.SetInstanceMembership, membershipRequest(testUserID, ws, "none")).Want(409)
	testutil.Call(t, h.SetInstanceMembership, membershipRequest(testUserID, ws, "member")).Want(409)
	testutil.Call(t, h.SetInstanceMembership, membershipRequest(user, ws, "none")).Want(200)
	var count int
	dbfx.QueryRow(t, `SELECT count(*) FROM member WHERE user_id=$1 AND workspace_id=$2`, user, ws).Scan(&count)
	if count != 0 {
		t.Fatal("removed membership remains")
	}
}

func TestInstanceMembershipConcurrentOwnerDemotions(t *testing.T) {
	ws := dbfx.Workspace(t, "Owner race", "directory-owner-race")
	first := dbfx.User(t, "First owner", "directory-owner-one@example.test")
	second := dbfx.User(t, "Second owner", "directory-owner-two@example.test")
	ids := []string{dbfx.Member(t, ws, first, "owner"), dbfx.Member(t, ws, second, "owner")}
	results := make(chan error, 2)
	for _, id := range ids {
		member, err := testHandler.Queries.GetMember(context.Background(), parseUUID(id))
		if err != nil {
			t.Fatal(err)
		}
		go func() { _, err := testHandler.setMemberRole(context.Background(), member, "member"); results <- err }()
	}
	success, blocked := 0, 0
	for range ids {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, errLastWorkspaceOwner) {
			blocked++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || blocked != 1 {
		t.Fatalf("owner invariant failed: success=%d blocked=%d", success, blocked)
	}
}
