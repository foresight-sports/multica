package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/daemon/execenv"
)

func TestRequeuedEnvironmentWaitsForCleanup(t *testing.T) {
	p := execenv.RootDirParams{WorkspacesRoot: t.TempDir(), WorkspaceID: "workspace", TaskID: "aaaaaaaa-1111-2222-3333-0123456789ab"}
	first, err := execenv.ClaimEnvRoot(p)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	marker := filepath.Join(first.RootDir(), "active-work.txt")
	if err := os.WriteFile(marker, []byte("still active"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		claim, err := waitForEnvRootClaim(ctx, p)
		if claim != nil {
			claim.Release()
		}
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("second claim did not wait: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "still active" {
		t.Fatalf("live work changed: %q %v", data, err)
	}
	first.Release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWaitingEnvironmentClaimHonorsCancellation(t *testing.T) {
	p := execenv.RootDirParams{WorkspacesRoot: t.TempDir(), WorkspaceID: "workspace", TaskID: "aaaaaaaa-1111-2222-3333-0123456789ab"}
	first, err := execenv.ClaimEnvRoot(p)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := waitForEnvRootClaim(ctx, p); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected cancellation: %v", err)
	}
	if _, err := execenv.ClaimEnvRoot(p); !errors.Is(err, execenv.ErrEnvRootBusy) {
		t.Fatalf("cancelled waiter released original lock: %v", err)
	}
}
