package service

import (
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/pkg/execution"
)

func TestValidateSelectedExecutionPreservesReason(t *testing.T) {
	profile := execution.Profile{ID: "default", RuntimeID: "machine", Model: "model", ThinkingLevel: "medium"}
	for _, reason := range []string{"Workspace repository is not ready", "Subscription capacity exhausted", "Runtime is offline or stale"} {
		err := validateSelectedExecution(profile, "machine", []execution.Candidate{{Profile: profile, Reason: reason}})
		if !IsExecutionMachineUnavailable(err) || !strings.Contains(err.Error(), reason) {
			t.Fatalf("lost rejection reason %q: %v", reason, err)
		}
	}
	other := profile
	other.RuntimeID = "other-machine"
	if err := validateSelectedExecution(profile, "machine", []execution.Candidate{{Profile: other, Eligible: true}}); !IsExecutionMachineUnavailable(err) {
		t.Fatalf("accepted a different machine: %v", err)
	}
	changed := profile
	changed.Model = "changed-model"
	if err := validateSelectedExecution(profile, "machine", []execution.Candidate{{Profile: changed, Eligible: true}}); !IsExecutionMachineUnavailable(err) {
		t.Fatalf("accepted changed profile: %v", err)
	}
	if err := validateSelectedExecution(profile, "machine", []execution.Candidate{{Profile: profile, Eligible: true}}); err != nil {
		t.Fatal(err)
	}
}
