// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fortress

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker"
)

// StartFunc starts a worker.Worker.
type StartFunc func() (worker.Worker, error)

// Occupy launches a Visit to fortress that creates a worker and holds the
// visit open until the worker completes. Like most funcs that return any
// Worker, the caller takes responsibility for its lifetime; be aware that
// the responsibility is especially heavy here, because failure to clean up
// the worker will block cleanup of the fortress.
//
// This may sound scary, but the alternative is to have multiple components
// concerned with a single worker's lifetime, and that ends up much worse.
func Occupy(fortress Guest, start StartFunc, abort Abort) (worker.Worker, error) {

	// Create two chans to communicate success and failure of worker
	// creation; and a worker-creation func that sends on exactly one
	// of them, and returns only when (1) a value has been sent and (2)
	// no worker is running. Note especially that it always returns nil.
	started := make(chan worker.Worker, 1)
	failed := make(chan error, 1)
	task := func() error {
		worker, err := start()
		if err != nil {
			failed <- err
		} else {
			started <- worker
			worker.Wait() // ignore error: worker is SEP now.
		}
		return nil
	}

	// Start a goroutine to run the task func inside the fortress. If
	// this operation succeeds, we must inspect started and failed to
	// determine what actually happened; but if it fails, we can be
	// sure it didn't run the task (because the task will never fail).
	finished := make(chan error, 1)
	go func() {
		finished <- fortress.Visit(task, abort)
	}()

	// Watch all these channels to figure out what happened and inform
	// the client. A non-nil error from finished indicates that there
	// will be a value waiting on one of the other channels, and is
	// therefore ignored.
	for {
		select {
		case err := <-finished:
			if err != nil {
				return nil, errors.Trace(err)
			}
		case err := <-failed:
			return nil, errors.Trace(err)
		case worker := <-started:
			return worker, nil
		}
	}
}
