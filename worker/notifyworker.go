// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"launchpad.net/tomb"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
)

// notifyWorker is the internal implementation of the NotifyWorker interface
type notifyWorker struct {

	// Internal structure
	tomb tomb.Tomb

	// handler is what will handle when events are triggered
	handler WatchHandler

	// closedHandler is set to watcher.MustErr, but that panic()s, so we
	// let the test suite override it.
	closedHandler func(watcher.Errer) error
}

// NotifyWorker encapsulates the logic for a worker which is based on a
// NotifyWatcher. We do a bit of setup, and then spin waiting for the watcher
// to trigger or for us to be killed, and then teardown cleanly.
type NotifyWorker interface {
	// Wait for the NotifyWorker to finish what it is doing an exit
	Wait() error

	// Kill the running worker, indicating that it should stop what it is
	// doing and exit. Killing a running worker should return error = nil
	// from Wait.
	Kill()

	// Stop will call both Kill and then Wait for the worker to exit. 
	Stop() error
}

// WatchHandler implements the business logic that is triggered as part of
// watching a NotifyWatcher.
type WatchHandler interface {

	// SetUp starts the handler, this should create the watcher we will be
	// waiting on for more events. SetUp can return a Watcher even if there
	// is an error, and NotifyWorker will make sure to stop the Watcher.
	SetUp() (params.NotifyWatcher, error)

	// TearDown should cleanup any resources that are left around
	TearDown() error

	// Handle is called when the Watcher has indicated there are changes,
	// do whatever work is necessary to process it
	Handle() error

	// String is used when reporting. It is required because NotifyWatcher
	// is wrapping the WatchHandler, but the WatchHandler is the
	// interesting (identifying) logic.
	String() string
}

// NewNotifyWorker starts a new worker running the business logic from the
// handler. The worker loop is started in another goroutine as a side effect of
// calling this.
func NewNotifyWorker(handler WatchHandler) NotifyWorker {
	nw := &notifyWorker{
		handler:       handler,
		closedHandler: watcher.MustErr,
	}
	go func() {
		defer nw.tomb.Done()
		nw.tomb.Kill(nw.loop())
	}()
	return nw
}

// Kill the loop with no-error
func (nw *notifyWorker) Kill() {
	nw.tomb.Kill(nil)
}

// Stop kils the worker and waits for it to exit
func (nw *notifyWorker) Stop() error {
	nw.tomb.Kill(nil)
	return nw.tomb.Wait()
}

// Wait for the looping to finish
func (nw *notifyWorker) Wait() error {
	return nw.tomb.Wait()
}

// String returns a nice description of this worker, taken from the underlying WatchHandler
func (nw *notifyWorker) String() string {
	return nw.handler.String()
}

// TearDown the handler, but ensure any error is propagated
func handlerTearDown(handler WatchHandler, t *tomb.Tomb) {
	if err := handler.TearDown(); err != nil {
		t.Kill(err)
	}
}

func (nw *notifyWorker) loop() error {
	var w params.NotifyWatcher
	var err error
	defer handlerTearDown(nw.handler, &nw.tomb)
	if w, err = nw.handler.SetUp(); err != nil {
		if w != nil {
			// We don't bother to propogate an error, because we
			// already have an error
			w.Stop()
		}
		return err
	}
	defer watcher.Stop(w, &nw.tomb)
	for {
		select {
		case <-nw.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-w.Changes():
			if !ok {
				return nw.closedHandler(w)
			}
			if err := nw.handler.Handle(); err != nil {
				return err
			}
		}
	}
	panic("unreachable")
}
