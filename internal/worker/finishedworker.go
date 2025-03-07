// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

// FinishedWorker is a worker that stops immediately with no error
// when started by a Runner, which then removes it from the list of
// workers without restarting it. Simply return FinishedWorker{}
// where you need to avoid starting a worker at all.
type FinishedWorker struct{}

// Kill implements Worker.Kill() and does nothing.
func (w FinishedWorker) Kill() {}

// Wait implements Worker.Wait() and immediately returns no error.
func (w FinishedWorker) Wait() error { return nil }
