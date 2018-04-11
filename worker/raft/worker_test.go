// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft_test

import (
	"time"

	coreraft "github.com/hashicorp/raft"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft"
	"github.com/juju/juju/worker/workertest"
)

type workerFixture struct {
	testing.IsolationSuite
	fsm    *raft.SimpleFSM
	config raft.Config
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fsm = &raft.SimpleFSM{}
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
		Clock:      testing.NewClock(time.Time{}),
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
		func(cfg *raft.Config) { cfg.Clock = nil },
		"nil Clock not valid",
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
	s.worker = worker.(*raft.Worker)
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

func (s *WorkerSuite) TestBootstrapAddress(c *gc.C) {
	r := s.waitLeader(c)

	f := r.GetConfiguration()
	c.Assert(f.Error(), jc.ErrorIsNil)
	c.Assert(f.Configuration().Servers, jc.DeepEquals, []coreraft.Server{{
		Suffrage: coreraft.Voter,
		ID:       "machine-123",
		Address:  "localhost",
	}})
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

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestShutdownRaftKillsWorker(c *gc.C) {
	r := s.waitLeader(c)
	c.Assert(r.Shutdown().Error(), jc.ErrorIsNil)

	err := workertest.CheckKilled(c, s.worker)
	c.Assert(err, gc.ErrorMatches, "raft shutdown")
}

type WorkerTimeoutSuite struct {
	workerFixture
}

var _ = gc.Suite(&WorkerTimeoutSuite{})

func (s *WorkerTimeoutSuite) SetUpTest(c *gc.C) {
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
}

func (s *WorkerTimeoutSuite) TestNewWorkerTimesOut(c *gc.C) {
	// If for some reason it takes a long time to create the Raft
	// object we don't want to just hang - that can make it really
	// hard to work out what's going on. Instead we should timeout if
	// the raft loop doesn't get started.
	testClock := testing.NewClock(time.Time{})
	s.config.Clock = testClock
	_, underlying := coreraft.NewInmemTransport("something")
	s.config.Transport = &hangingTransport{
		Transport: underlying,
		clock:     testClock,
	}
	errChan := make(chan error)
	go func() {
		w, err := raft.NewWorker(s.config)
		c.Check(w, gc.IsNil)
		errChan <- err
	}()

	// We wait for the transport and the worker to be waiting for the
	// clock, then we move it past the timeout.
	err := testClock.WaitAdvance(2*raft.LoopTimeout, coretesting.LongWait, 2)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-errChan:
		c.Assert(err, gc.ErrorMatches, "timed out waiting for worker loop")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker error")
	}
}

type hangingTransport struct {
	coreraft.Transport
	clock clock.Clock
}

func (t *hangingTransport) LocalAddr() coreraft.ServerAddress {
	<-t.clock.After(5 * raft.LoopTimeout)
	return t.Transport.LocalAddr()
}
