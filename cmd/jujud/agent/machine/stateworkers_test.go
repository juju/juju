// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type StateWorkersSuite struct {
	coretesting.BaseSuite
	manifold    dependency.Manifold
	startError  error
	startCalled bool
}

var _ = gc.Suite(&StateWorkersSuite{})

func (s *StateWorkersSuite) SetUpTest(c *gc.C) {
	s.startError = nil
	s.startCalled = false
	s.manifold = machine.StateWorkersManifold(machine.StateWorkersConfig{
		StateName:         "state",
		StartStateWorkers: s.startStateWorkers,
	})
}

func (s *StateWorkersSuite) startStateWorkers(st *state.State) (worker.Worker, error) {
	s.startCalled = true
	if s.startError != nil {
		return nil, s.startError
	}
	return new(mockWorker), nil
}

func (s *StateWorkersSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{
		"state",
	})
}

func (s *StateWorkersSuite) TestNoStartStateWorkers(c *gc.C) {
	manifold := machine.StateWorkersManifold(machine.StateWorkersConfig{})
	worker, err := manifold.Start(dt.StubGetResource(nil))
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "StartStateWorkers not specified")
	c.Check(s.startCalled, jc.IsFalse)
}

func (s *StateWorkersSuite) TestStateMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"state": dt.StubResource{Error: dependency.ErrMissing},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
	c.Check(s.startCalled, jc.IsFalse)
}

func (s *StateWorkersSuite) TestStartError(c *gc.C) {
	tracker := new(mockStateTracker)
	getResource := dt.StubGetResource(dt.StubResources{
		"state": dt.StubResource{Output: tracker},
	})
	s.startError = errors.New("boom")

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "worker startup: boom")
	c.Check(s.startCalled, jc.IsTrue)
	tracker.assertDoneCalled(c)
}

func (s *StateWorkersSuite) TestStartSuccess(c *gc.C) {
	tracker := new(mockStateTracker)
	getResource := dt.StubGetResource(dt.StubResources{
		"state": dt.StubResource{Output: tracker},
	})
	w, err := s.manifold.Start(getResource)
	c.Check(w, gc.Not(gc.IsNil))
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.startCalled, jc.IsTrue)
	c.Check(tracker.doneCalled, jc.IsFalse)

	// Ensure Done is called on tracker when worker exits.
	worker.Stop(w)
	tracker.assertDoneCalled(c)
}

// Implements StateTracker.
type mockStateTracker struct {
	doneCalled bool
}

func (t *mockStateTracker) Use() (*state.State, error) {
	return new(state.State), nil
}

func (t *mockStateTracker) Done() error {
	t.doneCalled = true
	return nil
}

func (t *mockStateTracker) assertDoneCalled(c *gc.C) {
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if t.doneCalled {
			return
		}
	}
	c.Fatal("Done() not called on tracker")
}
