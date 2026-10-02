package agent

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestRoutingClaudeDisablesToolsAndInheritedSettings(t *testing.T) {
	args := buildClaudeArgs(ExecOptions{RoutingOnly: true, MaxTurns: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	joined := strings.Join(args, "|")
	for _, want := range []string{"--tools||", "--strict-mcp-config", "--setting-sources|", "disableAllHooks"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing selector isolation %q: %v", want, args)
		}
	}
}
