// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/worker"
)

var _ worker.Runner = (*StubRunner)(nil)

// StubRunner is a testing stub for worker.Runner.
type StubRunner struct {
	worker.Worker
	// Stub is the underlying testing stub.
	Stub *testing.Stub
}

// NewStubRunner returns a new StubRunner.
func NewStubRunner(stub *testing.Stub) *StubRunner {
	return &StubRunner{
		Worker: NewStubWorker(stub),
		Stub:   stub,
	}
}

// StartWorker implements worker.Runner.
func (r *StubRunner) StartWorker(id string, newWorker func() (worker.Worker, error)) error {
	r.Stub.AddCall("StartWorker", id, newWorker)
	if err := r.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	// Do nothing.
	return nil
}

// StopWorker implements worker.Runner.
func (r *StubRunner) StopWorker(id string) error {
	r.Stub.AddCall("StopWorker", id)
	if err := r.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	// Do nothing.
	return nil
}
