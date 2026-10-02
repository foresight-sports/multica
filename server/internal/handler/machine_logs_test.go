package handler

import (
	"github.com/multica-ai/multica/server/internal/testutil"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMachineLogsOwnershipAndSnapshot(t *testing.T) {
	id := dbfx.Runtime(t, "Logs", testutil.Cols{"daemon_id": "logs-test", "visibility": "public", "metadata": `{"machine_logs_version":1}`})
	id2 := dbfx.Runtime(t, "Logs second runtime", testutil.Cols{"provider": "codex", "daemon_id": "logs-test", "visibility": "public"})
	t.Cleanup(func() {
		testPool.Exec(t.Context(), "DELETE FROM machine_logs WHERE machine_key=$1", testUserID+":logs-test")
	})
	_, _, other := runtimeVisibilityFixture(t)
	for _, method := range []string{"GET", "POST"} {
		w := httptest.NewRecorder()
		r := withURLParam(newRequestAs(other, method, "/logs", map[string]any{"log": "forged"}), "runtimeId", id)
		if method == "GET" {
			testHandler.GetMachineLogs(w, r)
		} else {
			testHandler.ReportMachineLogs(w, r)
		}
		if w.Code != 403 {
			t.Fatalf("non-owner %s: %d", method, w.Code)
		}
	}
	secret := "ghp_" + strings.Repeat("a", 36)
	w := httptest.NewRecorder()
	testHandler.ReportMachineLogs(w, withURLParam(newRequest("POST", "/logs", map[string]any{"log": "hello " + secret, "crash": "crash diagnostic"}), "runtimeId", id))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	testHandler.GetMachineLogs(w, withURLParam(newRequest("GET", "/logs", nil), "runtimeId", id2))
	if w.Code != 200 || strings.Contains(w.Body.String(), secret) || !strings.Contains(w.Body.String(), "REDACTED") || !strings.Contains(w.Body.String(), "crash diagnostic") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	testHandler.ReportMachineLogs(w, withURLParam(newRequest("POST", "/logs", map[string]any{"log": strings.Repeat("x", 65537)}), "runtimeId", id))
	if w.Code != 400 {
		t.Fatalf("oversize accepted: %d", w.Code)
	}
}

func TestMachineLogsRejectTaskCredentials(t *testing.T) {
	r := newRequest("GET", "/logs", nil)
	r.Header.Set("X-Actor-Source", "task_token")
	w := httptest.NewRecorder()
	testHandler.GetMachineLogs(w, r)
	if w.Code != 403 {
		t.Fatalf("task credential accepted: %d", w.Code)
	}
}
