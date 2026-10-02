package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/pkg/workspacerepo"
)

func validateWorkspaceRepository(ref localDirectoryRef) error {
	root, err := filepath.EvalSymlinks(ref.RepositoriesRoot)
	if err != nil {
		return fmt.Errorf("workspace repository: machine repositories folder is unavailable")
	}
	target, err := filepath.EvalSymlinks(ref.LocalPath)
	if err != nil {
		return fmt.Errorf("workspace repository: workspace folder is missing; waiting for workspace preparation")
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("workspace repository: checkout must be a directory")
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("workspace repository: checkout resolves outside the machine repositories folder")
	}
	if ref.ExpectedRepository == "" && ref.ExecutionMode != "worktree" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	top, err := exec.CommandContext(ctx, "git", "-C", target, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("workspace repository: folder is not a Git checkout")
	}
	actual, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil {
		return fmt.Errorf("workspace repository: cannot resolve Git root")
	}
	relative, err := filepath.Rel(target, actual)
	if err != nil || relative != "." {
		return fmt.Errorf("workspace repository: configured folder must be the Git repository root")
	}
	if ref.ExecutionMode == "worktree" {
		if err := exec.CommandContext(ctx, "git", "-C", target, "rev-parse", "--verify", "HEAD").Run(); err != nil {
			return fmt.Errorf("workspace repository: isolated worktrees require at least one commit")
		}
	}
	if ref.ExpectedRepository == "" {
		return nil
	}
	remote, err := exec.CommandContext(ctx, "git", "-C", target, "remote", "get-url", "origin").Output()
	if err != nil || workspacerepo.GitHubIdentity(string(remote)) != strings.ToLower(ref.ExpectedRepository) {
		return fmt.Errorf("workspace repository: origin does not match configured GitHub repository %s", ref.ExpectedRepository)
	}
	return nil
}
