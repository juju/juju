// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"
)

// Errer is implemented by all watchers.
type Errer interface {
	Err() error
}

// EnsureErr returns the error with which w died. Calling it will also
// return an error if w is still running or was stopped cleanly.
// Deprecated: This function is deprecated. Use apiserver/internal/EnsureRegisterWatcher
func EnsureErr(w Errer) error {
	err := w.Err()
	if err == nil {
		return errors.Errorf("expected an error from watcher, got nil")
	} else if err == tomb.ErrStillAlive {
		return errors.Annotatef(err, "expected watcher to be stopped")
	}
	return errors.Trace(err)
}
