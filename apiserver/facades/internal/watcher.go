// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
)

type Watcher[T any] interface {
	worker.Worker
	Changes() <-chan T
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
			return t, errors.Errorf("expected an error from %v, got nil", w)
		}
		return t, errors.Trace(err)
	}
	return changes, nil
}
