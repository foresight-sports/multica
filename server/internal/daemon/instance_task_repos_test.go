package daemon

import "testing"

func TestInstanceTaskReposWithoutWorkspaceEnrollment(t *testing.T) {
	d := &Daemon{workspaces: make(map[string]*workspaceState)}
	d.registerTaskRepos("target", "first", []RepoData{{URL: "https://example.test/one.git", Ref: "main"}})
	d.registerTaskRepos("target", "second", []RepoData{{URL: "https://example.test/two.git", Ref: "release"}})
	if len(d.workspaces) != 0 {
		t.Fatal("instance task enrolled a watched workspace")
	}
	if !d.workspaceRepoAllowed("target", "https://example.test/one.git") || d.workspaceRepoAllowed("other", "https://example.test/one.git") {
		t.Fatal("claim repo scope was not honored")
	}
	if d.taskRepoDefaultRef("target", "second", "https://example.test/two.git") != "release" {
		t.Fatal("claim repo ref was lost")
	}
	d.clearTaskRepoRefs("target", "first")
	if d.workspaceRepoAllowed("target", "https://example.test/one.git") || !d.workspaceRepoAllowed("target", "https://example.test/two.git") {
		t.Fatal("cleanup did not preserve only the active task")
	}
	d.clearTaskRepoRefs("target", "second")
	if len(d.taskWorkspaces) != 0 {
		t.Fatal("task repo grants survived completion")
	}
}
