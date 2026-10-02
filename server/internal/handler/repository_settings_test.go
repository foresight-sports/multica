package handler

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"net/http/httptest"
	"testing"
)

func TestWorkspaceRepositorySettingsAndClaim(t *testing.T) {
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM repository_configuration WHERE subject=$1 OR subject=$2", testWorkspaceID, testUserID+":repo-test")
	})
	save := func(revision int, want int) {
		w := httptest.NewRecorder()
		r := withURLParam(newRequest("PUT", "/repository-settings", map[string]any{"revision": revision, "repository": "org/repo"}), "id", testWorkspaceID)
		testHandler.RepositorySettings(w, r)
		if w.Code != want {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	save(0, 200)
	save(0, 409)
	save(1, 200)
	rt := db.AgentRuntime{OwnerID: parseUUID(testUserID), DaemonID: pgtype.Text{String: "repo-test", Valid: true}, Metadata: []byte(`{"workspace_repository_version":2}`)}
	resp := AgentTaskResponse{WorkspaceID: testWorkspaceID}
	if testHandler.applyWorkspaceRepository(t.Context(), &resp, rt, false) == nil {
		t.Fatal("old daemon allowed")
	}
	if testHandler.applyWorkspaceRepository(t.Context(), &resp, rt, true) == nil {
		t.Fatal("missing root allowed")
	}
	_, e := testHandler.Queries.SaveRepositoryConfiguration(t.Context(), db.SaveRepositoryConfigurationParams{Scope: "machine", Subject: machineRepositoryKey(rt), Config: []byte(`{"root":"D:/GitHub"}`)})
	if e != nil {
		t.Fatal(e)
	}
	if e = testHandler.applyWorkspaceRepository(t.Context(), &resp, rt, true); e != nil {
		t.Fatal(e)
	}
	var ref map[string]string
	json.Unmarshal(resp.ProjectResources[0].ResourceRef, &ref)
	if ref["local_path"] != "D:/GitHub/repo" || ref["execution_mode"] != "worktree" || ref["expected_repository"] != "org/repo" {
		t.Fatal(ref)
	}
	ref["local_path"] = "D:/GitHub/other"
	resp.ProjectResources[0].ResourceRef, _ = json.Marshal(ref)
	if testHandler.applyWorkspaceRepository(t.Context(), &resp, rt, true) == nil {
		t.Fatal("conflicting project path accepted")
	}
}

func TestMachineRepositorySettingsOwnershipAndSharedIdentity(t *testing.T) {
	id := dbfx.Runtime(t, "Repository machine", testutil.Cols{"daemon_id": "shared-root-test", "runtime_mode": "local", "visibility": "public"})
	id2 := dbfx.Runtime(t, "Repository machine second runtime", testutil.Cols{"provider": "codex", "daemon_id": "shared-root-test", "runtime_mode": "local", "visibility": "public"})
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM repository_configuration WHERE scope='machine' AND subject=$1", testUserID+":shared-root-test")
	})
	w := httptest.NewRecorder()
	testHandler.RepositorySettings(w, withURLParam(newRequest("PUT", "/repository-settings", map[string]any{"revision": 0, "root": "D:/GitHub"}), "runtimeId", id))
	if w.Code != 200 {
		t.Fatalf("owner save: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	testHandler.RepositorySettings(w, withURLParam(newRequest("GET", "/repository-settings", nil), "runtimeId", id2))
	var config repositorySettings
	json.Unmarshal(w.Body.Bytes(), &config)
	if w.Code != 200 || config.Root != "D:/GitHub" || config.Revision != 1 {
		t.Fatalf("shared root: %d %s", w.Code, w.Body.String())
	}
	_, _, other := runtimeVisibilityFixture(t)
	w = httptest.NewRecorder()
	testHandler.RepositorySettings(w, withURLParam(newRequestAs(other, "PUT", "/repository-settings", map[string]any{"revision": 1, "root": "D:/Other"}), "runtimeId", id))
	if w.Code != 403 {
		t.Fatalf("non-owner save: %d %s", w.Code, w.Body.String())
	}
}
