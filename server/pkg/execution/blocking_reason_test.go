package execution

import (
	"strings"
	"testing"
)

func TestSelectionPreservesBlockingReason(t *testing.T) {
	p := Policy{Mode: "default", DefaultProfile: "worker", AllowFallback: false}
	c := []Candidate{{Profile: Profile{ID: "worker", RuntimeID: "machine"}, Reason: "Waiting for installation approval"}}
	for _, request := range []Request{{}, {ProfileID: "worker"}} {
		_, _, err := Select(p, request, c, "")
		if err == nil || !strings.Contains(err.Error(), "Waiting for installation approval") {
			t.Fatalf("lost blocker: %v", err)
		}
	}
}
