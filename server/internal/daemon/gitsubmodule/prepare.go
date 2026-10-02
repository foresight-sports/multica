// Package gitsubmodule prepares the commits pinned by a checkout's gitlinks.
package gitsubmodule

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/daemon/processtree"
	"github.com/multica-ai/multica/server/pkg/redact"
)

// Prepare initializes nested submodules at their recorded commits. No --remote
// or --force: never advance dependencies or discard local submodule edits.
func Prepare(ctx context.Context, path string, logger *slog.Logger) error {
	if _, err := os.Stat(filepath.Join(path, ".gitmodules")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect submodules: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if logger != nil {
		logger.Info("Initializing Git submodules recursively", "path", path)
	}
	cmd := exec.Command("git", "-c", "core.longpaths=true", "-c", "credential.interactive=false", "-C", path, "submodule", "update", "--init", "--recursive")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	var output boundedOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	err := processtree.Run(ctx, cmd, 5*time.Second)
	if err != nil {
		out := output.data
		if output.truncated {
			if end := bytes.LastIndexByte(out, '\n'); end >= 0 {
				out = out[:end]
			} else {
				out = nil
			}
		}
		if logger != nil {
			logger.Warn("Git submodule preparation failed", "path", path, "git_output", redact.Text(urlCredentials.ReplaceAllString(string(out), "${1}[REDACTED]@")), "cancelled", ctx.Err() != nil)
		}
		return fmt.Errorf("Git submodule preparation failed [%s]; inspect this machine's preparation diagnostics", FailureCategory(ctx.Err(), string(out)))
	}
	if logger != nil {
		logger.Info("Git submodules ready", "path", path)
	}
	return nil
}

// Return only a safe category; raw Git output remains in owner-only logs.
func FailureCategory(ctxErr error, output string) string {
	if ctxErr == context.DeadlineExceeded {
		return "timeout"
	}
	if ctxErr != nil {
		return "cancelled"
	}
	s := strings.ToLower(output)
	switch {
	case strings.Contains(s, "filename too long"), strings.Contains(s, "file name too long"):
		return "path_length"
	case strings.Contains(s, "authentication failed"), strings.Contains(s, "could not read username"), strings.Contains(s, "permission denied (publickey)"):
		return "authentication"
	case strings.Contains(s, "repository not found"), strings.Contains(s, "not our ref"), strings.Contains(s, "did not contain"), strings.Contains(s, "invalid reference"):
		return "repository_or_revision"
	case strings.Contains(s, "index.lock"), strings.Contains(s, "another git process"):
		return "lock_contention"
	case strings.Contains(s, "could not resolve host"), strings.Contains(s, "connection timed out"), strings.Contains(s, "connection reset"):
		return "network"
	default:
		return "unknown"
	}
}

type boundedOutput struct {
	data      []byte
	truncated bool
}

var urlCredentials = regexp.MustCompile(`(?i)(https?://)[^\s/@]+(?::[^\s/@]*)?@`)

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 16384 - len(b.data)
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	b.data = append(b.data, p...)
	return n, nil
}
