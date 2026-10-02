package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServerUpdateDrainsBackgroundWorkspacePreparation(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "local"), 0700); err != nil {
		t.Fatal(err)
	}
	plan := workspacerepo.Plan{WorkspaceID: "workspace", Configuration: workspacerepo.Configuration{Root: root, Folder: "local", Mode: "in_place"}}
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	d, _ := updateReportDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]workspacerepo.Plan{plan})
			return
		}
		once.Do(func() { close(started); <-release })
		w.WriteHeader(http.StatusOK)
	})
	d.workspaces = map[string]*workspaceState{"workspace": {runtimeIDs: []string{"runtime"}}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var releaseOnce sync.Once
	finishPreparation := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(finishPreparation)
	go d.workspaceRepositoryLoop(ctx)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background preparation did not start")
	}
	if got := d.activeTasks.Load(); got != 0 {
		t.Fatalf("background preparation reported %d active agent tasks", got)
	}
	if d.trySetClaimBarrier() {
		t.Fatal("automatic update must not bypass active preparation")
	}
	result := make(chan serverUpdateAcquireResult, 1)
	go func() { result <- d.tryBeginServerUpdate(ctx) }()
	waitForServerUpdateBarrier(t, d)
	if d.tryEnterClaim() {
		t.Fatal("new work entered while an update was draining preparation")
	}
	select {
	case <-result:
		t.Fatal("update did not wait for preparation")
	default:
	}
	finishPreparation()
	select {
	case got := <-result:
		if got != serverUpdateAcquired {
			t.Fatalf("update result = %v, want acquired", got)
		}
	case <-time.After(time.Second):
		t.Fatal("update did not proceed after preparation finished")
	}
}

func TestPrepareWorkspaceFolderOnly(t *testing.T) {
	root := t.TempDir()
	plan := workspacerepo.Plan{Configuration: workspacerepo.Configuration{Root: root, Folder: "local", Mode: "in_place"}}
	report := func(string, string) {}
	if err := prepareWorkspaceRepository(context.Background(), plan, report, slog.Default()); err == nil {
		t.Fatal("missing folder accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "local")); !os.IsNotExist(err) {
		t.Fatal("folder-only preparation created a directory")
	}
	if err := os.Mkdir(filepath.Join(root, "local"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := prepareWorkspaceRepository(context.Background(), plan, report, slog.Default()); err != nil {
		t.Fatal(err)
	}
	plan.Folder = "../escape"
	if err := prepareWorkspaceRepository(context.Background(), plan, report, slog.Default()); err == nil {
		t.Fatal("path traversal accepted")
	}
}
func TestPrepareWorkspaceClonesMissingRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX Git wrapper")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source")
	for _, args := range [][]string{{"init", source}, {"-C", source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"}} {
		if out, err := exec.Command(realGit, args...).CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = '-c' ]; then\n case \" $* \" in *--recurse-submodules*) ;; *) exit 88;; esac\n exec \"$REAL_GIT\" -c \"url.$LOCAL_SOURCE.insteadOf=https://github.com/org/repo.git\" \"$@\"\nfi\nexec \"$REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REAL_GIT", realGit)
	t.Setenv("LOCAL_SOURCE", source)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	plan := workspacerepo.Plan{Configuration: workspacerepo.Configuration{Root: root, Folder: "repo", Repository: "org/repo", Mode: "worktree"}}
	states := []string{}
	if err := prepareWorkspaceRepository(context.Background(), plan, func(state, message string) { states = append(states, state) }, slog.Default()); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(states, "cloning") {
		t.Fatal(states)
	}
	sentinel := filepath.Join(root, "repo", "user.txt")
	os.WriteFile(sentinel, []byte("keep"), 0600)
	plan.Repository = "org/different"
	if err := prepareWorkspaceRepository(context.Background(), plan, func(string, string) {}, slog.Default()); err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("wrong existing checkout: %v", err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep" {
		t.Fatal("existing work changed")
	}
	entries, _ := filepath.Glob(filepath.Join(root, ".multica-clone-*"))
	if len(entries) > 0 {
		t.Fatal("clone staging leaked", entries)
	}
}

func TestPrepareWorkspaceCloneFailureLogs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake Git")
	}
	bin := t.TempDir()
	secret := "ghp_" + strings.Repeat("a", 36)
	script := "#!/bin/sh\necho 'fatal: Authentication failed for https://user:private-password@github.com/org/repo.git' >&2\necho '" + secret + "' >&2\nexit 128\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	root := t.TempDir()
	plan := workspacerepo.Plan{Configuration: workspacerepo.Configuration{Root: root, Folder: "repo", Repository: "org/repo", Mode: "worktree"}}
	err := prepareWorkspaceRepository(t.Context(), plan, func(string, string) {}, logger)
	if err == nil || !strings.Contains(err.Error(), "Git clone failed") {
		t.Fatal(err)
	}
	text := logs.String()
	for _, want := range []string{"Git clone attempt started", "Git clone failed", "Authentication failed", "exit status 128"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in logs", want)
		}
	}
	for _, secret := range []string{secret, "private-password"} {
		if strings.Contains(text, secret) {
			t.Fatal("credential leaked")
		}
	}
	if strings.Contains(err.Error(), "Authentication failed") {
		t.Fatal("diagnostic leaked into workspace status")
	}
	entries, _ := filepath.Glob(filepath.Join(root, ".multica-clone-*"))
	if len(entries) > 0 {
		t.Fatal("stage leaked")
	}
}
func TestCloneDiagnosticsBounded(t *testing.T) {
	var buffer cloneDiagnosticBuffer
	buffer.Write([]byte(strings.Repeat("x", 20000)))
	buffer.Write([]byte("\nfatal: repository not found\n"))
	if len(buffer.data) > 16384 || buffer.safeText() != "fatal: repository not found" {
		t.Fatal("invalid bounded diagnostic")
	}
}
