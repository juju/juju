// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"time"

	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/internal/errors"
)

// BlockedWorker is a testing worker implementation that blocks indefinitely
// until killed. This is used for simulating a running worker when the test does
// not care about the worker does.
type BlockedWorker struct {
	catacomb.Catacomb
}

// FailingWorker is a dummy test worker that will fail after defined duration.
//
// Used for simulating failing child workers and how this propagates through the
// worker.
type FailingWorker struct {
	catacomb.Catacomb
}

// FailingWorkerError the error returned when [FailingWorker] fails.
const FailingWorkerError = errors.ConstError("fail duration met")

// Kill places the worker into a dying state.
func (b *BlockedWorker) Kill() {
	b.Catacomb.Kill(nil)
}

// Kill places the worker into a dying state.
func (f *FailingWorker) Kill() {
	f.Catacomb.Kill(nil)
}

// NewBlockedWorker constructs and returns a [BlockedWorker] that will block and
// wait until the worker is killed.
func NewBlockedWorker() (*BlockedWorker, error) {
	b := &BlockedWorker{}

	loop := func() error {
		select {
		case <-b.Catacomb.Dying():
			return b.Catacomb.ErrDying()
		}
	}

	return b, catacomb.Invoke(catacomb.Plan{
		Name: "blocked-worker",
		Site: &b.Catacomb,
		Work: loop,
	})
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
func (b *BlockedWorker) Wait() error {
	return b.Catacomb.Wait()
}

// Wait blocks until the worker has finished returning any errors from the
// worker.
func (f *FailingWorker) Wait() error {
	return f.Catacomb.Wait()
}
