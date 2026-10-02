package workflow

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBranchRejectsInvalidGitRefs(t *testing.T) {
	for _, name := range []string{"-force", "a..b", "a/.hidden", "a.lock/b", "a.", "@", "a\x00b", "a b", "a@{1}", "/a"} {
		if ValidBranch(name) {
			t.Fatalf("invalid branch accepted: %q", name)
		}
	}
	if !ValidBranch("work/APP-3") {
		t.Fatal("valid work branch rejected")
	}
}
func TestToolsStayInCurrentUserEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", "")
	dir := filepath.Join(home, ".multica", "tools", "bin")
	os.MkdirAll(dir, 0700)
	name := "fixture-tool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	full := filepath.Join(dir, name)
	os.WriteFile(full, []byte("fixture"), 0700)
	found, err := FindTool(name)
	if err != nil || found != full {
		t.Fatal("durable install unavailable", found, err)
	}
	if _, err = FindTool("../fixture-tool"); err == nil {
		t.Fatal("path accepted as tool name")
	}
	for _, p := range filepath.SplitList(ToolPath()) {
		if !filepath.IsAbs(p) || strings.Contains(p, "other-user") {
			t.Fatal("nonlocal tool path", p)
		}
	}
}
