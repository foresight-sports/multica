package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskAPIGatewayAddsDaemonCredentials(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("CF-Access-Client-Id") != "host-id" || r.Header.Get("CF-Access-Client-Secret") != "host-secret" {
			t.Error("missing daemon credentials")
		}
		if r.Header.Get("Authorization") != "Bearer mat_test" {
			t.Error("lost task token")
		}
		w.WriteHeader(204)
	}))
	defer upstream.Close()
	original := http.DefaultTransport
	http.DefaultTransport = upstream.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	data, _ := json.Marshal(map[string]string{"origin": upstream.URL, "client_id": "host-id", "client_secret": "host-secret"})
	credentialPath := filepath.Join(t.TempDir(), "access.json")
	if err := os.WriteFile(credentialPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULTICA_CLOUDFLARE_ACCESS_FILE", credentialPath)
	d := &Daemon{cfg: Config{ServerBaseURL: upstream.URL}}
	d.registerActiveRepoCheckoutTask("mat_test", activeRepoCheckoutTask{WorkspaceID: "task-workspace"})
	r := httptest.NewRequest("GET", "/task-api/api/issues/test", nil)
	r.Header.Set("Authorization", "Bearer mat_test")
	w := httptest.NewRecorder()
	d.taskAPIHandler().ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("gateway status: %d", w.Code)
	}
}

func TestTaskAPIGatewayPreservesTaskAuthority(t *testing.T) {
	// A valid credential file for another origin must never leak over HTTP.
	credentialPath := filepath.Join(t.TempDir(), "access.json")
	if err := os.WriteFile(credentialPath, []byte(`{"origin":"https://multica.example","client_id":"host-id","client_secret":"host-secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULTICA_CLOUDFLARE_ACCESS_FILE", credentialPath)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.RequestURI() != "/api/issues/test?limit=2" {
			t.Errorf("target = %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer mat_test" {
			t.Error("task token replaced")
		}
		if r.Header.Get("X-Workspace-ID") != "task-workspace" {
			t.Error("workspace not bound to task")
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("CF-Access-Client-Secret") != "" {
			t.Error("untrusted credentials forwarded")
		}
		w.WriteHeader(204)
	}))
	defer upstream.Close()
	d := &Daemon{cfg: Config{ServerBaseURL: upstream.URL}}
	d.registerActiveRepoCheckoutTask("mat_test", activeRepoCheckoutTask{WorkspaceID: "task-workspace"})
	handler := d.taskAPIHandler()
	request := func(token, path string) int {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Workspace-ID", "other-workspace")
		r.Header.Set("Cookie", "owner=session")
		r.Header.Set("CF-Access-Client-Secret", "untrusted")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	if code := request("mat_test", "/task-api/api/issues/test?limit=2"); code != 204 {
		t.Fatalf("active task: %d", code)
	}
	if code := request("owner-token", "/task-api/api/issues/test"); code != 401 {
		t.Fatalf("owner token: %d", code)
	}
	if code := request("mat_test", "/task-api/not-api"); code != 404 {
		t.Fatalf("non API: %d", code)
	}
	d.clearActiveRepoCheckoutTask("mat_test")
	if code := request("mat_test", "/task-api/api/issues/test"); code != 401 {
		t.Fatalf("retired token: %d", code)
	}
	if calls != 1 {
		t.Fatalf("upstream calls: %d", calls)
	}
}
