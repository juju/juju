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

// MustErr returns the error with which w died.
// Calling it will panic if w is still running or was stopped cleanly.
func MustErr(w Errer) error {
	err := errors.Trace(w.Err())
	if err == nil {
		panic("watcher was stopped cleanly")
	} else if w.Err() == tomb.ErrStillAlive {
		panic("watcher is still running")
	}
	return err
}
