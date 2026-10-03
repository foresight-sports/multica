package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadCapabilitiesOnlyUsesSupportedReadRPC(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fake")
	}
	path := filepath.Join(t.TempDir(), "fake-codex")
	script := `#!/bin/sh
[ "$1" = "app-server" ] || exit 31
read -r line
case "$line" in *initialize*) ;; *) exit 32;; esac
printf '%s\n' '{"id":1,"result":{}}'
read -r line
read -r line
case "$line" in *skills/list*) ;; *) exit 33;; esac
printf '%s\n' '{"id":2,"result":{"data":[{"skills":[{"name":"design","description":"Create designs","path":"/home/private/.codex/plugins/cache/market/design/1/skills/design/SKILL.md","enabled":true}],"errors":[]}]}}'
read -r line
case "$line" in *app/installed*) ;; *) exit 34;; esac
printf '%s\n' '{"id":3,"result":{"apps":[{"id":"app-1","runtimeName":"Design","enabled":true,"callable":false}]}}'
read -r line
exit 35
`
	if e := os.WriteFile(path, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	r, e := ReadCodexCapabilities(context.Background(), NewCommand(path, nil))
	if e != nil || r.Status != "reported" || len(r.Entries) != 3 {
		t.Fatalf("inventory: %+v %v", r, e)
	}
	if r.Entries[0].Name != "design@market" || r.Entries[1].Plugin != "design@market" || r.Entries[2].Callable == nil || *r.Entries[2].Callable || r.Entries[2].Auth != "unknown" {
		t.Fatal("incorrect capability state", r)
	}
}
