// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workertest

import (
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
)

// NewErrorWorker returns a Worker that runs until Kill()ed; at which point it
// fails with the supplied error. The caller takes responsibility for causing
// it to be Kill()ed, lest the goroutine be leaked, but the worker has no
// outside interactions or safety concerns so there's no particular need to
// Wait() for it.
func NewErrorWorker(err error) worker.Worker {
	w := &errorWorker{err: err}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w
}

type errorWorker struct {
	tomb tomb.Tomb
	err  error
}

// Kill is part of the worker.Worker interface.
func (w *errorWorker) Kill() {
	w.tomb.Kill(w.err)
}

// Wait is part of the worker.Worker interface.
func (w *errorWorker) Wait() error {
	return w.tomb.Wait()
}

// NewDeadWorker returns a Worker that's already dead, and always immediately
// returns the supplied error from Wait().
func NewDeadWorker(err error) worker.Worker {
	return &deadWorker{err: err}
}

type deadWorker struct {
	err error
}

// Kill is part of the worker.Worker interface.
func (w *deadWorker) Kill() {}

// Wait is part of the worker.Worker interface.
func (w *deadWorker) Wait() error {
	return w.err
}

// NewForeverWorker returns a Worker that ignores Kill() calls. You must be sure
// to call ReallyKill() to cause the worker to fail with the supplied error,
// lest any goroutines trying to manage it be leaked or blocked forever.
func NewForeverWorker(err error) *ForeverWorker {
	w := &ForeverWorker{err: err}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w
}

// ForeverWorker is a Worker that breaks its contract. Use with care.
type ForeverWorker struct {
	tomb tomb.Tomb
	err  error
}

// Kill is part of the worker.Worker interface.
func (w *ForeverWorker) Kill() {}

// Wait is part of the worker.Worker interface.
func (w *ForeverWorker) Wait() error {
	return w.tomb.Wait()
}

// ReallyKill does what Kill should.
func (w *ForeverWorker) ReallyKill() {
	w.tomb.Kill(w.err)
}
