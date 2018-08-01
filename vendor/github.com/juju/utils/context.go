package utils

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/juju/utils/clock"
)

// timerCtx is an implementation of context.Context that
// is done when a given deadline has passed
// (as measured by the Clock in the clock field)
type timerCtx struct {
	clock    clock.Clock
	timer    clock.Timer
	deadline time.Time
	parent   context.Context
	done     chan struct{}

	// mu guards err.
	mu sync.Mutex

	// err holds context.Canceled or context.DeadlineExceeded
	// after the context has been canceled.
	// If this is non-nil, then done will have been closed.
	err error
}

func (ctx *timerCtx) Deadline() (time.Time, bool) {
	return ctx.deadline, true
}

func (ctx *timerCtx) Err() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.err
}

func (ctx *timerCtx) Value(key interface{}) interface{} {
	return ctx.parent.Value(key)
}

func (ctx *timerCtx) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *timerCtx) cancel(err error) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if err == nil {
		panic("cancel with nil error!")
	}
	if ctx.err != nil {
		// Already canceled - no need to do anything.
		return
	}
	ctx.err = err
	if ctx.timer != nil {
		ctx.timer.Stop()
	}
	close(ctx.done)
}

func (ctx *timerCtx) String() string {
	return fmt.Sprintf("%v.WithDeadline(%s [%s])", ctx.parent, ctx.deadline, ctx.deadline.Sub(ctx.clock.Now()))
}

// ContextWithTimeout is like context.WithTimeout
// except that it works with a clock.Clock rather than
// wall-clock time.
func ContextWithTimeout(parent context.Context, clk clock.Clock, timeout time.Duration) (context.Context, context.CancelFunc) {
	return ContextWithDeadline(parent, clk, clk.Now().Add(timeout))
}

// ContextWithDeadline is like context.WithDeadline
// except that it works with a clock.Clock rather than
// wall-clock time.
func ContextWithDeadline(parent context.Context, clk clock.Clock, deadline time.Time) (context.Context, context.CancelFunc) {
	d := deadline.Sub(clk.Now())
	ctx := &timerCtx{
		clock:    clk,
		parent:   parent,
		deadline: deadline,
		done:     make(chan struct{}),
	}
	if d <= 0 {
		// deadline has already passed
		ctx.cancel(context.DeadlineExceeded)
		return ctx, func() {}
	}
	ctx.timer = clk.NewTimer(d)
	go func() {
		select {
		case <-ctx.timer.Chan():
			ctx.cancel(context.DeadlineExceeded)
		case <-parent.Done():
			ctx.cancel(parent.Err())
		case <-ctx.done:
		}
	}()
	return ctx, func() {
		ctx.cancel(context.Canceled)
	}
}
