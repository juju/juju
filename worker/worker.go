// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.worker")

// Worker describes any type whose validity and/or activity is bounded
// in time. Most frequently, they will represent the duration of some
// task or tasks running on internal goroutines, but it's possible and
// rational to use them to represent any resource that might become
// invalid.
//
// Worker implementations must be goroutine-safe.
type Worker interface {

	// Kill asks the worker to stop and returns immediately.
	Kill()

	// Wait waits for the worker to complete and returns any
	// error encountered when it was running or stopping.
	Wait() error
}

// Stop kills the given Worker and waits for it to complete.
func Stop(worker Worker) error {
	worker.Kill()
	return worker.Wait()
}

// Dead returns a channel that will be closed when the supplied
// Worker has completed.
//
// Don't be too casual about calling Dead -- for example, in a
// standard select loop, `case <-worker.Dead(w):` will create
// one new goroutine per iteration, which is... untidy.
func Dead(worker Worker) <-chan struct{} {
	dead := make(chan struct{})
	go func() {
		defer close(dead)
		worker.Wait()
	}()
	return dead
}
