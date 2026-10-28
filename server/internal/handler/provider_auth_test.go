package handler

import (
	"encoding/json"
	"github.com/multica-ai/multica/server/internal/testutil"
	"net/http/httptest"
	"testing"
)

func TestProviderAuthOwnerLifecycle(t *testing.T) {
	id := dbfx.Runtime(t, "Auth setup", testutil.Cols{"daemon_id": "auth-test", "visibility": "public", "metadata": `{"provider_auth_version":1,"os":"windows"}`})
	t.Cleanup(func() { testPool.Exec(t.Context(), "DELETE FROM provider_auth_requests WHERE runtime_id=$1", id) })
	_, _, other := runtimeVisibilityFixture(t)
	for _, method := range []string{"GET", "POST"} {
		w := httptest.NewRecorder()
		testHandler.ProviderAuth(w, withURLParam(newRequestAs(other, method, "/provider-auth", map[string]string{"provider": "claude"}), "runtimeId", id))
		if w.Code != 403 {
			t.Fatalf("non-owner %s: %d", method, w.Code)
		}
	}
	post := func(provider string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		testHandler.ProviderAuth(w, withURLParam(newRequest("POST", "/provider-auth", map[string]string{"provider": provider}), "runtimeId", id))
		return w
	}
	if w := post("arbitrary-command"); w.Code != 400 {
		t.Fatalf("invalid provider: %d", w.Code)
	}
	if w := post("claude"); w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	if w := post("codex"); w.Code != 409 {
		t.Fatalf("duplicate: %d", w.Code)
	}
	if testHandler.claimProviderAuth(t.Context(), id, other, "") != nil {
		t.Fatal("another owner claimed setup")
	}
	if testHandler.claimProviderAuth(t.Context(), id, "", "wrong-daemon") != nil {
		t.Fatal("another machine claimed setup")
	}
	action := testHandler.claimProviderAuth(t.Context(), id, testUserID, "")
	if action == nil || action.Provider != "claude" {
		t.Fatal("missing action")
	}
	if testHandler.claimProviderAuth(t.Context(), id, testUserID, "") != nil {
		t.Fatal("action claimed twice")
	}
	report := func(requestID, status string) {
		w := httptest.NewRecorder()
		testHandler.ReportProviderAuth(w, withURLParam(newRequest("POST", "/provider-auth", map[string]string{"id": requestID, "status": status}), "runtimeId", id))
		if w.Code != 200 {
			t.Fatalf("report: %d %s", w.Code, w.Body.String())
		}
	}
	report(action.ID, "completed")
	forged := httptest.NewRecorder()
	testHandler.ReportProviderAuth(forged, withURLParam(newRequestAs(other, "POST", "/provider-auth", map[string]string{"id": action.ID, "status": "completed"}), "runtimeId", id))
	if forged.Code != 403 {
		t.Fatalf("non-owner reported setup: %d", forged.Code)
	}
	if w := post("codex"); w.Code != 202 {
		t.Fatalf("second setup: %d", w.Code)
	}
	report(action.ID, "failed") // An old helper cannot overwrite a new request.
	w := httptest.NewRecorder()
	testHandler.ProviderAuth(w, withURLParam(newRequest("GET", "/provider-auth", nil), "runtimeId", id))
	var state map[string]string
	if json.Unmarshal(w.Body.Bytes(), &state) != nil || state["status"] != "pending" || state["provider"] != "codex" {
		t.Fatal(w.Body.String())
	}
	testPool.Exec(t.Context(), "UPDATE provider_auth_requests SET expires_at=now()-interval '1 second' WHERE runtime_id=$1", id)
	if testHandler.claimProviderAuth(t.Context(), id, testUserID, "") != nil {
		t.Fatal("expired action executed")
	}
}

func TestProviderAuthRejectsTaskActor(t *testing.T) {
	r := newRequest("POST", "/provider-auth", nil)
	r.Header.Set("X-Actor-Source", "task_token")
	w := httptest.NewRecorder()
	testHandler.ProviderAuth(w, r)
	if w.Code != 403 {
		t.Fatalf("task actor accepted: %d", w.Code)
	}
}
