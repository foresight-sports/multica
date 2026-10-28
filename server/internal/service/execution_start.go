package service

import (
	"fmt"

	"github.com/multica-ai/multica/server/pkg/execution"
)

func validateSelectedExecution(selected execution.Profile, runtimeID string, candidates []execution.Candidate) error {
	reason := "Selected profile or machine is no longer accessible"
	for _, candidate := range candidates {
		p := candidate.Profile
		if p.ID != selected.ID || p.RuntimeID != runtimeID {
			continue
		}
		if !candidate.Eligible {
			if candidate.Reason != "" {
				reason = candidate.Reason
			}
			continue
		}
		if p.Model != selected.Model || p.ThinkingLevel != selected.ThinkingLevel || p.ServiceTier != selected.ServiceTier {
			reason = "Selected model, reasoning or speed no longer matches the execution profile"
			continue
		}
		return nil
	}
	return fmt.Errorf("%w: %s", errExecutionMachineUnavailable, reason)
}
