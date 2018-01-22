// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftclusterer_test

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/pubsub"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

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
	// Add machine-1.
	s.publishDetails(c, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
	})
	s.waitConfiguration(c, []raft.Server{{
		ID:       "machine-0",
		Address:  "machine-0",
		Suffrage: raft.Voter,
	}, {
		ID:       "machine-1",
		Address:  "machine-1",
		Suffrage: raft.Voter,
	}})

	// Remove machine-1.
	s.publishDetails(c, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {ID: "0"},
		},
	})
	s.waitConfiguration(c, []raft.Server{{
		ID:       "machine-0",
		Address:  "machine-0",
		Suffrage: raft.Voter,
	}})
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

func (s *WorkerSuite) waitConfiguration(c *gc.C, expectedServers []raft.Server) {
	var configuration raft.Configuration
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		f := s.Raft.GetConfiguration()
		c.Assert(f.Error(), jc.ErrorIsNil)
		configuration = f.Configuration()
		if len(configuration.Servers) == len(expectedServers) {
			break
		}
	}
	c.Assert(configuration.Servers, jc.SameContents, expectedServers)
}
