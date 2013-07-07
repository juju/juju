// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"launchpad.net/tomb"

	// TODO: Use api/params.NotifyWatcher to avoid redeclaring it here
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
)

// notifyWorker encapsulates the state logic for a worker which is based on a
// NotifyWatcher. We do a bit of setup, and then spin waiting for the watcher
// to trigger or for us to be killed, and then teardown cleanly.
type notifyWorker struct {
	// Internal structure
	tomb tomb.Tomb
	// The watcher the worker is waiting for
	watcher state.NotifyWatcher
	// While loop is starting, setup() will be called
	setup func() error
	// When we get a notification, we will call handler
	handler func() error
	// If we are stopping, call teardown to cleanup
	teardown func()
}

type NotifyWorker interface {
	Wait() error
	Kill()
	// This is just Kill + Wait
	Stop() error
}

func NewNotifyWorker(w state.NotifyWatcher, setup, handler func() error, teardown func()) NotifyWorker {
	nw := &notifyWorker{
		watcher:  w,
		setup:    setup,
		handler:  handler,
		teardown: teardown,
	}
	go func() {
		defer nw.tomb.Done()
		defer watcher.Stop(nw.watcher, &nw.tomb)
		nw.tomb.Kill(nw.loop())
	}()
	return nw
}

// Kill the loop with no-error
func (nw *notifyWorker) Kill() {
	nw.tomb.Kill(nil)
}

// Kill and Wait for this to exit
func (nw *notifyWorker) Stop() error {
	nw.tomb.Kill(nil)
	return nw.tomb.Wait()
}

// Wait for the looping to finish
func (nw *notifyWorker) Wait() error {
	return nw.tomb.Wait()
}

func (nw *notifyWorker) loop() error {
	if err := nw.setup(); err != nil {
		nw.teardown()
		return err
	}
	for {
		select {
		case <-nw.tomb.Dying():
			nw.teardown()
			return tomb.ErrDying
		case <-nw.watcher.Changes():
			if err := nw.handler(); err != nil {
				nw.teardown()
				return err
			}
		}
	}
	panic("unreachable")
}
