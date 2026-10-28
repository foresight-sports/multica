package execenv

import (
	"context"
	"sync"
	"time"
)

type preparationProgressKey struct{}

// WithPreparationInactivityTimeout bounds silence, not elapsed task time.
// Helper diagnostics renew it; routine lease heartbeats deliberately do not.
func WithPreparationInactivityTimeout(parent context.Context, timeout time.Duration, cause error) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	var mu sync.Mutex
	last := time.Now()
	stopped := false
	var timer *time.Timer
	mu.Lock()
	timer = time.AfterFunc(timeout, func() {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			return
		}
		if remaining := timeout - time.Since(last); remaining > 0 {
			timer.Reset(remaining)
			return
		}
		stopped = true
		cancel(cause)
	})
	mu.Unlock()
	progress := func() {
		mu.Lock()
		defer mu.Unlock()
		if !stopped && ctx.Err() == nil {
			last = time.Now()
			timer.Reset(timeout)
		}
	}
	return context.WithValue(ctx, preparationProgressKey{}, progress), func() {
		mu.Lock()
		stopped = true
		timer.Stop()
		mu.Unlock()
		cancel(context.Canceled)
	}
}

func ReportPreparationProgress(ctx context.Context) {
	if progress, ok := ctx.Value(preparationProgressKey{}).(func()); ok {
		progress()
	}
}
