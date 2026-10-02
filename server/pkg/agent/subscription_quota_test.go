package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestParseSubscriptionReports(t *testing.T) {
	now := time.Now()
	r, e := ParseCodexQuota([]byte(`{"rateLimits":{"primary":{"usedPercent":100,"windowDurationMins":300}},"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":0,"windowDurationMins":300},"secondary":{"windowDurationMins":10080}}}}`), now)
	if e != nil || len(r.Windows) != 2 || r.Windows[0].UsedPercent == nil || *r.Windows[0].UsedPercent != 0 || r.Windows[1].UsedPercent != nil || r.Blocks("m", now) {
		t.Fatalf("missing versus zero/map precedence: %+v %v", r, e)
	}
	c := ParseClaudeQuota([]byte(`{"status":"rejected","rateLimitType":"five_hour"}`), "m", now)
	if c == nil || !c.Blocks("other", now) || c.Windows[0].UsedPercent != nil {
		t.Fatal("Claude rejection without percentage")
	}
	if ParseClaudeQuota([]byte(`{"status":"bad"}`), "m", now) != nil {
		t.Fatal("invalid status accepted")
	}
}
func TestReadQuotaOnlyUsesAccountRPC(t *testing.T) {
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
case "$line" in *account/read*) ;; *) exit 33;; esac
printf '%s\n' '{"id":2,"result":{"account":{"type":"chatgpt"}}}'
read -r line
case "$line" in *account/rateLimits/read*) ;; *) exit 34;; esac
printf '%s\n' '{"id":3,"result":{"rateLimits":{"limitId":"codex","primary":{"usedPercent":72,"windowDurationMins":300}}}}'
read -r line
exit 35
`
	if e := os.WriteFile(path, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	r, e := ReadCodexQuota(context.Background(), NewCommand(path, nil))
	if e != nil || len(r.Windows) != 1 || *r.Windows[0].UsedPercent != 72 {
		t.Fatalf("report: %+v %v", r, e)
	}
}
