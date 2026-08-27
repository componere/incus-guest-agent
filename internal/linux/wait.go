package linux

import (
	"context"
	"time"
)

// Waiter blocks until a fixed duration elapses or the context is canceled.
type Waiter struct {
	// Duration is the interval to wait when [Waiter.Wait] is called.
	Duration time.Duration
}

// NewWaiter constructs a [Waiter] that sleeps for d.
func NewWaiter(d time.Duration) *Waiter {
	return &Waiter{Duration: d}
}

// Wait returns nil after [Waiter.Duration] or a non-nil error when ctx is
// canceled first.
func (w *Waiter) Wait(ctx context.Context) error {
	timer := time.NewTimer(w.Duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
