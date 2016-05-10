// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import "github.com/juju/loggo"

var logger = loggo.GetLogger("juju.worker")

// Worker describes any type using internal resources.
type Worker interface {

	// Kill asks the worker to stop without necessarily
	// waiting for it to do so.
	Kill()

	// Wait waits for the worker to exit and returns any
	// error encountered when it was running.
	Wait() error
}

// Stop kills the given Worker and waits for it to exit.
func Stop(worker Worker) error {
	worker.Kill()
	return worker.Wait()
}

// Dead returns a channel that will be closed when the supplied
// Worker has stopped.
//
// Don't be too casual about creating them -- for example, in a
// standard select loop, `case <-worker.Dead(w):` will create
// one goroutine per iteration, which is... untidy.
func Dead(worker Worker) <-chan struct{} {
	dead := make(chan struct{})
	go func() {
		defer close(dead)
		worker.Wait()
	}()
	return dead
}
