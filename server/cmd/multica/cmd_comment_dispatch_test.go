package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A 201 means the comment exists, not that its mentioned worker started.
// Compact output must retain that distinction without encouraging duplicate posts.
func TestCommentCompactDispatchReceipt(t *testing.T) {
	const issueID = "11111111-1111-4111-8111-111111111111"
	const agentID = "22222222-2222-4222-8222-222222222222"
	for _, tc := range []struct{ status, want string }{
		{"blocked", "No worker was dispatched"},
		{"queued", "actual state before saying the worker is running"},
		{"coalesced", "actual state before saying the worker is running"},
		{"deferred", "actual state before saying the worker is running"},
		{"", "Dispatch unconfirmed: no trigger receipt"},
		{"future_status", "Dispatch unconfirmed: unknown trigger status"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			posts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+issueID {
					_ = json.NewEncoder(w).Encode(map[string]any{"id": issueID, "identifier": "TST-1"})
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/api/issues/"+issueID+"/comments" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
					return
				}
				posts++
				result := map[string]any{"id": "saved-comment"}
				if tc.status != "" {
					result["trigger_outcomes"] = []any{map[string]any{"target_id": agentID, "status": tc.status, "reason_code": "invocation_not_allowed"}}
				}
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(result)
			}))
			defer srv.Close()
			setCLITestServerEnv(t, srv.URL)
			t.Setenv("MULTICA_TOKEN", "mat_test-token")
			cmd := newIssueCommentAddTestCmd()
			if err := cmd.Flags().Parse([]string{"--content", "[@Worker](mention://agent/" + agentID + ") implement", "--output", "table"}); err != nil {
				t.Fatal(err)
			}
			stderr := captureStderr(t)
			err := runIssueCommentAdd(cmd, []string{issueID})
			got := stderr.read()
			if err != nil || posts != 1 || !strings.Contains(got, "Comment added") || !strings.Contains(got, tc.want) {
				t.Fatalf("err=%v posts=%d receipt=%q; want %q", err, posts, got, tc.want)
			}
		})
	}
}
