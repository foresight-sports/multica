package execenv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestGitProgressHelper(t *testing.T) {
	mode := os.Getenv("MULTICA_GIT_PROGRESS_TEST")
	if mode == "" {
		return
	}
	if mode != "silent" {
		for i := 0; i < 12; i++ {
			fmt.Fprint(os.Stderr, "Updating files\r")
			time.Sleep(50 * time.Millisecond)
		}
	}
	if mode != "healthy" {
		time.Sleep(10 * time.Second)
	}
	os.Exit(0)
}

func TestGitCheckoutProgressTimeout(t *testing.T) {
	for _, mode := range []string{"healthy", "stalls", "silent"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGitProgressHelper$")
			cmd.Env = append(os.Environ(), "MULTICA_GIT_PROGRESS_TEST="+mode)
			cmd.WaitDelay = time.Second
			start := time.Now()
			out, err := runGitWithProgressTimeout(cmd, cancel, 300*time.Millisecond)
			if mode == "healthy" {
				if err != nil {
					t.Fatalf("healthy checkout stopped: %v", err)
				}
				if time.Since(start) < 600*time.Millisecond {
					t.Fatal("did not exceed original inactivity deadline")
				}
			} else if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("stalled checkout: %v", err)
			}
			if mode != "silent" && !strings.Contains(out, "Updating files") {
				t.Fatal("lost progress diagnostics")
			}
		})
	}
}

func TestPreparationProgressRenewsOuterTimeout(t *testing.T) {
	t.Setenv("MULTICA_GIT_PROGRESS_TEST", "stalls")
	cause := errors.New("preparation stale")
	ctx, cancel := WithPreparationInactivityTimeout(context.Background(), 300*time.Millisecond, cause)
	defer cancel()
	start := time.Now()
	_, err := runPreparationProcess(ctx, []string{os.Args[0], "-test.run=^TestGitProgressHelper$"}, preparationRequest{}, nil)
	if !errors.Is(err, cause) {
		t.Fatalf("stale helper error: %v", err)
	}
	if time.Since(start) < 800*time.Millisecond {
		t.Fatal("outer deadline ignored helper progress")
	}
}

func TestPreparationInactivityHonorsCancellation(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	ctx, cancel := WithPreparationInactivityTimeout(parent, time.Hour, errors.New("stale"))
	defer cancel()
	stop()
	ReportPreparationProgress(ctx)
	if !errors.Is(context.Cause(ctx), context.Canceled) {
		t.Fatalf("cancellation: %v", context.Cause(ctx))
	}
}
