package handler

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/testutil"
	"net/http"
	"testing"
)

func directoryRequest(method, ws, user string, body any) *http.Request {
	r := newRequest(method, "/api/workspace-directory", body)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", ws)
	rc.URLParams.Add("userId", user)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

func TestWorkspaceDirectoryAccessLifecycle(t *testing.T) {
	h := *testHandler
	h.cfg.PermissionManagerEmails = nil
	ws := dbfx.Workspace(t, "Discoverable team", "directory-access-test")
	t.Cleanup(func() { dbfx.Exec(t, `DELETE FROM workspace_access_request WHERE workspace_id=$1`, ws) })
	var listing struct {
		Workspaces []workspaceDirectoryEntry `json:"workspaces"`
		CanCreate  bool                      `json:"can_create"`
	}
	testutil.Call(t, h.WorkspaceDirectory, newRequest("GET", "/api/workspace-directory?q=directory-access-test", nil)).Want(200).JSON(&listing)
	if listing.CanCreate || len(listing.Workspaces) != 1 || listing.Workspaces[0].Role != "" {
		t.Fatal("nonmember discovery must not grant membership or creation")
	}
	for range 2 {
		testutil.Call(t, h.RequestWorkspaceAccess, directoryRequest("POST", ws, "", nil)).Want(204)
	}
	var count int
	dbfx.QueryRow(t, `SELECT count(*) FROM workspace_access_request WHERE workspace_id=$1`, ws).Scan(&count)
	if count != 1 {
		t.Fatal("retries must not duplicate requests")
	}
	var reviews struct {
		Requests []workspaceAccessReview `json:"requests"`
	}
	testutil.Call(t, h.ListWorkspaceAccessRequests, newRequest("GET", "/api/workspace-access-requests", nil)).Want(200).JSON(&reviews)
	for _, review := range reviews.Requests {
		if review.WorkspaceID == ws {
			t.Fatal("requester can see admin reviews")
		}
	}
	testutil.Call(t, h.ReviewWorkspaceAccess, directoryRequest("POST", ws, testUserID, map[string]string{"decision": "approved"})).Want(403)
	testutil.Call(t, h.RequestWorkspaceAccess, directoryRequest("DELETE", ws, "", nil)).Want(204)
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	testutil.Call(t, h.ReviewWorkspaceAccess, directoryRequest("POST", ws, testUserID, map[string]string{"decision": "approved"})).Want(409)
	testutil.Call(t, h.RequestWorkspaceAccess, directoryRequest("POST", ws, "", nil)).Want(204)
	testutil.Call(t, h.ReviewWorkspaceAccess, directoryRequest("POST", ws, testUserID, map[string]string{"decision": "declined"})).Want(204)
	testutil.Call(t, h.RequestWorkspaceAccess, directoryRequest("POST", ws, "", nil)).Want(204)
	testutil.Call(t, h.ReviewWorkspaceAccess, directoryRequest("POST", ws, testUserID, map[string]string{"decision": "approved"})).Want(204)
	testutil.Call(t, h.ReviewWorkspaceAccess, directoryRequest("POST", ws, testUserID, map[string]string{"decision": "approved"})).Want(409)
	var role string
	dbfx.QueryRow(t, `SELECT role FROM member WHERE workspace_id=$1 AND user_id=$2`, ws, testUserID).Scan(&role)
	if role != "member" {
		t.Fatal("approval must grant only member")
	}
	testutil.Call(t, h.RequestWorkspaceAccess, directoryRequest("POST", ws, "", nil)).Want(409)
}

func TestWorkspaceDirectoryAdminBoundaries(t *testing.T) {
	h := *testHandler
	h.cfg.PermissionManagerEmails = nil
	ws := dbfx.Workspace(t, "Admin team", "directory-admin-test")
	other := dbfx.Workspace(t, "Unrelated team", "directory-unrelated-test")
	target := dbfx.User(t, "Requester", "requester-directory@example.test")
	dbfx.Member(t, ws, testUserID, "admin")
	dbfx.Exec(t, `INSERT INTO workspace_access_request(workspace_id,user_id) VALUES($1,$3),($2,$3)`, ws, other, target)
	t.Cleanup(func() { dbfx.Exec(t, `DELETE FROM workspace_access_request WHERE user_id=$1`, target) })
	testutil.Call(t, h.ReviewWorkspaceAccess, directoryRequest("POST", other, target, map[string]string{"decision": "approved"})).Want(403)
	testutil.Call(t, h.ReviewWorkspaceAccess, directoryRequest("POST", ws, target, map[string]string{"decision": "owner"})).Want(400)
	testutil.Call(t, h.ReviewWorkspaceAccess, directoryRequest("POST", ws, target, map[string]string{"decision": "approved"})).Want(204)
	testutil.Call(t, h.WorkspaceDirectory, newDaemonTokenRequest("GET", "/api/workspace-directory", nil, ws, "daemon")).Want(401)
}

func TestWorkspaceCreationPermission(t *testing.T) {
	h := *testHandler
	h.cfg.PermissionManagerEmails = nil
	dbfx.Exec(t, `DELETE FROM permission_policy WHERE action='workspace.create'`)
	t.Cleanup(func() { dbfx.Exec(t, `DELETE FROM permission_policy WHERE action='workspace.create'`) })
	testutil.Call(t, h.CreateWorkspace, newRequest("POST", "/api/workspaces", map[string]string{"name": "Denied", "slug": "denied-directory"})).Want(403)
	h.cfg.PermissionManagerEmails = []string{handlerTestEmail}
	if !h.actionAllowed(t.Context(), handlerTestEmail, permissionCreateWorkspace) {
		t.Fatal("manager should receive default grant")
	}
	dbfx.Exec(t, `INSERT INTO permission_policy(action,allowed_emails,updated_by) VALUES('workspace.create','{}',$1)`, testUserID)
	if h.actionAllowed(t.Context(), handlerTestEmail, permissionCreateWorkspace) {
		t.Fatal("explicit empty policy must override default")
	}
	testutil.Call(t, h.CreateWorkspace, newDaemonTokenRequest("POST", "/api/workspaces", nil, testWorkspaceID, "daemon")).Want(401)
}
