// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package legacy

import (
	"launchpad.net/tomb"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
)

// ensureErr is defined as a variable to allow the test suite
// to override it.
var ensureErr = watcher.EnsureErr

// notifyWorker is the internal implementation of the Worker
// interface, using a NotifyWatcher for handling changes.
type notifyWorker struct {
	tomb    tomb.Tomb
	handler NotifyWatchHandler
}

// NotifyWatchHandler implements the business logic that is triggered
// as part of watching a NotifyWatcher.
type NotifyWatchHandler interface {
	// SetUp will be called once, and should return the watcher that will
	// be used to trigger subsequent Handle()s. SetUp can return a watcher
	// even if there is an error, and the notify worker will make sure
	// to stop the watcher.
	SetUp() (state.NotifyWatcher, error)

	// TearDown should cleanup any resources that are left around.
	TearDown() error

	// Handle is called whenever the watcher returned from SetUp sends a value
	// on its Changes() channel. The done channel will be closed if and when
	// the worker is being interrupted to finish. Any worker should avoid any
	// bare channel reads or writes, but instead use a select with the done
	// channel.
	Handle(done <-chan struct{}) error
}

// NewNotifyWorker starts a new worker running the business logic from
// the handler. The worker loop is started in another goroutine as a
// side effect of calling this.
func NewNotifyWorker(handler NotifyWatchHandler) worker.Worker {
	nw := &notifyWorker{
		handler: handler,
	}

	go func() {
		defer nw.tomb.Done()
		nw.tomb.Kill(nw.loop())
	}()
	return nw
}

// Kill is part of the worker.Worker interface.
func (nw *notifyWorker) Kill() {
	nw.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
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
