package execenv

import (
	"context"
	"fmt"
	"github.com/multica-ai/multica/server/pkg/workflow"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"os/exec"
	"strings"
)

// Fetch into a task-specific ref: concurrent fetches must not race through
// FETCH_HEAD, and neither the user's branch nor its working files are changed.
func FetchSharedBase(ctx context.Context, repo, branch, taskID string) (string, error) {
	if !workflow.ValidBranch(branch) {
		return "", fmt.Errorf("invalid shared base branch")
	}
	ref := "refs/multica/baselines/" + taskKey(taskID)
	if err := fetchSharedRef(ctx, repo, "+refs/heads/"+branch+":"+ref); err != nil {
		return "", err
	}
	return runGitTrimmed(repo, "rev-parse", "--verify", ref+"^{commit}")
}

func EnsureSharedCommit(ctx context.Context, repo, commit string) error {
	if !workspacerepo.ValidCommit(commit) {
		return fmt.Errorf("invalid shared baseline commit")
	}
	if kind, err := runGitTrimmed(repo, "cat-file", "-t", commit); err == nil && kind == "commit" {
		return nil
	}
	if err := fetchSharedRef(ctx, repo, commit); err != nil {
		return err
	}
	kind, err := runGitTrimmed(repo, "cat-file", "-t", commit)
	if err != nil || kind != "commit" {
		return fmt.Errorf("shared baseline is not a commit")
	}
	return nil
}

func fetchSharedRef(parent context.Context, repo, ref string) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-c", "core.longpaths=true", "-c", "credential.interactive=false", "-C", repo, "fetch", "--no-tags", "--progress", "origin", ref)
	cmd.WaitDelay = preparationWaitDelay
	_, err := runGitWithProgressTimeout(cmd, cancel, gitCheckoutTimeout, func() { ReportPreparationProgress(parent) })
	if err != nil {
		return fmt.Errorf("fetch shared repository baseline: %w", err)
	}
	return nil
}

func RepositoryProvenance(w *LocalWorktree) string {
	return fmt.Sprintf("\n\n## Verified repository source\nShared source commit: %s\nTask snapshot commit: %s\nTask branch: %s\nThe task snapshot is generated locally by Multica and may not exist on another machine or on GitHub. Compare the shared source commit to compare versions. Do not treat a snapshot missing on another machine as fabricated evidence.\n", strings.TrimSpace(w.SourceCommit), w.BaseCommit, w.Branch)
}
