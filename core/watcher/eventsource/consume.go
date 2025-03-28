// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"
	"time"

	"github.com/juju/worker/v4"

	"github.com/juju/juju/internal/errors"
)

const (
	// ErrWorkerNotStopping is returned when a worker is not stopping. We can't
	// do anything about it, so we just return the error.
	ErrWorkerNotStopping = errors.ConstError("worker not stopping")

	// ErrWorkerStopped is returned when a worker has stopped. We can't do
	// anything about it, so we just return the error.
	ErrWorkerStopped = errors.ConstError("worker stopped")

	// defaultWorkerStoppingTimeout is the default timeout for a worker to stop.
	defaultWorkerStoppingTimeout = time.Minute
)

// Watcher defines a generic watcher over a set of changes.
type Watcher[T any] interface {
	worker.Worker
	Changes() <-chan T
}

// ConsumeInitialEvent checks whether the first set of returned changes are
// available and returns them, otherwise it kills the worker and waits
// for the error and returns it.
func ConsumeInitialEvent[T any](ctx context.Context, w Watcher[T]) (T, error) {
	select {
	case <-ctx.Done():
		// The context is done waiting for any changes, clean kill the worker
		// and return the context error.
		// Note: we can't use the context deadline here, as it might already
		// be expired, so just use the default timeout.
		_ = killAndWait(w, defaultWorkerStoppingTimeout)
		return *new(T), errors.Capture(ctx.Err())

	case changes, ok := <-w.Changes():
		if ok {
			return changes, nil
		}

		timeout := defaultWorkerStoppingTimeout
		// If the context has a deadline, use that as the timeout.
		if deadline, valid := ctx.Deadline(); valid {
			timeout = time.Until(deadline)
		}

		// The changes channel has already closed, we can't do anything, but
		// kill the worker and wait for the error.
		err := killAndWait(w, timeout)
		if err != nil {
			return changes, errors.Capture(err)
		}
		return changes, errors.Errorf("expected an error from %T, got nil: %w", w, ErrWorkerStopped)
	}
}

func killAndWait[T any](w Watcher[T], timeout time.Duration) error {
	w.Kill()

	done := make(chan struct{})
	defer close(done)

	waitErr := make(chan error)
	go func() {
		select {
		case waitErr <- w.Wait():
		case <-done:
			// Ensure we don't leak a goroutine, because the worker won't stop.
			// In this case, just exit out, we're done.
			return
		}
	}()
	select {
	case err := <-waitErr:
		return errors.Capture(err)
	case <-time.After(timeout):
		return ErrWorkerNotStopping
	}
}
