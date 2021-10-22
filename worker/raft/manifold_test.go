// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft_test

import (
	"io"
	"path/filepath"
	"time"

	coreraft "github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/raft/queue"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
	raftleasestore "github.com/juju/juju/state/raftlease"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/raft"
)

type ManifoldSuite struct {
	statetesting.StateSuite

	manifold     dependency.Manifold
	context      dependency.Context
	agent        *mockAgent
	transport    *coreraft.InmemTransport
	clock        *testclock.Clock
	fsm          *raft.SimpleFSM
	logger       loggo.Logger
	worker       *mockRaftWorker
	stateTracker *stubStateTracker
	target       raftlease.NotifyTarget
	queue        *queue.OpQueue
	apply        func(raft.Raft, raftlease.NotifyTarget, raft.ApplierMetrics, clock.Clock, raft.Logger) raft.LeaseApplier
	stub         testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.clock = testclock.NewClock(time.Time{})
	s.agent = &mockAgent{
		conf: mockAgentConfig{
			tag:     names.NewMachineTag("99"),
			dataDir: filepath.Join("data", "dir"),
		},
	}
	s.fsm = &raft.SimpleFSM{}
	s.logger = loggo.GetLogger("juju.worker.raft_test")
	s.worker = &mockRaftWorker{
		r:  &coreraft.Raft{},
		ls: &mockLogStore{},
	}
	s.target = &struct{ raftlease.NotifyTarget }{}
	s.queue = queue.NewOpQueue(s.clock)
	s.stateTracker = &stubStateTracker{
		pool: s.StatePool,
		done: make(chan struct{}),
	}
	s.apply = func(raft.Raft, raftlease.NotifyTarget, raft.ApplierMetrics, clock.Clock, raft.Logger) raft.LeaseApplier {
		return nil
	}
	s.stub.ResetCalls()

	_, transport := coreraft.NewInmemTransport(coreraft.ServerAddress(
		s.agent.conf.tag.Id(),
	))
	s.transport = transport
	s.AddCleanup(func(c *gc.C) {
		s.transport.Close()
	})

	s.context = s.newContext(nil)
	s.manifold = raft.Manifold(raft.ManifoldConfig{
		ClockName:     "clock",
		AgentName:     "agent",
		TransportName: "transport",
		StateName:     "state",
		FSM:           s.fsm,
		Logger:        s.logger,
		NewWorker:     s.newWorker,
		NewTarget:     s.newTarget,
		LeaseLog:      &noopLeaseLog{},
		Queue:         s.queue,
		NewApplier:    s.apply,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":     s.agent,
		"transport": s.transport,
		"clock":     s.clock,
		"state":     s.stateTracker,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config raft.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *ManifoldSuite) newTarget(st *state.State, logger raftleasestore.Logger) raftlease.NotifyTarget {
	s.stub.MethodCall(s, "NewTarget", st, logger)
	return s.target
}

var expectedInputs = []string{
	"clock", "agent", "transport", "state",
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	s.startWorkerClean(c)

	s.stub.CheckCallNames(c, "NewTarget", "NewWorker")
	args := s.stub.Calls()[1].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, raft.Config{})
	config := args[0].(raft.Config)

	// We can't compare apply operations functions, so just check it's not nil.
	c.Assert(config.NewApplier, gc.NotNil)
	config.NewApplier = nil

	c.Assert(config, jc.DeepEquals, raft.Config{
		FSM:          s.fsm,
		Logger:       s.logger,
		StorageDir:   filepath.Join(s.agent.conf.dataDir, "raft"),
		LocalID:      "99",
		Transport:    s.transport,
		Clock:        s.clock,
		Queue:        s.queue,
		NotifyTarget: s.target,
	})
}

func (s *ManifoldSuite) TestStoppingWorkerReleasesState(c *gc.C) {
	w := s.startWorkerClean(c)

	s.stateTracker.CheckCallNames(c, "Use")
	select {
	case <-s.stateTracker.done:
		c.Fatal("unexpected state release")
	case <-time.After(coretesting.ShortWait):
	}

	// Stopping the worker should cause the state to
	// eventually be released.
	workertest.CleanKill(c, w)

	s.stateTracker.waitDone(c)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	w := s.startWorkerClean(c)

	var r *coreraft.Raft
	err := s.manifold.Output(w, &r)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.Equals, s.worker.r)

	s.worker.CheckCallNames(c, "Raft")
}

func (s *ManifoldSuite) TestLogStoreOutput(c *gc.C) {
	w := s.startWorkerClean(c)

	var ls coreraft.LogStore
	err := s.manifold.Output(w, &ls)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ls, gc.Equals, s.worker.ls)

	s.worker.CheckCallNames(c, "LogStore")
}

func (s *ManifoldSuite) TestOutputRaftError(c *gc.C) {
	w := s.startWorkerClean(c)

	s.worker.SetErrors(errors.New("no Raft for you"))

	var r *coreraft.Raft
	err := s.manifold.Output(w, &r)
	c.Assert(err, gc.ErrorMatches, "no Raft for you")
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	cleanupW, ok := w.(*common.CleanupWorker)
	c.Assert(ok, gc.Equals, true)
	c.Assert(cleanupW.Worker, gc.Equals, s.worker)
	return w
}

type stubStateTracker struct {
	testing.Stub
	pool *state.StatePool
	done chan struct{}
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return s.pool, s.NextErr()
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

type noopLeaseLog struct {
	io.Writer
}

func (noopLeaseLog) Write(p []byte) (n int, err error) {
	return len(p), nil
}
