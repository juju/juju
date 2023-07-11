// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"

	apiservererrors "github.com/juju/juju/apiserver/errors"
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
func FirstResult[T any](w Watcher[T]) (T, error) {
	changes, ok := <-w.Changes()
	if !ok {
		w.Kill()
		var t T
		err := w.Wait()
		if err == nil {
			return t, errors.Annotatef(apiservererrors.ErrStoppedWatcher, "expected an error from %T, got nil", w)
		}
		return t, errors.Trace(err)
	}
	return changes, nil
}

// EnsureRegisterWatcher registers a watcher and returns the first set
// of changes.
func EnsureRegisterWatcher[T any](reg WatcherRegistry, w Watcher[T]) (string, T, error) {
	changes, err := FirstResult(w)
	if err != nil {
		return "", changes, errors.Trace(err)
	}
	id, err := reg.Register(w)
	if err != nil {
		return "", changes, errors.Trace(err)
	}
	return id, changes, nil
}
