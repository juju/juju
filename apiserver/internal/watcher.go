// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/watcher/eventsource"
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
	Register(context.Context, worker.Worker) (string, error)
}

// FirstResult checks whether the first set of returned changes are
// available and returns them, otherwise it kills the worker and waits
// for the error and returns it.
func FirstResult[T any](ctx context.Context, w eventsource.Watcher[T]) (T, error) {
	t, err := eventsource.ConsumeInitialEvent(ctx, w)
	if err == nil {
		return t, nil
	}

	// Enable backwards compatibility with the apiserver error.
	if errors.Is(err, eventsource.ErrWorkerStopped) {
		return t, errors.Annotatef(apiservererrors.ErrStoppedWatcher, "expected an error from %T, got nil", w)
	}

	return t, errors.Trace(err)
}

// EnsureRegisterWatcher registers a watcher and returns the first set
// of changes.
func EnsureRegisterWatcher[T any](ctx context.Context, reg WatcherRegistry, w eventsource.Watcher[T]) (string, T, error) {
	changes, err := FirstResult(ctx, w)
	if err != nil {
		return "", changes, errors.Trace(err)
	}
	id, err := reg.Register(ctx, w)
	if err != nil {
		// We failed to register the watcher, so we must kill it, even though
		// we don't own it. We've taken responsibility for it now and we
		// need to ensure that it is stopped if it can't be assigned to
		// the registry.
		// This is safe as calling multiple kills on a worker is idempotent.
		w.Kill()
		return "", changes, errors.Trace(err)
	}
	return id, changes, nil
}
