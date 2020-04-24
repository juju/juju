// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftflag_test

import (
	coreraft "github.com/hashicorp/raft"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/engine"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft"
	"github.com/juju/juju/worker/raft/raftflag"
	"github.com/juju/juju/worker/raft/rafttest"
)

type workerFixture struct {
	rafttest.RaftFixture
	config raftflag.Config
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.FSM = &raft.SimpleFSM{}
	s.RaftFixture.SetUpTest(c)
	s.config = raftflag.Config{
		Raft: s.Raft,
	}
}

type WorkerValidationSuite struct {
	workerFixture
}

var _ = gc.Suite(&WorkerValidationSuite{})

func (s *WorkerValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*raftflag.Config)
		expect string
	}
	tests := []test{{
		func(cfg *raftflag.Config) { cfg.Raft = nil },
		"nil Raft not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *gc.C, f func(*raftflag.Config), expect string) {
	config := s.config
	f(&config)
	w, err := raftflag.NewWorker(config)
	if !c.Check(err, gc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

type WorkerSuite struct {
	workerFixture
	worker worker.Worker
	flag   engine.Flag
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)
	worker, err := raftflag.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	s.worker = worker
	s.flag = worker.(engine.Flag)
}

func (s *WorkerSuite) TestCleanKill(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestCheckLeader(c *gc.C) {
	c.Assert(s.Raft.VerifyLeader().Error(), jc.ErrorIsNil)
	c.Assert(s.flag.Check(), jc.IsTrue)
}

func (s *WorkerSuite) TestErrRefresh(c *gc.C) {
	raft1, _, transport1, _, _ := s.NewRaft(c, "1", &raft.SimpleFSM{})
	raft2, _, transport2, _, _ := s.NewRaft(c, "2", &raft.SimpleFSM{})
	transports := []coreraft.LoopbackTransport{s.Transport, transport1, transport2}
	for _, t1 := range transports {
		for _, t2 := range transports {
			t1.Connect(t2.LocalAddr(), t2)
		}
	}
	var f coreraft.Future = s.Raft.AddVoter("1", transport1.LocalAddr(), 0, 0)
	c.Assert(f.Error(), jc.ErrorIsNil)
	f = s.Raft.AddVoter("2", transport2.LocalAddr(), 0, 0)
	c.Assert(f.Error(), jc.ErrorIsNil)

	// Start a new raftflag worker for the second raft.
	newFlagWorker := func(r *coreraft.Raft) (worker.Worker, bool) {
		config := s.config
		config.Raft = r
		worker, err := raftflag.NewWorker(config)
		c.Assert(err, jc.ErrorIsNil)
		s.AddCleanup(func(c *gc.C) {
			workertest.DirtyKill(c, worker)
		})
		return worker, worker.(engine.Flag).Check()
	}
	worker1, flag1 := newFlagWorker(raft1)
	worker2, flag2 := newFlagWorker(raft2)
	c.Assert(flag1, jc.IsFalse)
	c.Assert(flag2, jc.IsFalse)

	// Shutdown the original node, causing one of the other
	// two nodes to become the leader.
	f = s.Raft.Shutdown()
	c.Assert(f.Error(), jc.ErrorIsNil)

	// When the raft node toggles between leader/follower,
	// then the worker will exit with ErrRefresh.
	err := workertest.CheckKilled(c, s.worker)
	c.Assert(err, gc.Equals, raftflag.ErrRefresh)

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if raft1.State() == coreraft.Leader || raft2.State() == coreraft.Leader {
			break
		}
	}
	var leaderWorker, followerWorker worker.Worker
	switch {
	case raft1.State() == coreraft.Leader:
		c.Assert(raft2.State(), gc.Equals, coreraft.Follower)
		leaderWorker, followerWorker = worker1, worker2
	case raft2.State() == coreraft.Leader:
		c.Assert(raft1.State(), gc.Equals, coreraft.Follower)
		leaderWorker, followerWorker = worker2, worker1
	}
	err = workertest.CheckKilled(c, leaderWorker)
	c.Assert(err, gc.Equals, raftflag.ErrRefresh)
	workertest.CheckAlive(c, followerWorker)
}
