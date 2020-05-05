// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/globalclockupdater"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	stub         testing.Stub
	config       globalclockupdater.ManifoldConfig
	stateTracker stubStateTracker
	worker       worker.Worker
	logger       loggo.Logger
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub.ResetCalls()
	s.logger = loggo.GetLogger("globalclockupdater_test")
	s.config = globalclockupdater.ManifoldConfig{
		Clock:          fakeClock{},
		StateName:      "state",
		NewWorker:      s.newWorker,
		UpdateInterval: time.Second,
		BackoffDelay:   time.Second,
		Logger:         s.logger,
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
	expectInputs := []string{"state"}
	c.Check(manifold.Inputs, jc.SameContents, expectInputs)
}

func (s *ManifoldSuite) TestLeaseManagerInputs(c *gc.C) {
	s.config.StateName = ""
	s.config.LeaseManagerName = "lease-manager"
	manifold := globalclockupdater.Manifold(s.config)
	expectInputs := []string{"lease-manager"}
	c.Check(manifold.Inputs, jc.SameContents, expectInputs)
}

func (s *ManifoldSuite) TestLeaseManagerAndRaftInputs(c *gc.C) {
	s.config.StateName = ""
	s.config.LeaseManagerName = "lease-manager"
	s.config.RaftName = "raft"
	manifold := globalclockupdater.Manifold(s.config)
	expectInputs := []string{"lease-manager", "raft"}
	c.Check(manifold.Inputs, jc.SameContents, expectInputs)
}

func (s *ManifoldSuite) TestStartValidateClock(c *gc.C) {
	s.config.Clock = nil
	s.testStartValidateConfig(c, "nil Clock not valid")
}

func (s *ManifoldSuite) TestStartValidateStateName(c *gc.C) {
	s.config.StateName = ""
	s.testStartValidateConfig(c, "both StateName and LeaseManagerName empty not valid")
}

func (s *ManifoldSuite) TestStartValidateNotBoth(c *gc.C) {
	s.config.LeaseManagerName = "lease-manager"
	s.testStartValidateConfig(c, "only one of StateName and LeaseManagerName can be set")
}

func (s *ManifoldSuite) TestStartValidateNotRaftAndState(c *gc.C) {
	s.config.RaftName = "raft"
	s.testStartValidateConfig(c, "RaftName only valid with LeaseManagerName")
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
		"state": nil,
	})
	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingLeaseManager(c *gc.C) {
	s.config.StateName = ""
	s.config.LeaseManagerName = "lease-manager"
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"lease-manager": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingRaft(c *gc.C) {
	updater := fakeUpdater{}
	s.config.StateName = ""
	s.config.LeaseManagerName = "lease-manager"
	s.config.RaftName = "raft"
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"lease-manager": &updater,
		"raft":          dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingState(c *gc.C) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
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
		Logger:         s.logger,
	})
}

func (s *ManifoldSuite) TestStartNewWorkerSuccessWithLeaseManager(c *gc.C) {
	updater := fakeUpdater{}
	s.config.StateName = ""
	s.config.LeaseManagerName = "lease-manager"
	s.config.RaftName = "raft"
	worker, err := s.startManifoldWithContext(c, map[string]interface{}{
		"clock":         fakeClock{},
		"lease-manager": &updater,
		"raft":          nil,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.worker)

	s.stub.CheckCallNames(c, "NewWorker")
	config := s.stub.Calls()[0].Args[0].(globalclockupdater.Config)
	c.Assert(config.NewUpdater, gc.NotNil)
	actualUpdater, err := config.NewUpdater()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actualUpdater, gc.Equals, &updater)
	config.NewUpdater = nil
	c.Assert(config, jc.DeepEquals, globalclockupdater.Config{
		LocalClock:     fakeClock{},
		UpdateInterval: s.config.UpdateInterval,
		BackoffDelay:   s.config.BackoffDelay,
		Logger:         s.logger,
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

	s.stateTracker.waitDone(c)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) startManifold(c *gc.C) (worker.Worker, error) {
	worker, err := s.startManifoldWithContext(c, map[string]interface{}{
		"clock": fakeClock{},
		"state": &s.stateTracker,
	})
	if err != nil {
		return nil, err
	}
	// Add a cleanup to wait for the worker to be done; this
	// is necessary to avoid races.
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
		s.stateTracker.waitDone(c)
	})
	return worker, err
}

func (s *ManifoldSuite) startManifoldWithContext(c *gc.C, data map[string]interface{}) (worker.Worker, error) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, data)
	worker, err := manifold.Start(context)
	if err != nil {
		return nil, err
	}
	return worker, nil
}

type fakeClock struct {
	clock.Clock
}

type fakeUpdater struct {
	globalclock.Updater
}

type stubStateTracker struct {
	testing.Stub
	pool state.StatePool
	done chan struct{}
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return &s.pool, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	err := s.NextErr()
	// close must be the last read or write on stubStateTracker in Done
	close(s.done)
	return err
}

func (s *stubStateTracker) waitDone(c *gc.C) {
	select {
	case <-s.done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for state to be released")
	}
}

func (s *stubStateTracker) Report() map[string]interface{} {
	s.MethodCall(s, "Report")
	return nil
}
