package handler

import (
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/pkg/daemonrelease"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func releaseFixture(t *testing.T) *Handler {
	t.Helper()
	h := &Handler{cfg: Config{DaemonReleaseDir: t.TempDir()}}
	m := daemonrelease.Manifest{Version: "0.4.44-foresight.11", Assets: map[string]daemonrelease.Asset{"windows-amd64": {SHA256: strings.Repeat("a", 64), Size: 4}}}
	raw, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(h.cfg.DaemonReleaseDir, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return h
}
func TestInstanceUpdateStatusGuards(t *testing.T) {
	h := releaseFixture(t)
	for _, tc := range []struct {
		name, version, arch, launcher, status, reason string
		protocol                                      int
		edit, stale, can                              bool
	}{{"ready", "0.4.44-foresight.10", "amd64", "", "online", "", 1, true, false, true}, {"old", "0.4.44-foresight.4", "amd64", "", "online", "bootstrap_required", 0, true, false, false}, {"stale capability", "0.4.44-foresight.4", "amd64", "", "online", "bootstrap_required", 1, true, false, false}, {"read only", "0.4.44-foresight.10", "amd64", "", "online", "read_only", 1, false, false, false}, {"stale heartbeat", "0.4.44-foresight.10", "amd64", "", "online", "offline", 1, true, true, false}, {"offline", "0.4.44-foresight.10", "amd64", "", "offline", "offline", 1, true, false, false}, {"desktop", "0.4.44-foresight.10", "amd64", "desktop", "online", "managed_by_desktop", 1, true, false, false}, {"no artifact", "0.4.44-foresight.10", "arm64", "", "online", "platform_unavailable", 1, true, false, false}, {"current", "0.4.44-foresight.11", "amd64", "", "online", "current", 1, true, false, false}, {"newer", "0.4.44-foresight.12", "amd64", "", "online", "unrecognized_or_newer", 1, true, false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			meta, _ := json.Marshal(map[string]any{"cli_version": tc.version, "os": "windows", "arch": tc.arch, "launched_by": tc.launcher, "instance_update_version": tc.protocol})
			seen := time.Now()
			if tc.stale {
				seen = seen.Add(-5 * time.Minute)
			}
			s := h.instanceUpdateStatus(db.AgentRuntime{Metadata: meta, Status: tc.status, LastSeenAt: pgtype.Timestamptz{Time: seen, Valid: true}}, tc.edit)
			if s.CanUpdate != tc.can || s.Reason != tc.reason {
				t.Fatalf("%+v", s)
			}
		})
	}
}
func TestInstanceReleaseDownload(t *testing.T) {
	h := releaseFixture(t)
	dir := filepath.Join(h.cfg.DaemonReleaseDir, "0.4.44-foresight.11")
	os.Mkdir(dir, 0700)
	os.WriteFile(filepath.Join(dir, "multica-windows-amd64.exe"), []byte("data"), 0600)
	for _, tc := range []struct {
		version, platform string
		status            int
	}{{"0.4.44-foresight.11", "windows", 200}, {"0.4.44-foresight.10", "windows", 404}, {"0.4.44-foresight.11", "../windows", 404}} {
		r := withURLParams(httptest.NewRequest("GET", "/release", nil), "version", tc.version)
		r = withURLParams(r, "os", tc.platform)
		r = withURLParams(r, "arch", "amd64")
		w := httptest.NewRecorder()
		h.DownloadDaemonRelease(w, r)
		if w.Code != tc.status {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}
func TestInstanceUpdateInitiationRequiresPublishedSupportedTarget(t *testing.T) {
	h := releaseFixture(t)
	h.Queries = testHandler.Queries
	h.UpdateStore = NewInMemoryUpdateStore()
	id := createClaimReclaimRuntime(t, t.Context(), "Instance updater")
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_runtime SET status='online',last_seen_at=now(),metadata='{"cli_version":"0.4.44-foresight.10","os":"windows","arch":"amd64","instance_update_version":1}' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"v0.4.99", "0.4.44-foresight.12", "0.4.44-foresight.10"} {
		w := httptest.NewRecorder()
		r := withURLParam(newRequest("POST", "/update", map[string]string{"target_version": target}), "runtimeId", id)
		h.InitiateUpdate(w, r)
		if w.Code != 409 {
			t.Fatalf("target %s: %d %s", target, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	r := withURLParam(newRequest("GET", "/update-status", nil), "runtimeId", id)
	h.GetRuntimeUpdateStatus(w, r)
	if w.Code != 200 {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	var status runtimeUpdateStatus
	json.Unmarshal(w.Body.Bytes(), &status)
	if !status.CanUpdate {
		t.Fatalf("status: %+v", status)
	}
	w = httptest.NewRecorder()
	r = withURLParam(newRequest("POST", "/update", map[string]string{"target_version": "0.4.44-foresight.11"}), "runtimeId", id)
	h.InitiateUpdate(w, r)
	if w.Code != 200 {
		t.Fatalf("valid update: %d %s", w.Code, w.Body.String())
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_runtime SET metadata=metadata || '{"cli_version":"0.4.44-foresight.4"}'::jsonb WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r = withURLParam(newRequest("POST", "/update", map[string]string{"target_version": "0.4.44-foresight.11"}), "runtimeId", id)
	h.InitiateUpdate(w, r)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "bootstrap_required") {
		t.Fatalf("old daemon: %d %s", w.Code, w.Body.String())
	}
}
