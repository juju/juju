// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/errors"
	"launchpad.net/tomb"
)

// Stopper is implemented by all watchers.
type Stopper interface {
	Stop() error
}

// Errer is implemented by all watchers.
type Errer interface {
	Err() error
}

// Stop stops the watcher. If an error is returned by the
// watcher, t is killed with the error.
func Stop(w Stopper, t *tomb.Tomb) {
	if err := w.Stop(); err != nil {
		if err != tomb.ErrStillAlive && err != tomb.ErrDying {
			// tomb.Kill() checks for the two errors above
			// by value, so we shouldn't wrap them, but we
			// wrap any other error.
			err = errors.Trace(err)
		}
		t.Kill(err)
	}
}

// EnsureErr returns the error with which w died. Calling it will also
// return an error if w is still running or was stopped cleanly.
func EnsureErr(w Errer) error {
	err := w.Err()
	if err == nil {
		return errors.Errorf("expected an error from %#v, got nil", w)
	} else if err == tomb.ErrStillAlive {
		return errors.Annotatef(err, "expected %#v to be stopped", w)
	}
	return errors.Trace(err)
}
