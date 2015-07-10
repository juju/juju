// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"launchpad.net/tomb"

	apiWatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/state/watcher"
)

// ensureErr is defined as a variable to allow the test suite
// to override it.
var ensureErr = watcher.EnsureErr

// notifyWorker is the internal implementation of the Worker
// interface, using a NotifyWatcher for handling changes.
type notifyWorker struct {
	tomb tomb.Tomb

	// handler is what will handle when events are triggered
	handler NotifyWatchHandler
}

// NotifyWatchHandler implements the business logic that is triggered
// as part of watching a NotifyWatcher.
type NotifyWatchHandler interface {
	// SetUp starts the handler, this should create the watcher we
	// will be waiting on for more events. SetUp can return a Watcher
	// even if there is an error, and the notify Worker will make sure
	// to stop the watcher.
	SetUp() (apiWatcher.NotifyWatcher, error)

	// TearDown should cleanup any resources that are left around
	TearDown() error

	// Handle is called when the Watcher has indicated there are changes, do
	// whatever work is necessary to process it. The done channel is closed if
	// the worker is being interrupted to finish. Any worker should avoid any
	// bare channel reads or writes, but instead use a select with the done
	// channel.
	Handle(done <-chan struct{}) error
}

// NewNotifyWorker starts a new worker running the business logic from
// the handler. The worker loop is started in another goroutine as a
// side effect of calling this.
func NewNotifyWorker(handler NotifyWatchHandler) Worker {
	nw := &notifyWorker{
		handler: handler,
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

// Wait for the looping to finish
func (nw *notifyWorker) Wait() error {
	return nw.tomb.Wait()
}

type tearDowner interface {
	TearDown() error
}

// propagateTearDown tears down the handler, but ensures any error is
// propagated through the tomb's Kill method.
func propagateTearDown(handler tearDowner, t *tomb.Tomb) {
	if err := handler.TearDown(); err != nil {
		t.Kill(err)
	}
}

func (nw *notifyWorker) loop() error {
	w, err := nw.handler.SetUp()
	if err != nil {
		if w != nil {
			// We don't bother to propagate an error, because we
			// already have an error
			w.Stop()
		}
		return err
	}
	defer propagateTearDown(nw.handler, &nw.tomb)
	defer watcher.Stop(w, &nw.tomb)
	for {
		select {
		case <-nw.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-w.Changes():
			if !ok {
				return ensureErr(w)
			}
			if err := nw.handler.Handle(nw.tomb.Dying()); err != nil {
				return err
			}
		}
	}
}
