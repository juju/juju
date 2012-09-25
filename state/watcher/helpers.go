package watcher

import (
	"fmt"
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
		t.Kill(err)
	}
}

// MustErr returns the error with which w died.
// Calling it will panic if w is still running or was stopped cleanly.
func MustErr(w Errer) error {
	err := w.Err()
	if err == nil {
		panic(fmt.Errorf("watcher %#v was stopped cleanly", w))
	} else if err == tomb.ErrStillAlive {
		panic(fmt.Errorf("watcher %#v is still running", w))
	}
	return err
}
