package daemon

import (
	"context"
	"fmt"
	"github.com/multica-ai/multica/server/internal/daemon/execenv"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
)

func (d *Daemon) prepareSharedBaseline(ctx context.Context, task Task, local *localDirectoryAssignment) (*workspacerepo.Baseline, error) {
	if local == nil || local.Ref.BaseBranch == "" {
		return nil, nil
	}
	if !local.UsesWorktree() {
		return nil, fmt.Errorf("shared repository baselines require isolated worktrees")
	}
	sha, err := execenv.FetchSharedBase(ctx, local.AbsPath, local.Ref.BaseBranch, task.ID)
	if err != nil {
		return nil, err
	}
	candidate := workspacerepo.Baseline{Repository: local.Ref.ExpectedRepository, Branch: local.Ref.BaseBranch, Commit: sha, Revision: local.Ref.RepositoryRevision}
	var pinned workspacerepo.Baseline
	if err = d.client.postJSON(ctx, "/api/daemon/tasks/"+task.ID+"/repository-baseline", candidate, &pinned); err != nil {
		return nil, err
	}
	if pinned.Repository != candidate.Repository || pinned.Branch != candidate.Branch || pinned.Revision != candidate.Revision {
		return nil, fmt.Errorf("repository baseline response does not match configuration")
	}
	if err = execenv.EnsureSharedCommit(ctx, local.AbsPath, pinned.Commit); err != nil {
		return nil, err
	}
	return &pinned, nil
}
