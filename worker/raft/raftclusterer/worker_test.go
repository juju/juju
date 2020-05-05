// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftclusterer_test

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/centralhub"
	coretesting "github.com/juju/juju/testing"
	jujuraft "github.com/juju/juju/worker/raft"
	"github.com/juju/juju/worker/raft/raftclusterer"
	"github.com/juju/juju/worker/raft/rafttest"
)

type workerFixture struct {
	rafttest.RaftFixture
	hub    *pubsub.StructuredHub
	config raftclusterer.Config
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.FSM = &jujuraft.SimpleFSM{}
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
	reqs   chan apiserver.DetailsRequest
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)
	s.reqs = make(chan apiserver.DetailsRequest, 10)

	// Use a local variable to send to the channel in the callback, so
	// we don't get races when a subsequent test overwrites s.reqs
	// with a new channel.
	reqs := s.reqs
	unsubscribe, err := s.hub.Subscribe(
		apiserver.DetailsRequestTopic,
		func(topic string, req apiserver.DetailsRequest, err error) {
			c.Check(err, jc.ErrorIsNil)
			reqs <- req
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { unsubscribe() })

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
	// Create 4 servers: 0, 1, 2, and 3, where all servers can connect
	// bidirectionally.
	raft1, _, transport1, _, _ := s.NewRaft(c, "1", &jujuraft.SimpleFSM{})
	_, _, transport2, _, _ := s.NewRaft(c, "2", &jujuraft.SimpleFSM{})
	_, _, transport3, _, _ := s.NewRaft(c, "3", &jujuraft.SimpleFSM{})
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

	// Add machines 1 and 2.
	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"1": machine1Address,
		"2": machine2Address,
	})
	rafttest.CheckConfiguration(c, s.Raft, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}})

	select {
	case <-raft1Observations:
		c.Assert(raft1.Leader(), gc.Equals, s.Transport.LocalAddr())
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for leader observation")
	}

	// Remove machine 1, add machine 3.
	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"2": machine2Address,
		"3": machine3Address,
	})
	rafttest.CheckConfiguration(c, raft1, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "3",
		Address:  raft.ServerAddress(machine3Address),
		Suffrage: raft.Voter,
	}})
}

func (s *WorkerSuite) TestChangeLocalServer(c *gc.C) {
	// This test asserts that a configuration change which updates a
	// raft leader's address does not result in a leadership change.

	// Machine 0's address will be updated to a non-localhost address, and
	// two new servers are added.

	// We add 1 and 2, and change 0's address. Changing machine 0's
	// address should not affect its leadership.
	raft1, _, transport1, _, _ := s.NewRaft(c, "1", &jujuraft.SimpleFSM{})
	_, _, transport2, _, _ := s.NewRaft(c, "2", &jujuraft.SimpleFSM{})
	connectTransports(s.Transport, transport1, transport2)
	machine1Address := string(transport1.LocalAddr())
	machine2Address := string(transport2.LocalAddr())

	alternateAddress := "testing.invalid:1234"
	c.Assert(s.Raft.Leader(), gc.Not(gc.Equals), alternateAddress)
	s.publishDetails(c, map[string]string{
		"0": alternateAddress,
		"1": machine1Address,
		"2": machine2Address,
	})
	//Check configuration asserts that the raft configuration should have
	//been updated to reflect the two added machines and that the address of
	//the leader has been changed.
	rafttest.CheckConfiguration(c, raft1, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(alternateAddress),
		Suffrage: raft.Voter,
	}, {
		ID:       "1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}})

	// Machine 0 should still be the leader.
	future := s.Raft.VerifyLeader()
	c.Assert(future.Error(), jc.ErrorIsNil)
}

func (s *WorkerSuite) TestDisappearingAddresses(c *gc.C) {
	// If we had 3 servers but the peergrouper publishes an update
	// that sets all of their addresses to "", ignore that change.
	_, _, transport1, _, _ := s.NewRaft(c, "1", &jujuraft.SimpleFSM{})
	_, _, transport2, _, _ := s.NewRaft(c, "2", &jujuraft.SimpleFSM{})
	connectTransports(s.Transport, transport1, transport2)
	machine0Address := string(s.Transport.LocalAddr())
	machine1Address := string(transport1.LocalAddr())
	machine2Address := string(transport2.LocalAddr())

	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"1": machine1Address,
		"2": machine2Address,
	})
	expectedConfiguration := []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}}
	rafttest.CheckConfiguration(c, s.Raft, expectedConfiguration)

	s.publishDetails(c, map[string]string{
		"0": "",
		"1": "",
		"2": "",
	})
	// Check that it ignores the update - removing all servers isn't
	// something that we should allow.
	rafttest.CheckConfiguration(c, s.Raft, expectedConfiguration)

	// But publishing an update with one machines with a blank address
	// should still remove it.
	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"1": "",
		"2": machine2Address,
	})
	// Machine "2" should be demoted to keep an odd number of voters.
	expectedConfiguration[2].Suffrage = raft.Nonvoter
	rafttest.CheckConfiguration(c, s.Raft, []raft.Server{
		expectedConfiguration[0],
		expectedConfiguration[2],
	})
}

func (s *WorkerSuite) TestRequestsDetails(c *gc.C) {
	// The worker is started in SetUpTest.
	select {
	case req := <-s.reqs:
		c.Assert(req, gc.Equals, apiserver.DetailsRequest{
			Requester: "raft-clusterer",
			LocalOnly: true,
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for details request")
	}
}

func (s *WorkerSuite) TestDemotesAServerWhenThereAre2(c *gc.C) {
	// Create 3 servers: 0, 1 and 2, where all servers can connect
	// bidirectionally.
	raft1, _, transport1, _, _ := s.NewRaft(c, "1", &jujuraft.SimpleFSM{})
	raft2, _, transport2, _, _ := s.NewRaft(c, "2", &jujuraft.SimpleFSM{})
	connectTransports(s.Transport, transport1, transport2)

	machine0Address := string(s.Transport.LocalAddr())
	machine1Address := string(transport1.LocalAddr())
	machine2Address := string(transport2.LocalAddr())

	raft1Observations := make(chan raft.Observation, 1)
	raft1Observer := raft.NewObserver(raft1Observations, false, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	raft1.RegisterObserver(raft1Observer)
	defer raft1.DeregisterObserver(raft1Observer)

	// Add machines 1 and 2.
	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"1": machine1Address,
		"2": machine2Address,
	})
	rafttest.CheckConfiguration(c, s.Raft, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}})

	select {
	case <-raft1Observations:
		c.Assert(raft1.Leader(), gc.Equals, s.Transport.LocalAddr())
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for leader observation")
	}

	// Remove machine 1.
	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"2": machine2Address,
	})
	f := raft1.Shutdown()
	c.Assert(f.Error(), jc.ErrorIsNil)
	rafttest.CheckConfiguration(c, raft2, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Nonvoter,
	}})
}

func (s *WorkerSuite) TestPromotesAServerWhenThereAre3Again(c *gc.C) {
	// Create 2 servers: 0 and 1, where both servers can connect
	// bidirectionally.
	raft1, _, transport1, _, _ := s.NewRaft(c, "1", &jujuraft.SimpleFSM{})
	connectTransports(s.Transport, transport1)

	machine0Address := string(s.Transport.LocalAddr())
	machine1Address := string(transport1.LocalAddr())

	raft1Observations := make(chan raft.Observation, 1)
	raft1Observer := raft.NewObserver(raft1Observations, false, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	raft1.RegisterObserver(raft1Observer)
	defer raft1.DeregisterObserver(raft1Observer)

	// Add machine 1.
	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"1": machine1Address,
	})
	rafttest.CheckConfiguration(c, s.Raft, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Nonvoter,
	}})

	select {
	case <-raft1Observations:
		c.Assert(raft1.Leader(), gc.Equals, s.Transport.LocalAddr())
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for leader observation")
	}

	// Add machine 2.
	raft2, _, transport2, _, _ := s.NewRaft(c, "2", &jujuraft.SimpleFSM{})
	connectTransports(s.Transport, transport1, transport2)
	machine2Address := string(transport2.LocalAddr())

	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"1": machine1Address,
		"2": machine2Address,
	})
	rafttest.CheckConfiguration(c, raft2, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}})
}

func (s *WorkerSuite) TestKeepsNonvoterIfAddressChanges(c *gc.C) {
	// Create 2 servers: 0 and 1, where both servers can connect
	// bidirectionally.
	raft1, _, transport1, _, _ := s.NewRaft(c, "1", &jujuraft.SimpleFSM{})
	connectTransports(s.Transport, transport1)

	machine0Address := string(s.Transport.LocalAddr())
	machine1Address := string(transport1.LocalAddr())

	raft1Observations := make(chan raft.Observation, 1)
	raft1Observer := raft.NewObserver(raft1Observations, false, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	raft1.RegisterObserver(raft1Observer)
	defer raft1.DeregisterObserver(raft1Observer)

	// Add machine 1.
	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"1": machine1Address,
	})
	rafttest.CheckConfiguration(c, s.Raft, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Nonvoter,
	}})

	select {
	case <-raft1Observations:
		c.Assert(raft1.Leader(), gc.Equals, s.Transport.LocalAddr())
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for leader observation")
	}

	// Update the non-voting server's address - ensure it doesn't
	// accidentally get promoted to voting at the same time.
	alternateAddress := "testing.invalid:1234"
	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"1": alternateAddress,
	})
	rafttest.CheckConfiguration(c, s.Raft, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "1",
		Address:  raft.ServerAddress(alternateAddress),
		Suffrage: raft.Nonvoter,
	}})
}

func (s *WorkerSuite) TestDemotesLeaderIfRemoved(c *gc.C) {
	// Create 3 servers: 0, 1 and 2, where all servers can connect
	// bidirectionally.
	raft1, _, transport1, _, _ := s.NewRaft(c, "1", &jujuraft.SimpleFSM{})
	_, _, transport2, _, _ := s.NewRaft(c, "2", &jujuraft.SimpleFSM{})
	connectTransports(s.Transport, transport1, transport2)

	machine0Address := string(s.Transport.LocalAddr())
	machine1Address := string(transport1.LocalAddr())
	machine2Address := string(transport2.LocalAddr())

	raft1Observations := make(chan raft.Observation, 1)
	raft1Observer := raft.NewObserver(raft1Observations, false, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	raft1.RegisterObserver(raft1Observer)
	defer raft1.DeregisterObserver(raft1Observer)

	// Add machines 1 and 2.
	s.publishDetails(c, map[string]string{
		"0": machine0Address,
		"1": machine1Address,
		"2": machine2Address,
	})
	rafttest.CheckConfiguration(c, s.Raft, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}})

	select {
	case <-raft1Observations:
		c.Assert(raft1.Leader(), gc.Equals, s.Transport.LocalAddr())
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for leader observation")
	}

	// Remove machine 0. This should prompt the clusterer to demote
	// the leader but not remove it - the new leader after the
	// election will remove it instead.
	s.publishDetails(c, map[string]string{
		"1": machine1Address,
		"2": machine2Address,
	})
	rafttest.CheckConfiguration(c, raft1, []raft.Server{{
		ID:       "0",
		Address:  raft.ServerAddress(machine0Address),
		Suffrage: raft.Nonvoter,
	}, {
		ID:       "1",
		Address:  raft.ServerAddress(machine1Address),
		Suffrage: raft.Voter,
	}, {
		ID:       "2",
		Address:  raft.ServerAddress(machine2Address),
		Suffrage: raft.Voter,
	}})
}

func (s *WorkerSuite) publishDetails(c *gc.C, serverAddrs map[string]string) {
	details := makeDetails(serverAddrs)
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

func makeDetails(serverInfo map[string]string) apiserver.Details {
	servers := make(map[string]apiserver.APIServer)
	for id, address := range serverInfo {
		servers[id] = apiserver.APIServer{
			ID:              id,
			InternalAddress: address,
		}
	}
	return apiserver.Details{Servers: servers}
}
