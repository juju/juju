// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"time"

	"github.com/juju/juju/internal/errors"
	"github.com/juju/worker/v4/catacomb"
)

// FailingWorker is a dummy test worker that will fail after defined duration.
//
// Used for simulating failing child workers and how this propogates through the
// worker.
type FailingWorker struct {
	Catacomb catacomb.Catacomb
}

// FailingWorkerError the error returned when [FailingWorker] fails.
const FailingWorkerError = errors.ConstError("fail duration met")

// Kill places the worker into a dying state.
func (f *FailingWorker) Kill() {
	f.Catacomb.Kill(nil)
}

// NewFailingWorker constructs and returns a [FailingWorker] that will fail
// after the supplied duration is reached.
func NewFailingWorker(d time.Duration) (*FailingWorker, error) {
	f := &FailingWorker{}

	loop := func() error {
		select {
		case <-f.Catacomb.Dying():
			return f.Catacomb.ErrDying()
		case <-time.After(d):
			return FailingWorkerError
		}
	}

	return f, catacomb.Invoke(catacomb.Plan{
		Name: "failing-worker",
		Site: &f.Catacomb,
		Work: loop,
	})
}

// Wait blocks until the worker has finished returning any errors from the
// worker.
func (f *FailingWorker) Wait() error {
	return f.Catacomb.Wait()
}
