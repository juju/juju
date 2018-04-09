// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftclusterer_test

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
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
	stub   testing.Stub
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)

	s.stub.ResetCalls()
	s.hub.Subscribe(
		apiserver.DetailsRequestTopic,
		func(topic string, req apiserver.DetailsRequest, err error) {
			s.stub.AddCall("DetailsRequest", req, err)
		},
	)

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
	connectTransports(s.Transport, transport1, transport2, transport3)

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
	rafttest.CheckConfiguration(c, s.Raft, []raft.Server{{
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
	rafttest.CheckConfiguration(c, raft1, []raft.Server{{
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
	// This test asserts that a
	// configuration change which updates a raft leader's address does not
	// result in a leadership change.

	// Machine-0's address will be updated to a non-localhost address, and
	// two new servers are added.

	// We add machine-1 and machine-2, and change machine-0's
	// address. Changing machine-0's address should not affect
	// its leadership.
	raft1, _, transport1, _, _ := s.NewRaft(c, "machine-1", &rafttest.FSM{})
	_, _, transport2, _, _ := s.NewRaft(c, "machine-2", &rafttest.FSM{})
	connectTransports(s.Transport, transport1, transport2)
	machine1Address := string(transport1.LocalAddr())
	machine2Address := string(transport2.LocalAddr())

	alternateAddress := "testing.invalid:1234"
	c.Assert(s.Raft.Leader(), gc.Not(gc.Equals), alternateAddress)
	s.publishDetails(c, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				InternalAddress: alternateAddress,
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
	//Check configuration asserts that the raft configuration should have
	//been updated to reflect the two added machines and that the address of
	//the leader has been changed.
	rafttest.CheckConfiguration(c, raft1, []raft.Server{{
		ID:       "machine-0",
		Address:  raft.ServerAddress(alternateAddress),
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

	// machine-0 should still be the leader
	future := s.Raft.VerifyLeader()
	c.Assert(future.Error(), jc.ErrorIsNil)
}

func (s *WorkerSuite) TestRequestsDetails(c *gc.C) {
	s.stub.CheckCall(c, 0, "DetailsRequest", apiserver.DetailsRequest{Requester: "raft-clusterer"}, nil)
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

// Connect the provided transport bidirectionally.
func connectTransports(transports ...raft.LoopbackTransport) {
	for _, t1 := range transports {
		for _, t2 := range transports {
			if t1 == t2 {
				continue
			}
			t1.Connect(t2.LocalAddr(), t2)
		}
	}
}
