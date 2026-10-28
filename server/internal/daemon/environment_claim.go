package daemon

import (
	"context"
	"errors"
	"time"

	"github.com/multica-ai/multica/server/internal/daemon/execenv"
)

// A requeued claim can arrive before its predecessor has finished unwinding.
// Wait for the OS lock rather than failing the new claim or resetting live work.
// The caller keeps the preparation lease alive and observes cancellation.
func waitForEnvRootClaim(ctx context.Context, params execenv.RootDirParams) (*execenv.EnvRootClaim, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		claim, err := execenv.ClaimEnvRoot(params)
		if !errors.Is(err, execenv.ErrEnvRootBusy) {
			return claim, err
		}
		if err := sleepWithContext(ctx, 100*time.Millisecond); err != nil {
			return nil, err
		}
	}
}
