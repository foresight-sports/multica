package daemon

import (
	"context"
	"fmt"
	"github.com/multica-ai/multica/server/pkg/redact"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Preparation is independent of task dispatch so unready machines never claim work.
func (d *Daemon) workspaceRepositoryLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	var mu sync.Mutex
	active := map[string]bool{}
	type failedPreparation struct {
		fingerprint string
		message     string
		at          time.Time
	}
	failures := map[string]failedPreparation{}
	for {
		for _, rid := range d.allRuntimeIDs() {
			var plans []workspacerepo.Plan
			if d.client.getJSON(ctx, "/api/daemon/runtimes/"+rid+"/workspace-repositories", &plans) != nil {
				continue
			}
			for _, plan := range plans {
				if plan.Root == "" || plan.Folder == "" {
					continue
				}
				key := plan.WorkspaceID
				mu.Lock()
				busy := active[key]
				if !busy {
					active[key] = true
				}
				mu.Unlock()
				if busy {
					continue
				}
				go func(rid string, p workspacerepo.Plan) {
					defer func() { mu.Lock(); delete(active, p.WorkspaceID); mu.Unlock() }()
					// Share the updater's claim barrier: binary replacement must wait for cloning.
					if !d.tryEnterClaim() {
						return
					}
					// Keep background checks in the drainable claim barrier, not the
					// agent-task count. Updates can pause new checks and await this one.
					defer d.exitClaim()
					jobCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
					defer cancel()
					report := func(state, message string) {
						c, done := context.WithTimeout(ctx, 10*time.Second)
						defer done()
						_ = d.client.postJSON(c, "/api/daemon/runtimes/"+rid+"/workspace-repositories", workspacerepo.Readiness{WorkspaceID: p.WorkspaceID, Fingerprint: p.Fingerprint, State: state, Message: message}, nil)
					}
					mu.Lock()
					failed, hadFailure := failures[p.WorkspaceID]
					mu.Unlock()
					if hadFailure && failed.fingerprint == p.Fingerprint && time.Since(failed.at) < time.Minute {
						report("unavailable", failed.message)
						return
					}
					logger := d.logger.With("workspace_id", p.WorkspaceID, "repository", p.Repository, "folder", p.Folder)
					if hadFailure && failed.fingerprint == p.Fingerprint {
						logger.Info("Retrying workspace repository preparation")
					}
					err := prepareWorkspaceRepository(jobCtx, p, report, logger)
					if err != nil {
						mu.Lock()
						failures[p.WorkspaceID] = failedPreparation{p.Fingerprint, err.Error(), time.Now()}
						mu.Unlock()
						logger.Warn("Workspace repository preparation failed; retry scheduled", "error", err.Error(), "retry_after", "1m", "check_interval", "15s")
						report("unavailable", err.Error())
					} else {
						if hadFailure {
							logger.Info("Workspace repository preparation recovered")
						}
						mu.Lock()
						delete(failures, p.WorkspaceID)
						mu.Unlock()
						report("ready", "Ready for this workspace")
					}
				}(rid, plan)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func prepareWorkspaceRepository(ctx context.Context, p workspacerepo.Plan, report func(string, string), logger *slog.Logger) error {
	workspace := p.Configuration
	workspace.Root = ""
	if err := workspace.Validate("workspace"); err != nil || workspace.Folder == "" {
		return fmt.Errorf("Invalid workspace repository configuration")
	}
	if !filepath.IsAbs(p.Root) {
		return fmt.Errorf("Repositories folder must be an absolute path on this computer")
	}
	p.Folder = workspace.Folder
	p.Repository = workspace.Repository
	p.Mode = workspace.Mode
	report("preparing", "Checking the workspace folder")
	root, err := filepath.EvalSymlinks(p.Root)
	if err != nil {
		return fmt.Errorf("Repositories folder is missing or inaccessible; configure an existing folder on this computer")
	}
	target := filepath.Join(root, p.Folder)
	ref := localDirectoryRef{RepositoriesRoot: root, LocalPath: target, ExpectedRepository: p.Repository, ExecutionMode: p.Mode}
	if _, err = os.Lstat(target); err == nil {
		return validateWorkspaceRepository(ref)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Workspace folder is inaccessible")
	}
	if p.Repository == "" {
		return fmt.Errorf("Local folder is missing on this computer; folder-only workspaces are not cloned")
	}
	if _, err = exec.LookPath("git"); err != nil {
		return fmt.Errorf("Install Git on this computer to clone the workspace")
	}
	stage, err := os.MkdirTemp(root, ".multica-clone-")
	if err != nil {
		return fmt.Errorf("Cannot create a clone folder inside the repositories folder")
	}
	defer os.RemoveAll(stage) // Only our freshly allocated temporary clone, never an existing checkout.
	logger.Info("Git clone attempt started", "destination", target)
	report("cloning", "Cloning the GitHub repository using this computer's Git credentials")
	cloneCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	cmd := exec.CommandContext(cloneCtx, "git", "-c", "core.longpaths=true", "-c", "credential.interactive=false", "clone", "--recurse-submodules", "--", "https://github.com/"+p.Repository+".git", stage)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	var diagnostics cloneDiagnosticBuffer
	cmd.Stderr = &diagnostics
	go func() { done <- cmd.Run() }()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err = <-done:
			if err != nil {
				logger.Warn("Git clone failed", "error", err.Error(), "git_output", diagnostics.safeText(), "cancelled", cloneCtx.Err() != nil)
				return fmt.Errorf("Git clone failed; check this computer's GitHub credentials, repository access and network, then retry")
			}
			logger.Info("Git clone completed; validating checkout")
			ref.LocalPath = stage
			if err = validateWorkspaceRepository(ref); err != nil {
				return err
			}
			// Never replace an existing folder, even if another process created it during cloning.
			if _, err = os.Lstat(target); err == nil {
				return fmt.Errorf("Workspace folder appeared while cloning; it will be checked on the next refresh")
			}
			if !os.IsNotExist(err) {
				return fmt.Errorf("Cannot inspect the destination folder")
			}
			if err = os.Rename(stage, target); err != nil {
				return fmt.Errorf("Cannot move the cloned repository into the workspace folder")
			}
			ref.LocalPath = target
			if err := validateWorkspaceRepository(ref); err != nil {
				return err
			}
			logger.Info("Workspace repository clone ready", "destination", target)
			return nil
		case <-ticker.C:
			logger.Info("Git clone still running")
			report("cloning", "Cloning the GitHub repository; waiting for Git to finish")
		}
	}
}

// Drain stderr without retaining unbounded Git/credential-helper output.
// Read only after cmd.Run completes so no lock is needed.
type cloneDiagnosticBuffer struct {
	data      []byte
	truncated bool
}

func (b *cloneDiagnosticBuffer) Write(p []byte) (int, error) {
	const limit = 16 * 1024
	n := len(p)
	if n >= limit {
		b.data = append(b.data[:0], p[n-limit:]...)
		b.truncated = true
	} else {
		if len(b.data)+n > limit {
			b.data = append(b.data[:0], b.data[len(b.data)+n-limit:]...)
			b.truncated = true
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

var cloneURLCredentials = regexp.MustCompile(`(?i)(https?://)[^\s/@]+(?::[^\s/@]*)?@`)

func (b *cloneDiagnosticBuffer) safeText() string {
	text := strings.ToValidUTF8(string(b.data), "")
	if b.truncated {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		} else {
			text = "[Git diagnostic line exceeded capture limit]"
		}
	}
	text = cloneURLCredentials.ReplaceAllString(text, "${1}[REDACTED]@")
	return strings.TrimSpace(redact.Text(text))
}
