// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"launchpad.net/tomb"

	apiWatcher "launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/state/watcher"
)

// stringsWorker is the internal implementation of the Worker
// interface, using a StringsWatcher for handling changes.
type stringsWorker struct {
	tomb tomb.Tomb

	// handler is what will be called when events are triggered.
	handler StringsWatchHandler
}

// StringsWatchHandler implements the business logic triggered as part
// of watching a StringsWatcher.
type StringsWatchHandler interface {
	// SetUp starts the handler, this should create the watcher we
	// will be waiting on for more events. SetUp can return a Watcher
	// even if there is an error, and strings Worker will make sure to
	// stop the watcher.
	SetUp() (apiWatcher.StringsWatcher, error)

	// TearDown should cleanup any resources that are left around
	TearDown() error

	// Handle is called when the Watcher has indicated there are
	// changes, do whatever work is necessary to process it
	Handle(changes []string) error
}

// NewStringsWorker starts a new worker running the business logic
// from the handler. The worker loop is started in another goroutine
// as a side effect of calling this.
func NewStringsWorker(handler StringsWatchHandler) Worker {
	sw := &stringsWorker{
		handler: handler,
	}
	go func() {
		defer sw.tomb.Done()
		sw.tomb.Kill(sw.loop())
	}()
	return sw
}

// Kill the loop with no-error
func (sw *stringsWorker) Kill() {
	sw.tomb.Kill(nil)
}

// Wait for the looping to finish
func (sw *stringsWorker) Wait() error {
	return sw.tomb.Wait()
}

func (sw *stringsWorker) loop() error {
	w, err := sw.handler.SetUp()
	if err != nil {
		if w != nil {
			// We don't bother to propagate an error, because we
			// already have an error
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
				return mustErr(w)
			}
			if err := sw.handler.Handle(changes); err != nil {
				return err
			}
		}
	}
}
