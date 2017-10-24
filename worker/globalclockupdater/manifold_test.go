// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/globalclockupdater"
	"github.com/juju/juju/worker/workertest"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	stub         testing.Stub
	config       globalclockupdater.ManifoldConfig
	stateTracker stubStateTracker
	worker       worker.Worker
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub.ResetCalls()
	s.config = globalclockupdater.ManifoldConfig{
		ClockName:      "clock",
		StateName:      "state",
		NewWorker:      s.newWorker,
		UpdateInterval: time.Second,
		BackoffDelay:   time.Second,
	}
	s.stateTracker = stubStateTracker{
		done: make(chan struct{}),
	}
	s.worker = worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, s.worker) })
}

func (s *ManifoldSuite) newWorker(config globalclockupdater.Config) (worker.Worker, error) {
	s.stub.AddCall("NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	manifold := globalclockupdater.Manifold(s.config)
	expectInputs := []string{"clock", "state"}
	c.Check(manifold.Inputs, jc.DeepEquals, expectInputs)
}

func (s *ManifoldSuite) TestStartValidateClockName(c *gc.C) {
	s.config.ClockName = ""
	s.testStartValidateConfig(c, "empty ClockName not valid")
}

func (s *ManifoldSuite) TestStartValidateStateName(c *gc.C) {
	s.config.StateName = ""
	s.testStartValidateConfig(c, "empty StateName not valid")
}

func (s *ManifoldSuite) TestStartValidateUpdateInterval(c *gc.C) {
	s.config.UpdateInterval = 0
	s.testStartValidateConfig(c, "non-positive UpdateInterval not valid")
}

func (s *ManifoldSuite) TestStartValidateBackoffDelay(c *gc.C) {
	s.config.BackoffDelay = -1
	s.testStartValidateConfig(c, "non-positive BackoffDelay not valid")
}

func (s *ManifoldSuite) testStartValidateConfig(c *gc.C, expect string) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"clock": nil,
		"state": nil,
	})
	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingClock(c *gc.C) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"clock": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingState(c *gc.C) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"clock": fakeClock{},
		"state": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartStateTrackerError(c *gc.C) {
	s.stateTracker.SetErrors(errors.New("phail"))
	worker, err := s.startManifold(c)
	c.Check(err, gc.ErrorMatches, "phail")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartNewWorkerError(c *gc.C) {
	s.stub.SetErrors(errors.New("phail"))
	worker, err := s.startManifold(c)
	c.Check(err, gc.ErrorMatches, "phail")
	c.Check(worker, gc.IsNil)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) TestStartNewWorkerSuccess(c *gc.C) {
	worker, err := s.startManifold(c)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.worker)

	s.stub.CheckCallNames(c, "NewWorker")
	config := s.stub.Calls()[0].Args[0].(globalclockupdater.Config)
	c.Assert(config.NewUpdater, gc.NotNil)
	config.NewUpdater = nil
	c.Assert(config, jc.DeepEquals, globalclockupdater.Config{
		LocalClock:     fakeClock{},
		UpdateInterval: s.config.UpdateInterval,
		BackoffDelay:   s.config.BackoffDelay,
	})
}

func (s *ManifoldSuite) TestStoppingWorkerReleasesState(c *gc.C) {
	worker, err := s.startManifold(c)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.worker)

	s.stateTracker.CheckCallNames(c, "Use")
	select {
	case <-s.stateTracker.done:
		c.Fatal("unexpected state release")
	case <-time.After(coretesting.ShortWait):
	}

	// Stopping the worker should cause the state to
	// eventually be released.
	workertest.CleanKill(c, worker)

	select {
	case <-s.stateTracker.done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for state to be released")
	}
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) startManifold(c *gc.C) (worker.Worker, error) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"clock": fakeClock{},
		"state": &s.stateTracker,
	})
	return manifold.Start(context)
}

type fakeClock struct {
	clock.Clock
}

type stubStateTracker struct {
	testing.Stub
	st   state.State
	done chan struct{}
}

func (s *stubStateTracker) Use() (*state.State, error) {
	s.MethodCall(s, "Use")
	return &s.st, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	close(s.done)
	return s.NextErr()
}
