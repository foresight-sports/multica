package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/pkg/workflow"
)

func TestCheckpointRemoteVerificationAndResume(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL="+filepath.Join(root, "missing-config"))
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("git %v: %s %v", args, out, e)
		}
		return strings.TrimSpace(string(out))
	}
	git(root, "init", "--bare", origin)
	git(root, "clone", origin, first)
	git(first, "config", "user.email", "test@example.invalid")
	git(first, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(first, "code.txt"), []byte("one"), 0600)
	git(first, "add", "code.txt")
	git(first, "commit", "-m", "Initial")
	git(first, "switch", "-c", "work/APP-3")
	// A local transport alias exercises real Git without network credentials.
	git(first, "remote", "set-url", "origin", "https://github.com/example/repo")
	git(first, "config", "url."+filepath.ToSlash(origin)+".insteadOf", "https://github.com/example/repo")
	d := workflow.Data{Repository: "example/repo", Branch: "work/APP-3"}
	if _, e := verifiedRemoteCheckpoint(first, d); e == nil {
		t.Fatal("unpublished commit accepted")
	}
	git(first, "push", "origin", "HEAD:refs/heads/work/APP-3")
	var e error
	d.Commit, e = verifiedRemoteCheckpoint(first, d)
	if e != nil {
		t.Fatal(e)
	}
	git(root, "clone", "--branch", "work/APP-3", origin, second)
	git(second, "remote", "set-url", "origin", "https://github.com/example/repo")
	git(second, "config", "url."+filepath.ToSlash(origin)+".insteadOf", "https://github.com/example/repo")
	git(second, "switch", "-c", "temporary")
	if e = resumeWorkCheckpoint(second, d); e != nil {
		t.Fatal(e)
	}
	if got := git(second, "rev-parse", "HEAD"); got != d.Commit {
		t.Fatal("resumed wrong commit", got)
	}
	os.WriteFile(filepath.Join(second, "code.txt"), []byte("uncommitted"), 0600)
	if e = resumeWorkCheckpoint(second, d); e == nil {
		t.Fatal("dirty checkout overwritten")
	}
	os.WriteFile(filepath.Join(second, "code.txt"), []byte("one"), 0600)
	git(second, "config", "user.email", "test@example.invalid")
	git(second, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(second, "code.txt"), []byte("local work"), 0600)
	git(second, "add", "code.txt")
	git(second, "commit", "-m", "Local")
	os.WriteFile(filepath.Join(first, "code.txt"), []byte("remote work"), 0600)
	git(first, "add", "code.txt")
	git(first, "commit", "-m", "Remote")
	git(first, "push", "origin", "HEAD")
	if e = resumeWorkCheckpoint(second, d); e == nil {
		t.Fatal("divergent branch accepted")
	}
}
