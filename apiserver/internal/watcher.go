// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"

	apiservererrors "github.com/juju/juju/apiserver/errors"
)

const (
	// ErrWorkerNotStopping is returned when a worker is not stopping. We can't
	// do anything about it, so we just return the error.
	ErrWorkerNotStopping = errors.ConstError("worker not stopping")

	// defaultWorkerStoppingTimeout is the default timeout for a worker to stop.
	defaultWorkerStoppingTimeout = time.Minute
)

// Watcher defines a generic watcher over a set of changes.
type Watcher[T any] interface {
	worker.Worker
	Changes() <-chan T
}

// WatcherRegistry defines a generic registry for watchers.
type WatcherRegistry interface {
	// Register registers a watcher and returns a string that can be used
	// to unregister it.
	Register(w worker.Worker) (string, error)
}

// FirstResult checks whether the first set of returned changes are
// available and returns them, otherwise it kills the worker and waits
// for the error and returns it.
func FirstResult[T any](ctx context.Context, w Watcher[T]) (T, error) {
	select {
	case <-ctx.Done():
		// The context is done waiting for any changes, clean kill the worker
		// and return the context error.
		_ = cleanKill(w, defaultWorkerStoppingTimeout)
		return *new(T), errors.Trace(ctx.Err())

	case changes, ok := <-w.Changes():
		if ok {
			return changes, nil
		}

		// The changes channel has already closed, we can't do anything, but
		// kill the worker and wait for the error.
		err := cleanKill(w, defaultWorkerStoppingTimeout)
		if err != nil {
			return changes, errors.Trace(err)
		}
		return changes, errors.Annotatef(apiservererrors.ErrStoppedWatcher, "expected an error from %T, got nil", w)
	}
}

// EnsureRegisterWatcher registers a watcher and returns the first set
// of changes.
func EnsureRegisterWatcher[T any](ctx context.Context, reg WatcherRegistry, w Watcher[T]) (string, T, error) {
	changes, err := FirstResult(ctx, w)
	if err != nil {
		return "", changes, errors.Trace(err)
	}
	id, err := reg.Register(w)
	if err != nil {
		return "", changes, errors.Trace(err)
	}
	return id, changes, nil
}

func cleanKill[T any](w Watcher[T], timeout time.Duration) error {
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
		return errors.Trace(err)
	case <-time.After(timeout):
		return ErrWorkerNotStopping
	}
}
