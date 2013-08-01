// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

// CommonWorker encapsulates the logic for a worker which is based on
// a watcher. We do a bit of setup, and then spin waiting for the
// watcher to trigger or for us to be killed, and then teardown
// cleanly.
type CommonWorker interface {
	// Wait for the worker to finish what it is doing an exit
	Wait() error

	// Kill the running worker, indicating that it should stop what it
	// is doing and exit. Killing a running worker should return a nil
	// error from Wait.
	Kill()

	// Stop will call both Kill and then Wait for the worker to exit.
	Stop() error
}
