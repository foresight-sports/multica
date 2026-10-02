package gitsubmodule

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPrepareInitializesNestedPinnedSubmodules(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	// File remotes are allowed only in this local fixture, never in production.
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	git := func(dir string, args ...string) {
		t.Helper()
		prefix := []string{"-c", "core.longpaths=true", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-C", dir}
		if out, err := exec.Command("git", append(prefix, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	newRepo := func() string {
		p := t.TempDir()
		git(p, "init")
		os.WriteFile(filepath.Join(p, "fixture.txt"), []byte("pinned"), 0600)
		git(p, "add", ".")
		git(p, "commit", "-m", "initial")
		return p
	}
	leaf, middle, parent := newRepo(), newRepo(), newRepo()
	git(middle, "submodule", "add", leaf, "nested")
	git(middle, "commit", "-am", "nested dependency")
	git(parent, "submodule", "add", middle, "dependency")
	git(parent, "commit", "-am", "dependency")
	// The dependency advances after the superproject pinned it.
	os.WriteFile(filepath.Join(leaf, "fixture.txt"), []byte("newer"), 0600)
	git(leaf, "commit", "-am", "newer dependency")
	checkout := filepath.Join(t.TempDir(), "checkout")
	git(parent, "worktree", "add", "--detach", checkout, "HEAD")
	if err := Prepare(context.Background(), checkout, nil); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(checkout, "dependency", "nested", "fixture.txt")
	if data, err := os.ReadFile(file); err != nil || string(data) != "pinned" {
		t.Fatalf("nested pinned content missing: %q %v", data, err)
	}
	os.WriteFile(file, []byte("local edit"), 0600)
	if err := Prepare(context.Background(), checkout, nil); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(file); string(data) != "local edit" {
		t.Fatal("local edits overwritten")
	}
	// A missing remote must fail instead of claiming the checkout is ready.
	git(parent, "config", "-f", ".gitmodules", "submodule.dependency.url", filepath.Join(t.TempDir(), "missing"))
	git(parent, "commit", "-am", "missing remote")
	broken := filepath.Join(t.TempDir(), "broken")
	git(parent, "clone", "--no-local", parent, broken)
	if err := Prepare(context.Background(), broken, nil); err == nil {
		t.Fatal("missing submodule accepted")
	}
}
