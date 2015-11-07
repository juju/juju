// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker"
)

var _ worker.Runner = (*StubRunner)(nil)

var runnerMethodNames = []string{
	"StartWorker",
	"StopWorker",
}

// StubRunner is a testing stub for worker.Runner.
type StubRunner struct {
	worker.Worker
	// Stub is the underlying testing stub.
	Stub *testing.Stub
	// CallWhenStarted indicates that the newWorker func should be
	// called when StartWorker is called.
	CallWhenStarted bool
}

// NewStubRunner returns a new StubRunner.
func NewStubRunner(stub *testing.Stub) *StubRunner {
	return &StubRunner{
		Worker: NewStubWorker(stub),
		Stub:   stub,
	}
}

func (r *StubRunner) validMethodName(funcName string) bool {
	for _, knownName := range runnerMethodNames {
		if funcName == knownName {
			return true
		}
	}
	return false
}

func (r *StubRunner) checkCallIDs(c *gc.C, methName string, skipMismatch bool, expected []string) {
	var ids []string
	for _, call := range r.Stub.Calls() {
		if !r.validMethodName(call.FuncName) {
			c.Logf("invalid called func name %q (must be one of %#v)", call.FuncName, runnerMethodNames)
			c.FailNow()
		}
		if methName != "" {
			if skipMismatch && call.FuncName != methName {
				continue
			}
			c.Check(call.FuncName, gc.Equals, methName)
		}
		ids = append(ids, call.Args[0].(string))
	}
	sort.Strings(ids)
	sort.Strings(expected)
	c.Check(ids, jc.DeepEquals, expected)
}

// CheckCallIDs verifies that the worker IDs in all calls match the
// provided ones. If a method name is provided as well then all calls must
// have that method name.
func (r *StubRunner) CheckCallIDs(c *gc.C, methName string, expected ...string) {
	r.checkCallIDs(c, methName, false, expected)
}

// StartWorker implements worker.Runner.
func (r *StubRunner) StartWorker(id string, newWorker func() (worker.Worker, error)) error {
	r.Stub.AddCall("StartWorker", id, newWorker)
	if err := r.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if r.CallWhenStarted {
		// TODO(ericsnow) Save the workers?
		if _, err := newWorker(); err != nil {
			return errors.Trace(err)
		}
	}
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
