// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fortress

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
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
// "responsible for" a single worker's lifetime -- and Fortress itself would
// have to grow new concerns, of understanding and managing worker.Workers --
// and that scenario ends up much worse.
func Occupy(ctx context.Context, fortress Guest, start StartFunc) (worker.Worker, error) {
	// Create two channels to communicate success and failure of worker
	// creation; and a worker-running func that sends on exactly one
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
			_ = worker.Wait() // ignore error: worker is SEP now.
		}
		return nil
	}

	// Start a goroutine to run the task func inside the fortress. If
	// this operation succeeds, we must inspect started and failed to
	// determine what actually happened; but if it fails, we can be
	// confident that the task (which never fails) did not run, and can
	// therefore return the failure without waiting further.
	finished := make(chan error, 1)
	go func() {
		finished <- fortress.Visit(ctx, task)
	}()

	// Watch all these channels to figure out what happened and inform
	// the client. A nil error from finished indicates that there will
	// be some value waiting on one of the other channels.
	for {
		select {
		case <-ctx.Done():
			return nil, ErrAborted
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
