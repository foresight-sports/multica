package gitsubmodule

import (
	"context"
	"testing"
)

func TestFailureCategory(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"Filename too long", "path_length"}, {"fatal: Authentication failed for https://secret@example.invalid", "authentication"}, {"not our ref deadbeef", "repository_or_revision"}, {"index.lock already exists", "lock_contention"}, {"Could not resolve host", "network"}, {"fatal: something else", "unknown"}} {
		if got := FailureCategory(nil, tc.input); got != tc.want {
			t.Errorf("%q: %s", tc.input, got)
		}
	}
	if FailureCategory(context.DeadlineExceeded, "") != "timeout" || FailureCategory(context.Canceled, "") != "cancelled" {
		t.Fatal("lost context failure category")
	}
}
