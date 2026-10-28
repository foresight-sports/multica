package execenv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedBaselinePreservesStaleLocalCheckout(t *testing.T) {
	remote := newTestRepo(t)
	file := filepath.Join(remote, "source.txt")
	if err := os.WriteFile(file, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "source.txt"}, {"commit", "-m", "old"}, {"branch", "dev"}} {
		if out, err := runGit(remote, args...); err != nil {
			t.Fatalf("%s %v", out, err)
		}
	}
	local := filepath.Join(t.TempDir(), "local")
	if out, err := runGit(remote, "clone", "--no-hardlinks", remote, local); err != nil {
		t.Fatalf("%s %v", out, err)
	}
	for _, args := range [][]string{{"config", "user.name", "Test"}, {"config", "user.email", "test@example.invalid"}, {"config", "commit.gpgsign", "false"}} {
		if _, err := runGit(local, args...); err != nil {
			t.Fatal(err)
		}
	}
	old, _ := runGitTrimmed(local, "rev-parse", "HEAD")
	params := LocalWorktreeParams{LocalPath: local, SharedBase: old, EnvRoot: t.TempDir(), AgentName: "test", TaskID: "11112222-3333-4444-5555-666677778888", ConversationKey: "app-4", WorkspaceID: "workspace", AgentID: "agent", ConversationID: "issue"}
	prior, err := PrepareLocalWorktree(params, worktreeTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	priorBranch := prior.Branch
	removeLocalWorktreeDir(prior.GitRoot, prior.Path, worktreeTestLogger())
	if _, err := runGit(remote, "checkout", "dev"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("current"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "source.txt"}, {"commit", "-m", "current"}} {
		if out, err := runGit(remote, args...); err != nil {
			t.Fatalf("%s %v", out, err)
		}
	}
	if err := os.WriteFile(filepath.Join(local, "source.txt"), []byte("personal edit"), 0644); err != nil {
		t.Fatal(err)
	}
	sha, err := FetchSharedBase(context.Background(), local, "dev", "11112222-3333-4444-5555-666677778888")
	if err != nil {
		t.Fatal(err)
	}
	params.SharedBase = sha
	params.TaskID = "22223333-3333-4444-5555-666677778888"
	wt, err := PrepareLocalWorktree(params, worktreeTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Finalize(worktreeTestLogger())
	if wt.Branch == priorBranch {
		t.Fatal("obsolete branch reused")
	}
	if _, err := runGitTrimmed(local, "rev-parse", "--verify", "refs/heads/"+priorBranch); err != nil {
		t.Fatal("prior branch lost", err)
	}
	if got, _ := os.ReadFile(filepath.Join(wt.Path, "source.txt")); string(got) != "current" {
		t.Fatalf("task saw %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(local, "source.txt")); string(got) != "personal edit" {
		t.Fatalf("local edit changed: %q", got)
	}
	if got, _ := runGitTrimmed(local, "rev-parse", "HEAD"); got != old {
		t.Fatal("local branch moved")
	}
	if wt.SourceCommit != sha || wt.BaseCommit == sha {
		t.Fatal("source and task snapshot provenance lost")
	}
}
