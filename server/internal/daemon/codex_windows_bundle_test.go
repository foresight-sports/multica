package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsCodexBundleDiscovery(t *testing.T) {
	root := t.TempDir()
	var expected string
	for i, name := range []string{"old", "new"} {
		path := filepath.Join(root, "OpenAI", "Codex", "bin", name, "codex.exe")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fake"), 0600); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(int64(i+1), 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		expected = path
	}
	paths := windowsCodexBundlePaths(root)
	if len(paths) != 2 || paths[0] != expected {
		t.Fatalf("paths=%v", paths)
	}
	if paths := windowsCodexBundlePaths(""); len(paths) != 0 {
		t.Fatalf("empty root resolved %v", paths)
	}
}
