// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft_test

import (
	"log"
	"time"

	coreraft "github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft"
	"github.com/juju/juju/worker/raft/rafttest"
	"github.com/juju/juju/worker/raft/raftutil"
)

type workerFixture struct {
	testing.IsolationSuite
	fsm    *raft.SimpleFSM
	config raft.Config
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fsm = &raft.SimpleFSM{}
	s.config = raft.Config{
		FSM:        s.fsm,
		Logger:     loggo.GetLogger("juju.worker.raft_test"),
		StorageDir: c.MkDir(),
		LocalID:    "123",
		Transport:  s.newTransport("123"),
		Clock:      testclock.NewClock(time.Time{}),
	}
}

func (s *workerFixture) newTransport(address coreraft.ServerAddress) *coreraft.InmemTransport {
	_, transport := coreraft.NewInmemTransport(address)
	s.AddCleanup(func(c *gc.C) {
		c.Assert(transport.Close(), jc.ErrorIsNil)
	})
	return transport
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
		func(cfg *raft.Config) { cfg.Logger = nil },
		"nil Logger not valid",
	}, {
		func(cfg *raft.Config) { cfg.StorageDir = "" },
		"empty StorageDir not valid",
	}, {
		func(cfg *raft.Config) { cfg.LocalID = "" },
		"empty LocalID not valid",
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
	clock  *testclock.Clock
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

	// Make a new clock so the waits from the bootstrap aren't hanging
	// around. Use time.Now() as the start so the time can be compared
	// to raft.LastContact(), which unfortunately uses wallclock time.
	s.clock = testclock.NewClock(time.Now())
	s.config.Clock = s.clock
	s.config.NoLeaderTimeout = 4 * time.Second

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
		ID:       "123",
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

func (s *WorkerSuite) TestLogStore(c *gc.C) {
	_, err := s.worker.LogStore()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) newRaft(c *gc.C, id coreraft.ServerID) (
	*coreraft.Raft, *coreraft.InmemTransport,
) {
	transport := s.newTransport("")
	store := coreraft.NewInmemStore()
	raftConfig := coreraft.DefaultConfig()
	raftConfig.LocalID = id
	raftConfig.HeartbeatTimeout = 100 * time.Millisecond
	raftConfig.ElectionTimeout = raftConfig.HeartbeatTimeout
	raftConfig.LeaderLeaseTimeout = raftConfig.HeartbeatTimeout
	raftConfig.Logger = log.New(&raftutil.LoggoWriter{
		loggo.GetLogger("juju.worker.raft_test_" + string(id)),
		loggo.DEBUG,
	}, "", 0)
	r, err := coreraft.NewRaft(
		raftConfig,
		&raft.SimpleFSM{},
		store,
		store,
		coreraft.NewInmemSnapshotStore(),
		transport,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		c.Assert(r.Shutdown().Error(), jc.ErrorIsNil)
	})
	return r, transport
}

func (s *WorkerSuite) TestNoLeaderTimeout(c *gc.C) {
	// Get the raft node into a state where it has no contact with the
	// leader by adding 2 more nodes, demoting the local one so that
	// it isn't the leader, then stopping the other nodes.
	transport0 := s.config.Transport.(coreraft.LoopbackTransport)
	raft1, transport1 := s.newRaft(c, "1")
	raft2, transport2 := s.newRaft(c, "2")
	connectTransports(transport0, transport1, transport2)

	raft0 := s.waitLeader(c)
	f1 := raft0.AddVoter("1", transport1.LocalAddr(), 0, 0)
	f2 := raft0.AddVoter("2", transport2.LocalAddr(), 0, 0)
	c.Assert(f1.Error(), jc.ErrorIsNil)
	c.Assert(f2.Error(), jc.ErrorIsNil)
	// Now that we are leader, check that we are listed as such
	initialLeader := raft0.Leader()

	rafttest.CheckConfiguration(c, raft0, []coreraft.Server{{
		ID:       "123",
		Address:  coreraft.ServerAddress("localhost"),
		Suffrage: coreraft.Voter,
	}, {
		ID:       "1",
		Address:  transport1.LocalAddr(),
		Suffrage: coreraft.Voter,
	}, {
		ID:       "2",
		Address:  transport2.LocalAddr(),
		Suffrage: coreraft.Voter,
	}})

	c.Logf("demoting self")
	f3 := raft0.DemoteVoter("123", 0, 0)
	c.Assert(f3.Error(), jc.ErrorIsNil)
	// Wait until raft0 isn't the leader anymore.
	leader := true
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		curLeader := raft0.Leader()
		c.Logf("found leader: %q", curLeader)
		leader = (curLeader == initialLeader)
		if !leader {
			break
		}
	}
	c.Assert(leader, jc.IsFalse)

	// We do this fast enough that both secondaries cannot have synchronized yet
	// so they aren't actually eligible to become leaders. They should still
	// shutdown cleanly.

	f4 := raft1.Shutdown()
	f5 := raft2.Shutdown()
	c.Assert(f4.Error(), jc.ErrorIsNil)
	c.Assert(f5.Error(), jc.ErrorIsNil)

	// Now advance time to trigger the timeout. There should be 2
	// waits when we advance:
	// * the loop timeout wait from starting the worker
	// * the no leader timeout check in loop.
	c.Assert(s.clock.WaitAdvance(10*time.Second, coretesting.LongWait, 2), jc.ErrorIsNil)
	c.Assert(workertest.CheckKilled(c, s.worker), gc.Equals, raft.ErrNoLeaderTimeout)
}

// Connect the provided transport bidirectionally.
func connectTransports(transports ...coreraft.LoopbackTransport) {
	for _, t1 := range transports {
		for _, t2 := range transports {
			if t1 == t2 {
				continue
			}
			t1.Connect(t2.LocalAddr(), t2)
		}
	}
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
	testClock := testclock.NewClock(time.Time{})
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
