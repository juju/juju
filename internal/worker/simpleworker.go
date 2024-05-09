// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"context"

	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"
)

// simpleWorker implements the worker returned by NewSimpleWorker.
type simpleWorker struct {
	tomb tomb.Tomb
}

// NewSimpleWorker returns a worker that runs the given function.  The
// stopCh argument will be closed when the worker is killed. The error returned
// by the doWork function will be returned by the worker's Wait function.
func NewSimpleWorker(doWork func(context.Context) error) worker.Worker {
	w := &simpleWorker{}
	w.tomb.Go(func() error {
		ctx, cancel := context.WithCancel(w.tomb.Context(context.Background()))
		defer cancel()

		return doWork(ctx)
	})
	return w
}

// Kill implements Worker.Kill() and will close the channel given to the doWork
// function.
func (w *simpleWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements Worker.Wait(), and will return the error returned by
// the doWork function.
func (w *simpleWorker) Wait() error {
	return w.tomb.Wait()
}
