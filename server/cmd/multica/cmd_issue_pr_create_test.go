package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPRCreateStopsBeforeGitHubWhenJiraMissing(t *testing.T) {
	t.Chdir(t.TempDir())
	os.WriteFile("body.md", []byte("Test body"), 0600)
	checked := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pull-request-check") {
			checked = true
			w.WriteHeader(409)
			w.Write([]byte(`{"error":"Jira key required"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"11111111-1111-4111-8111-111111111111"}`))
	}))
	defer srv.Close()
	t.Setenv("MULTICA_SERVER_URL", srv.URL)
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
	t.Setenv("MULTICA_TOKEN", "mat_test-token")
	// No gh executable: a policy denial must be returned before any subprocess.
	t.Setenv("PATH", t.TempDir())
	cmd := newIssuePRCreateCmd()
	cmd.Flags().Set("title", "Test")
	cmd.Flags().Set("body-file", "body.md")
	cmd.Flags().Set("base", "dev")
	err := cmd.RunE(cmd, []string{"11111111-1111-4111-8111-111111111111"})
	if !checked || err == nil || !strings.Contains(err.Error(), "Jira key required") {
		t.Fatalf("policy not checked before gh: %v", err)
	}
}
