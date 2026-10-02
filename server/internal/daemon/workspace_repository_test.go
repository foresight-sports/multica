package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorkspaceRepositoryValidation(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", repo}, {"-C", repo, "remote", "add", "origin", "git@github.com:org/repo.git"}} {
		if out, e := exec.Command("git", args...).CombinedOutput(); e != nil {
			t.Fatalf("%s %v", out, e)
		}
	}
	ref := localDirectoryRef{LocalPath: repo, RepositoriesRoot: root, ExpectedRepository: "org/repo"}
	if e := validateWorkspaceRepository(ref); e != nil {
		t.Fatal(e)
	}
	ref.ExpectedRepository = "org/wrong"
	if validateWorkspaceRepository(ref) == nil {
		t.Fatal("wrong remote accepted")
	}
	ref.ExpectedRepository = "org/repo"
	ref.LocalPath = filepath.Join(repo, "nested")
	os.Mkdir(ref.LocalPath, 0700)
	if validateWorkspaceRepository(ref) == nil {
		t.Fatal("nested directory accepted")
	}
	ref.LocalPath = filepath.Join(root, "missing")
	if validateWorkspaceRepository(ref) == nil {
		t.Fatal("missing checkout accepted")
	}
	ref.LocalPath = repo
	ref.RepositoriesRoot = t.TempDir()
	if validateWorkspaceRepository(ref) == nil {
		t.Fatal("outside root accepted")
	}
}
