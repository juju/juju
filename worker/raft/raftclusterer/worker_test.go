// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftclusterer_test

import (
	"reflect"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/pubsub"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/centralhub"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft/raftclusterer"
	"github.com/juju/juju/worker/raft/rafttest"
	"github.com/juju/juju/worker/workertest"
)

type workerFixture struct {
	rafttest.RaftFixture
	hub    *pubsub.StructuredHub
	config raftclusterer.Config
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.FSM = &rafttest.FSM{}
	s.RaftFixture.SetUpTest(c)
	s.hub = centralhub.New(names.NewMachineTag("0"))
	s.config = raftclusterer.Config{
		Raft: s.Raft,
		Hub:  s.hub,
	}
}

type WorkerValidationSuite struct {
	workerFixture
}

var _ = gc.Suite(&WorkerValidationSuite{})

func (s *WorkerValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*raftclusterer.Config)
		expect string
	}
	tests := []test{{
		func(cfg *raftclusterer.Config) { cfg.Raft = nil },
		"nil Raft not valid",
	}, {
		func(cfg *raftclusterer.Config) { cfg.Hub = nil },
		"nil Hub not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *gc.C, f func(*raftclusterer.Config), expect string) {
	config := s.config
	f(&config)
	w, err := raftclusterer.NewWorker(config)
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
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)
	worker, err := raftclusterer.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	s.worker = worker
}

func (s *WorkerSuite) TestCleanKill(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestAddRemoveServers(c *gc.C) {
	// Create 4 servers: machine-0, machine-1, machine-2,
	// and machine-3, where all servers can connect
	// bidirectionally.
	raft1, _, transport1, _, _ := s.NewRaft(c, "machine-1", &rafttest.FSM{})
	_, _, transport2, _, _ := s.NewRaft(c, "machine-2", &rafttest.FSM{})
	_, _, transport3, _, _ := s.NewRaft(c, "machine-3", &rafttest.FSM{})
	transports := []raft.LoopbackTransport{s.Transport, transport1, transport2, transport3}
	for _, t1 := range transports {
		for _, t2 := range transports {
			if t1 == t2 {
				continue
			}
			t1.Connect(t2.LocalAddr(), t2)
		}
	}

	machine0Address := string(s.Transport.LocalAddr())
	machine1Address := string(transport1.LocalAddr())
	machine2Address := string(transport2.LocalAddr())
	machine3Address := string(transport3.LocalAddr())

	raft1Observations := make(chan raft.Observation, 1)
	raft1Observer := raft.NewObserver(raft1Observations, false, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	raft1.RegisterObserver(raft1Observer)
	defer raft1.DeregisterObserver(raft1Observer)

	// Add machine-1, machine-2.
	s.publishDetails(c, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				InternalAddress: machine0Address,
			},
			"1": {
				ID:              "1",
				InternalAddress: machine1Address,
			},
			"2": {
				ID:              "2",
				InternalAddress: machine2Address,
			},
		},
	})
	waitConfiguration(c, s.Raft, []raft.Server{{
		ID:       "machine-0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "machine-1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "machine-2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}})

	select {
	case <-raft1Observations:
		c.Assert(raft1.Leader(), gc.Equals, s.Transport.LocalAddr())
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for leader observation")
	}

	// Remove machine-1, add machine-3.
	s.publishDetails(c, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				InternalAddress: machine0Address,
			},
			"2": {
				ID:              "2",
				InternalAddress: machine2Address,
			},
			"3": {
				ID:              "3",
				InternalAddress: machine3Address,
			},
		},
	})
	waitConfiguration(c, raft1, []raft.Server{{
		ID:       "machine-0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "machine-2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "machine-3",
		Address:  raft.ServerAddress(machine3Address),
		Suffrage: raft.Voter,
	}})
}

func (s *WorkerSuite) TestChangeLocalServer(c *gc.C) {
	// Create 3 servers, machine-0, machine-1, and machine-2.
	// The latter two servers can communicate bidirectionally;
	// only machine-0 can dial the others, and not the other
	// way around.
	raft1, _, transport1, _, _ := s.NewRaft(c, "machine-1", &rafttest.FSM{})
	_, _, transport2, _, _ := s.NewRaft(c, "machine-2", &rafttest.FSM{})
	s.Transport.Connect(transport1.LocalAddr(), transport1)
	s.Transport.Connect(transport2.LocalAddr(), transport2)
	transport1.Connect(transport2.LocalAddr(), transport2)
	transport2.Connect(transport1.LocalAddr(), transport1)
	machine1Address := string(transport1.LocalAddr())
	machine2Address := string(transport2.LocalAddr())

	raft1Observations := make(chan raft.Observation, 10)
	raft1Observer := raft.NewObserver(raft1Observations, false, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	raft1.RegisterObserver(raft1Observer)
	defer close(raft1Observations)
	defer raft1.DeregisterObserver(raft1Observer)

	newLeaderElected := make(chan struct{})
	go func() {
		for {
			select {
			case _, ok := <-raft1Observations:
				if !ok {
					return
				}
			}
			leaderAddr := raft1.Leader()
			if leaderAddr != "" && leaderAddr != s.Transport.LocalAddr() {
				close(newLeaderElected)
				return
			}
		}
	}()

	// Here we simulate "ensure-ha": machine-0's address will
	// be updated to a non-localhost address, and two new
	// servers are added.
	//
	// We add machine-1 and machine-2, and change machine-0's
	// address. Changing machine-0's address should not affect
	// its leadership.
	s.publishDetails(c, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				InternalAddress: "testing.invalid:1234",
			},
			"1": {
				ID:              "1",
				InternalAddress: machine1Address,
			},
			"2": {
				ID:              "2",
				InternalAddress: machine2Address,
			},
		},
	})
	waitConfiguration(c, raft1, []raft.Server{{
		ID:       "machine-0",
		Address:  "testing.invalid:1234",
		Suffrage: raft.Voter,
	}, {
		ID:       "machine-1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "machine-2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}})

	// Having the leader change its own address
	// should not cause it to step down.
	future := s.Raft.VerifyLeader()
	c.Assert(future.Error(), jc.ErrorIsNil)
}

func (s *WorkerSuite) publishDetails(c *gc.C, details apiserver.Details) {
	received, err := s.hub.Publish(apiserver.DetailsTopic, details)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-received:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for details to be received")
	}
}

func waitConfiguration(c *gc.C, r *raft.Raft, expectedServers []raft.Server) {
	var configuration raft.Configuration
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		f := r.GetConfiguration()
		c.Assert(f.Error(), jc.ErrorIsNil)
		configuration = f.Configuration()
		if reflect.DeepEqual(configuration.Servers, expectedServers) {
			return
		}
	}
	c.Assert(
		configuration.Servers, jc.SameContents, expectedServers,
		gc.Commentf(
			"waited %s and still did not see the expected configuration",
			coretesting.LongAttempt.Total,
		),
	)
}
