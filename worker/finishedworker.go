// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

// finishedWorker implements the worker returned by NewFinishedWorker.
type finishedWorker struct{}

// NewFinishedWorker returns a worker that is doing nothing and immediatelly
// finishes. It is used in those situations where the constructor of a
// concrete worker detects that it is not needed, e.g. due to not supported
// features of the current environment. Then instead of creating an
// instance of that worker the finished worker is returned and ends
// itself in a clean way.
func NewFinishedWorker() Worker {
	return &finishedWorker{}
}

// Kill implements Worker.Kill().
func (w *finishedWorker) Kill() {}

// Wait implements Worker.Wait().
func (w *finishedWorker) Wait() error {
	return nil
}

// IsFinishedWorker returns true if the passed worker is a finished worker.
func IsFinishedWorker(w Worker) bool {
	_, ok := w.(*finishedWorker)
	return ok
}
