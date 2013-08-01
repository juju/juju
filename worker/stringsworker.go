// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"launchpad.net/tomb"

	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/watcher"
)

// stringsWorker is the internal implementation of the StringsWorker
// interface
type stringsWorker struct {
	tomb tomb.Tomb

	// handler is what will be called when events are triggered.
	handler StringsWatchHandler

	// closedHandler is set to watcher.MustErr, but that panic()s, so
	// we let the test suite override it.
	closedHandler func(watcher.Errer) error
}

// StringsWorker encapsulates the logic for a worker which is based on
// a StringsWatcher. We do a bit of setup, and then spin waiting for
// the watcher to trigger of for use to be killed, and then torn down
// cleanly.
type StringsWorker CommonWorker

// StringsWatchHandler implements the business logic triggered as part
// of watching a StringsWorker.
type StringsWatchHandler interface {
	// SetUp starts the handler, this should create the watcher we
	// will be waiting on for more events. SetUp can return a Watcher
	// even if there is an error, and StringsWorker will make sure to
	// stop the Watcher.
	SetUp() (api.StringsWatcher, error)

	// TearDown should cleanup any resources that are left around
	TearDown() error

	// Handle is called when the Watcher has indicated there are
	// changes, do whatever work is necessary to process it
	Handle(changes []string) error

	// String is used when reporting. It is required because
	// StringsWatcher is wrapping the StringsWatchHandler, but the
	// StringsWatchHandler is the interesting (identifying) logic.
	String() string
}

// NewStringsWorker starts a new worker running the business logic
// from the handler. The worker loop is started in another goroutine
// as a side effect of calling this.
func NewStringsWorker(handler StringsWatchHandler) StringsWorker {
	sw := &stringsWorker{
		handler:       handler,
		closedHandler: watcher.MustErr,
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

// Stop kils the worker and waits for it to exit
func (sw *stringsWorker) Stop() error {
	sw.tomb.Kill(nil)
	return sw.tomb.Wait()
}

// Wait for the looping to finish
func (sw *stringsWorker) Wait() error {
	return sw.tomb.Wait()
}

// String returns a nice description of this worker, taken from the
// underlying StringsWatchHandler
func (sw *stringsWorker) String() string {
	return sw.handler.String()
}

func stringsHandlerTearDown(handler StringsWatchHandler, t *tomb.Tomb) {
	// Tear down the handler, but ensure any error is propagated
	if err := handler.TearDown(); err != nil {
		t.Kill(err)
	}
}

func (sw *stringsWorker) loop() error {
	var w api.StringsWatcher
	var err error
	defer stringsHandlerTearDown(sw.handler, &sw.tomb)
	if w, err = sw.handler.SetUp(); err != nil {
		if w != nil {
			// We don't bother to propogate an error, because we
			// already have an error
			w.Stop()
		}
		return err
	}
	defer watcher.Stop(w, &sw.tomb)
	for {
		select {
		case <-sw.tomb.Dying():
			return tomb.ErrDying
		case changes, ok := <-w.Changes():
			if !ok {
				return sw.closedHandler(w)
			}
			if err := sw.handler.Handle(changes); err != nil {
				return err
			}
		}
	}
}
