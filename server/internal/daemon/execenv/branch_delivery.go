package execenv

import (
	"fmt"
	"strings"
)

// Preserve a named feature-branch delivery on the conversation's managed ref.
// The feature branch is never moved. The managed ref must still be where this
// turn started, and must not be checked out by another worktree.
func (w *LocalWorktree) reconcileDeliveryBranch(tip string) (string, error) {
	if !w.tracksState || tip == "" {
		return tip, nil
	}
	ref := "refs/heads/" + w.Branch
	previous, err := runGitTrimmed(w.GitRoot, "rev-parse", "--verify", ref)
	if err != nil || previous == tip {
		return tip, err
	}
	active, err := runGitTrimmed(w.Path, "symbolic-ref", "--quiet", "HEAD")
	if err != nil || !strings.HasPrefix(active, "refs/heads/") || active == ref {
		return tip, nil
	}
	if previous != w.BaseCommit {
		return tip, fmt.Errorf("managed branch %s changed during this run", w.Branch)
	}
	trees, err := runGitTrimmed(w.GitRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return tip, err
	}
	for _, line := range strings.Split(trees, "\n") {
		if line == "branch "+ref {
			return tip, fmt.Errorf("managed branch %s is checked out in another worktree", w.Branch)
		}
	}
	delivery := tip
	if _, err = runGit(w.GitRoot, "merge-base", "--is-ancestor", w.BaseCommit, tip); err != nil {
		// A fresh, empty baseline is bookkeeping, not user work. Retain it as a
		// merge parent without changing the feature branch's tree or history.
		if !w.createdBranch || w.Continued || w.DirtyBaseCaptured {
			return tip, nil
		}
		parents, e := runGitTrimmed(w.GitRoot, "show", "-s", "--format=%P", w.BaseCommit)
		if e != nil || len(strings.Fields(parents)) != 1 || tip == parents {
			return tip, nil
		}
		baseTree, e := runGitTrimmed(w.GitRoot, "rev-parse", w.BaseCommit+"^{tree}")
		if e != nil {
			return tip, e
		}
		parentTree, e := runGitTrimmed(w.GitRoot, "rev-parse", parents+"^{tree}")
		if e != nil {
			return tip, e
		}
		if baseTree != parentTree {
			return tip, nil
		}
		if _, e = runGit(w.GitRoot, "merge-base", "--is-ancestor", parents, tip); e != nil {
			return tip, nil
		}
		args := append(commitIdentityArgs(w.GitRoot), "commit-tree", tip+"^{tree}", "-p", w.BaseCommit, "-p", tip, "-m", "chore(agent): preserve feature branch delivery")
		delivery, e = runGitTrimmed(w.GitRoot, args...)
		if e != nil {
			return tip, e
		}
	}
	if _, err = runGit(w.GitRoot, "update-ref", ref, delivery, previous); err != nil {
		return tip, err
	}
	return delivery, nil
}
