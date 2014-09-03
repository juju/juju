// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"time"

	"launchpad.net/tomb"

	"github.com/juju/juju/state"
)

// machineErrorRetry is a notify watcher that fires when it is
// appropriate to retry provisioning machines with transient errors.
type machineErrorRetry struct {
	tomb tomb.Tomb
	out  chan struct{}
}

func newWatchMachineErrorRetry() state.NotifyWatcher {
	w := &machineErrorRetry{
		out: make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Stop stops the watcher, and returns any error encountered while running
// or shutting down.
func (w *machineErrorRetry) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill kills the watcher without waiting for it to shut down.
func (w *machineErrorRetry) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the watcher to die and returns any
// error encountered when it was running.
func (w *machineErrorRetry) Wait() error {
	return w.tomb.Wait()
}

// Err returns any error encountered while running or shutting down, or
// tomb.ErrStillAlive if the watcher is still running.
func (w *machineErrorRetry) Err() error {
	return w.tomb.Err()
}

// Changes returns the event channel for the machineErrorRetry watcher.
func (w *machineErrorRetry) Changes() <-chan struct{} {
	return w.out
}

// ErrorRetryWaitDelay is the poll time currently used to trigger the watcher.
var ErrorRetryWaitDelay = 1 * time.Minute

// The initial implementation of this watcher simply acts as a poller,
// triggering every ErrorRetryWaitDelay minutes.
func (w *machineErrorRetry) loop() error {
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(ErrorRetryWaitDelay):
			out = w.out
		case out <- struct{}{}:
			out = nil
		}
	}
}
