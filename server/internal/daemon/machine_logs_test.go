package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMachineLogsTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	secret := "ghp_" + strings.Repeat("a", 36)
	if err := os.WriteFile(path, []byte(strings.Repeat("old line\n", 100)+"new "+secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	text, truncated, err := readMachineLogTail(path, 100)
	if err != nil || !truncated || strings.Contains(text, secret) || !strings.Contains(text, "REDACTED") || len(text) > 100 || !utf8.ValidString(text) {
		t.Fatalf("invalid tail: %q %v %v", text, truncated, err)
	}
	// Rotation/replacement is picked up on the next read.
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("restarted\n"), 0600)
	text, truncated, err = readMachineLogTail(path, 100)
	if err != nil || truncated || text != "restarted\n" {
		t.Fatalf("replacement: %q %v", text, err)
	}
	text, _, err = readMachineLogTail(path+".missing", 100)
	if err != nil || text != "" {
		t.Fatal("missing logs should be empty", err)
	}
}
