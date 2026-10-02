package workspacerepo

import "testing"

func TestConfigurationPathsAndIdentity(t *testing.T) {
	for _, root := range []string{"D:\\GitHub", "/srv/repos", "\\\\server\\repos"} {
		c := Configuration{Root: root}
		if e := c.Validate("machine"); e != nil {
			t.Fatal(e)
		}
	}
	for _, root := range []string{"relative", "/", "C:\\", "/srv/../etc"} {
		c := Configuration{Root: root}
		if c.Validate("machine") == nil {
			t.Fatalf("accepted %q", root)
		}
	}
	for _, folder := range []string{"../other", "a/b", "C:\\repo", "repo."} {
		c := Configuration{Repository: "org/repo", Folder: folder}
		if c.Validate("workspace") == nil {
			t.Fatal(folder)
		}
	}
	c := Configuration{Repository: "org/repo"}
	if e := c.Validate("workspace"); e != nil || c.Folder != "repo" || c.Mode != "worktree" {
		t.Fatal(c, e)
	}
	for _, remote := range []string{"git@github.com:Org/Repo.git", "https://github.com/org/repo.git", "ssh://git@github.com/org/repo.git"} {
		if GitHubIdentity(remote) != "org/repo" {
			t.Fatal(remote)
		}
	}
	for _, remote := range []string{"https://evil.example/org/repo", "https://github.com/org/../repo", "file:///org/repo"} {
		if GitHubIdentity(remote) != "" {
			t.Fatal(remote)
		}
	}
}

func TestFolderOnlyConfiguration(t *testing.T) {
	c := Configuration{Folder: "existing"}
	if err := c.Validate("workspace"); err != nil || c.Folder != "existing" || c.Mode != "in_place" {
		t.Fatal(c, err)
	}
	c = Configuration{}
	if err := c.Validate("workspace"); err != nil || c.Folder != "" {
		t.Fatal(c, err)
	}
}
