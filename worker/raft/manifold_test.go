// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft_test

import (
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
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/raft/notifyproxy"
	"github.com/juju/juju/core/raft/queue"
	"github.com/juju/juju/core/raftlease"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/raft"
)

type ManifoldSuite struct {
	statetesting.StateSuite

	manifold  dependency.Manifold
	context   dependency.Context
	agent     *mockAgent
	transport *coreraft.InmemTransport
	clock     *testclock.Clock
	fsm       *raft.SimpleFSM
	logger    loggo.Logger
	worker    *mockRaftWorker
	target    *notifyproxy.NotifyProxy
	queue     *queue.OpQueue
	apply     func(raft.Raft, raftlease.NotifyTarget, raft.ApplierMetrics, clock.Clock, raft.Logger) raft.LeaseApplier
	stub      testing.Stub
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
	s.target = notifyproxy.New()
	s.queue = queue.NewOpQueue(s.clock)
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
		FSM:           s.fsm,
		Logger:        s.logger,
		NewWorker:     s.newWorker,
		Queue:         s.queue,
		NewApplier:    s.apply,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":     s.agent,
		"transport": s.transport,
		"clock":     s.clock,
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

var expectedInputs = []string{
	"clock", "agent", "transport",
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
		FSM:        s.fsm,
		Logger:     s.logger,
		StorageDir: filepath.Join(s.agent.conf.dataDir, "raft"),
		LocalID:    "99",
		Transport:  s.transport,
		Clock:      s.clock,
		Queue:      s.queue,
		NewNotifyTarget: func() *notifyproxy.NotifyProxy {
			return s.target
		},
	})
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
