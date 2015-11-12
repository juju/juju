// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/worker"
)

var _ worker.Worker = (*StubWorker)(nil)

// StubWorker is a testing stub for worker.Worker.
type StubWorker struct {
	// Stub is the underlying testing stub.
	Stub *testing.Stub
}

// NewStubWorker returns a new StubWorker.
func NewStubWorker(stub *testing.Stub) *StubWorker {
	return &StubWorker{
		Stub: stub,
	}
}

// Kill implements worker.Worker.
func (w *StubWorker) Kill() {
	w.Stub.AddCall("Kill")
	w.Stub.NextErr()

	// Do nothing.
}

// Wait implements worker.Worker.
func (w *StubWorker) Wait() error {
	w.Stub.AddCall("Wait")
	if err := w.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	// Do nothing.
	return nil
}
