// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package legacy

import (
	"github.com/juju/worker/v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// stringsWorker is the internal implementation of the Worker
// interface, using a StringsWatcher for handling changes.
type stringsWorker struct {
	tomb    tomb.Tomb
	handler StringsWatchHandler
}

// StringsWatchHandler implements the business logic triggered as part
// of watching a StringsWatcher.
type StringsWatchHandler interface {
	// SetUp will be called once, and should return the watcher that will
	// be used to trigger subsequent Handle()s. SetUp can return a watcher
	// even if there is an error, and the notify worker will make sure
	// to stop the watcher.
	SetUp() (state.StringsWatcher, error)

	// TearDown should cleanup any resources that are left around.
	TearDown() error

	// Handle is called whenever the watcher returned from SetUp sends a value
	// on its Changes() channel.
	Handle(changes []string) error
}

// NewStringsWorker starts a new worker running the business logic
// from the handler. The worker loop is started in another goroutine
// as a side effect of calling this.
func NewStringsWorker(handler StringsWatchHandler) worker.Worker {
	sw := &stringsWorker{
		handler: handler,
	}
	sw.tomb.Go(sw.loop)
	return sw
}

// Kill is part of the worker.Worker interface.
func (sw *stringsWorker) Kill() {
	sw.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (sw *stringsWorker) Wait() error {
	return sw.tomb.Wait()
}

func (sw *stringsWorker) loop() error {
	w, err := sw.handler.SetUp()
	if err != nil {
		if w != nil {
			// We don't bother to propagate an error, because we
			// already have an error.
			w.Stop()
		}
		return err
	}
	defer propagateTearDown(sw.handler, &sw.tomb)
	defer watcher.Stop(w, &sw.tomb)
	for {
		select {
		case <-sw.tomb.Dying():
			return tomb.ErrDying
		case changes, ok := <-w.Changes():
			if !ok {
				return ensureErr(w)
			}
			if err := sw.handler.Handle(changes); err != nil {
				return err
			}
		}
	}
}
