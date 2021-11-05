// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater_test

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
	raftleasestore "github.com/juju/juju/state/raftlease"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/globalclockupdater"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	stub         testing.Stub
	config       globalclockupdater.ManifoldConfig
	worker       worker.Worker
	stateTracker *stubStateTracker
	target       raftlease.NotifyTarget
	logger       loggo.Logger
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub.ResetCalls()
	s.logger = loggo.GetLogger("globalclockupdater_test")
	s.config = globalclockupdater.ManifoldConfig{
		Clock:          fakeClock{},
		RaftName:       "raft",
		StateName:      "state",
		FSM:            stubFSM{},
		NewWorker:      s.newWorker,
		NewTarget:      s.newTarget,
		UpdateInterval: time.Second,
		Logger:         s.logger,
	}
	s.worker = worker.NewRunner(worker.RunnerParams{})
	s.target = &noopTarget{}
	s.stateTracker = &stubStateTracker{
		done: make(chan struct{}),
	}
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, s.worker) })
}

func (s *ManifoldSuite) newWorker(config globalclockupdater.Config) (worker.Worker, error) {
	s.stub.AddCall("NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *ManifoldSuite) newTarget(st *state.State, logger raftleasestore.Logger) raftlease.NotifyTarget {
	s.stub.AddCall("NewTarget", st, logger)
	return s.target
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	manifold := globalclockupdater.Manifold(s.config)
	expectInputs := []string{"raft", "state"}
	c.Check(manifold.Inputs, jc.SameContents, expectInputs)
}

func (s *ManifoldSuite) TestStartValidateClock(c *gc.C) {
	s.config.Clock = nil
	s.testStartValidateConfig(c, "nil Clock not valid")
}

func (s *ManifoldSuite) TestStartValidateFSM(c *gc.C) {
	s.config.FSM = nil
	s.testStartValidateConfig(c, "nil FSM not valid")
}

func (s *ManifoldSuite) TestStartValidateRaftName(c *gc.C) {
	s.config.RaftName = ""
	s.testStartValidateConfig(c, "empty RaftName not valid")
}

func (s *ManifoldSuite) TestStartValidateStateName(c *gc.C) {
	s.config.StateName = ""
	s.testStartValidateConfig(c, "empty StateName not valid")
}

func (s *ManifoldSuite) TestStartValidateUpdateInterval(c *gc.C) {
	s.config.UpdateInterval = 0
	s.testStartValidateConfig(c, "non-positive UpdateInterval not valid")
}

func (s *ManifoldSuite) testStartValidateConfig(c *gc.C, expect string) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"raft": nil,
	})
	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingRaft(c *gc.C) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"raft": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartNewWorkerSuccess(c *gc.C) {
	worker, err := s.startManifoldWithContext(c, map[string]interface{}{
		"clock": fakeClock{},
		"raft":  new(raft.Raft),
		"state": s.stateTracker,
	})
	c.Check(err, jc.ErrorIsNil)
	cleanupW, ok := worker.(*common.CleanupWorker)
	c.Assert(ok, gc.Equals, true)
	c.Assert(cleanupW.Worker, gc.Equals, s.worker)

	s.stub.CheckCallNames(c, "NewTarget", "NewWorker")

	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 2)
	c.Assert(args[0], gc.Equals, s.stateTracker.pool.SystemState())
	var logWriter raftleasestore.Logger
	c.Assert(args[1], gc.Implements, &logWriter)

	config := s.stub.Calls()[1].Args[0].(globalclockupdater.Config)
	c.Assert(config.NewUpdater, gc.NotNil)

	config.NewUpdater = nil
	c.Assert(config, jc.DeepEquals, globalclockupdater.Config{
		LocalClock:     fakeClock{},
		UpdateInterval: s.config.UpdateInterval,
		Logger:         s.logger,
	})
}

func (s *ManifoldSuite) TestStoppingWorkerReleasesState(c *gc.C) {
	worker, err := s.startManifoldWithContext(c, map[string]interface{}{
		"clock": fakeClock{},
		"raft":  new(raft.Raft),
		"state": s.stateTracker,
	})
	c.Check(err, jc.ErrorIsNil)
	cleanupW, ok := worker.(*common.CleanupWorker)
	c.Assert(ok, gc.Equals, true)
	c.Assert(cleanupW.Worker, gc.Equals, s.worker)

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

type stubFSM struct{}

func (stubFSM) GlobalTime() time.Time {
	return time.Now()
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
	close(s.done)
	return err
}

func (s *stubStateTracker) Report() map[string]interface{} {
	return map[string]interface{}{"hey": "mum"}
}

func (s *stubStateTracker) waitDone(c *gc.C) {
	select {
	case <-s.done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for state to be released")
	}
}

type noopTarget struct{}

func (noopTarget) Claimed(lease.Key, string) error {
	return nil
}

// Expired will be called when an existing lease has expired. Not
// allowed to return an error because this is purely advisory.
func (noopTarget) Expired(lease.Key) error {
	return nil
}
