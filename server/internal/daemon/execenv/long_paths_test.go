package execenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeSupportsLongPathsWithoutChangingRepositoryConfig(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := runGit(repo, "config", "--local", "core.longpaths", "false"); err != nil {
		t.Fatal(err)
	}
	relative := filepath.Join(strings.Repeat("nested", 20), strings.Repeat("golden", 20), "fixture.png")
	file := filepath.Join(repo, relative)
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "--", relative}, {"commit", "-m", "long path fixture"}} {
		if out, err := runGit(repo, args...); err != nil {
			t.Fatalf("prepare: %s %v", out, err)
		}
	}
	worktree := filepath.Join(t.TempDir(), "task-worktree")
	if err := runGitWorktreeAdd(repo, worktree, "long-path-fixture", "HEAD"); err != nil {
		t.Fatal(err)
	}
	defer removeGitWorktree(repo, worktree, "long-path-fixture", worktreeTestLogger())
	if raw, err := os.ReadFile(filepath.Join(worktree, relative)); err != nil || string(raw) != "fixture" {
		t.Fatalf("checkout: %v", err)
	}
	if out, err := runGitTrimmed(repo, "config", "--local", "--get", "core.longpaths"); err != nil || out != "false" {
		t.Fatalf("changed repository config: %s %v", out, err)
	}
}
