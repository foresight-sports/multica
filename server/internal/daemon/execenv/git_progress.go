package execenv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// gitProgressBuffer retains the diagnostic tail and treats every output write
// (including carriage-return progress updates) as proof the checkout is alive.
type gitProgressBuffer struct {
	mu               sync.Mutex
	tail             []byte
	lastProgress     time.Time
	timeout          time.Duration
	timer            *time.Timer
	cancel           context.CancelFunc
	stopped, expired bool
	lastReported     time.Time
	progress         func()
}

func (b *gitProgressBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p) > 0 && !b.stopped {
		b.lastProgress = time.Now()
		b.timer.Reset(b.timeout)
		if b.progress != nil {
			b.progress()
		}
		if os.Getenv("MULTICA_PREPARATION_PROGRESS") == "1" && time.Since(b.lastReported) >= time.Second {
			// A fixed marker carries activity across the helper boundary without
			// leaking Git output, paths, or credentials into progress messages.
			fmt.Fprintln(os.Stderr, "Git checkout progress")
			b.lastReported = time.Now()
		}
	}
	const limit = 64 * 1024
	b.tail = append(b.tail, p...)
	if len(b.tail) > limit {
		b.tail = append([]byte(nil), b.tail[len(b.tail)-limit:]...)
	}
	return len(p), nil
}

func (b *gitProgressBuffer) checkStale() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return
	}
	// Reset can race with a callback that has already fired. Recheck the last
	// progress under the same lock before cancelling a healthy process.
	if remaining := b.timeout - time.Since(b.lastProgress); remaining > 0 {
		b.timer.Reset(remaining)
		return
	}
	b.expired = true
	b.stopped = true
	b.cancel()
}

func runGitWithProgressTimeout(cmd *exec.Cmd, cancel context.CancelFunc, timeout time.Duration, progress ...func()) (string, error) {
	b := &gitProgressBuffer{lastProgress: time.Now(), timeout: timeout, cancel: cancel}
	if len(progress) > 0 {
		b.progress = progress[0]
	}
	b.mu.Lock()
	b.timer = time.AfterFunc(timeout, b.checkStale)
	b.mu.Unlock()
	cmd.Stdout, cmd.Stderr = b, b
	err := cmd.Run()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopped = true
	b.timer.Stop()
	if b.expired {
		err = fmt.Errorf("git checkout made no progress for %s: %w", timeout, context.DeadlineExceeded)
	}
	return string(b.tail), err
}
