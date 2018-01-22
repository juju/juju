// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftflag_test

import (
	"github.com/hashicorp/raft"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/raft/raftflag"
	"github.com/juju/juju/worker/raft/rafttest"
	"github.com/juju/juju/worker/workertest"
)

type workerFixture struct {
	rafttest.RaftFixture
	config raftflag.Config
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.FSM = &rafttest.FSM{}
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
	addr, transport := raft.NewInmemTransport("machine-1")
	defer transport.Close()
	s.Transport.Connect(addr, transport)
	defer s.Transport.Disconnect(addr)

	store := raft.NewInmemStore()
	snapshotStore := raft.NewInmemSnapshotStore()
	raftConfig := s.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(string(addr))
	fsm := &rafttest.FSM{}
	raft2, err := raft.NewRaft(raftConfig, fsm, store, store, snapshotStore, transport)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		f := raft2.Shutdown()
		c.Assert(f.Error(), jc.ErrorIsNil)
	}()
	f := s.Raft.AddVoter("machine-1", addr, 0, 0)
	c.Assert(f.Error(), jc.ErrorIsNil)

	// Start a new raftflag worker for the second raft.
	config2 := s.config
	config2.Raft = raft2
	worker2, err := raftflag.NewWorker(config2)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		workertest.DirtyKill(c, worker2)
	}()
	flag2 := worker2.(engine.Flag)
	c.Assert(flag2.Check(), jc.IsFalse)

	// Demote the original node, causing the new node to
	// become the leader.
	f = s.Raft.DemoteVoter(raft.ServerID(string(s.Transport.LocalAddr())), 0, 0)
	c.Assert(f.Error(), jc.ErrorIsNil)

	// When the raft node toggles between leader/follower,
	// then the worker will exit with ErrRefresh.
	err = s.worker.Wait()
	c.Assert(err, gc.Equals, raftflag.ErrRefresh)
	err = worker2.Wait()
	c.Assert(err, gc.Equals, raftflag.ErrRefresh)
}
