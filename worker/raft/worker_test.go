// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft_test

import (
	"time"

	coreraft "github.com/hashicorp/raft"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft"
	"github.com/juju/juju/worker/raft/rafttest"
	"github.com/juju/juju/worker/workertest"
)

type workerFixture struct {
	testing.IsolationSuite
	fsm    *rafttest.FSM
	config raft.Config
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fsm = &rafttest.FSM{}
	_, transport := coreraft.NewInmemTransport("machine-123")
	s.AddCleanup(func(c *gc.C) {
		c.Assert(transport.Close(), jc.ErrorIsNil)
	})
	s.config = raft.Config{
		FSM:        s.fsm,
		Logger:     loggo.GetLogger("juju.worker.raft_test"),
		StorageDir: c.MkDir(),
		Tag:        names.NewMachineTag("123"),
		Transport:  transport,
	}
}

type WorkerValidationSuite struct {
	workerFixture
}

var _ = gc.Suite(&WorkerValidationSuite{})

func (s *WorkerValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*raft.Config)
		expect string
	}
	tests := []test{{
		func(cfg *raft.Config) { cfg.FSM = nil },
		"nil FSM not valid",
	}, {
		func(cfg *raft.Config) { cfg.StorageDir = "" },
		"empty StorageDir not valid",
	}, {
		func(cfg *raft.Config) { cfg.Tag = nil },
		"nil Tag not valid",
	}, {
		func(cfg *raft.Config) { cfg.HeartbeatTimeout = time.Millisecond },
		"validating raft config: Heartbeat timeout is too low",
	}, {
		func(cfg *raft.Config) { cfg.Transport = nil },
		"nil Transport not valid",
	}, {
		func(cfg *raft.Config) {
			_, transport := coreraft.NewInmemTransport("321-enihcam")
			cfg.Transport = transport
		},
		`transport local address "321-enihcam" not valid, expected "machine-123"`,
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *gc.C, f func(*raft.Config), expect string) {
	config := s.config
	f(&config)
	w, err := raft.NewWorker(config)
	if !c.Check(err, gc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *WorkerValidationSuite) TestBootstrapFSM(c *gc.C) {
	s.config.Transport = nil
	err := raft.Bootstrap(s.config)
	c.Assert(err, gc.ErrorMatches, "non-nil FSM during Bootstrap not valid")
}

func (s *WorkerValidationSuite) TestBootstrapTransport(c *gc.C) {
	s.config.FSM = nil
	err := raft.Bootstrap(s.config)
	c.Assert(err, gc.ErrorMatches, "non-nil Transport during Bootstrap not valid")
}

type WorkerSuite struct {
	workerFixture
	worker *raft.Worker
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)

	// Speed up the tests.
	s.config.HeartbeatTimeout = 100 * time.Millisecond
	s.config.ElectionTimeout = s.config.HeartbeatTimeout
	s.config.LeaderLeaseTimeout = s.config.HeartbeatTimeout

	// Bootstrap before starting the worker.
	transport := s.config.Transport
	fsm := s.config.FSM
	s.config.Transport = nil
	s.config.FSM = nil
	err := raft.Bootstrap(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.config.Transport = transport
	s.config.FSM = fsm
	worker, err := raft.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	s.worker = worker
}

func (s *WorkerSuite) waitLeader(c *gc.C) *coreraft.Raft {
	r, err := s.worker.Raft()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.NotNil)

	select {
	case leader := <-r.LeaderCh():
		c.Assert(leader, jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for leadership change")
	}
	return r
}

func (s *WorkerSuite) TestRaft(c *gc.C) {
	r := s.waitLeader(c)

	f := r.Apply([]byte("command1"), time.Minute)
	c.Assert(f.Error(), jc.ErrorIsNil)
	c.Assert(f.Index(), gc.Equals, uint64(3))
	c.Assert(f.Response(), gc.Equals, 1)

	f = r.Apply([]byte("command2"), time.Minute)
	c.Assert(f.Error(), jc.ErrorIsNil)
	c.Assert(f.Index(), gc.Equals, uint64(4))
	c.Assert(f.Response(), gc.Equals, 2)

	c.Assert(s.fsm.Logs(), jc.DeepEquals, [][]byte{
		[]byte("command1"),
		[]byte("command2"),
	})
}

func (s *WorkerSuite) TestRaftWorkerStopped(c *gc.C) {
	s.worker.Kill()

	r, err := s.worker.Raft()
	c.Assert(err, gc.Equals, raft.ErrWorkerStopped)
	c.Assert(r, gc.IsNil)
}

func (s *WorkerSuite) TestRestoreSnapshot(c *gc.C) {
	r := s.waitLeader(c)

	f := r.Apply([]byte("command1"), time.Minute)
	c.Assert(f.Error(), jc.ErrorIsNil)
	c.Assert(f.Index(), gc.Equals, uint64(3))
	c.Assert(f.Response(), gc.Equals, 1)

	sf := r.Snapshot()
	c.Assert(sf.Error(), jc.ErrorIsNil)
	meta, rc, err := sf.Open()
	c.Assert(err, jc.ErrorIsNil)
	defer rc.Close()

	f = r.Apply([]byte("command2"), time.Minute)
	c.Assert(f.Error(), jc.ErrorIsNil)
	c.Assert(f.Index(), gc.Equals, uint64(4))
	c.Assert(f.Response(), gc.Equals, 2)

	err = r.Restore(meta, rc, time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fsm.Logs(), jc.DeepEquals, [][]byte{
		[]byte("command1"),
	})
}
