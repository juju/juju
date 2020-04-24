// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftbackstop_test

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/raft/raftbackstop"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold dependency.Manifold
	context  dependency.Context
	raft     *raft.Raft
	logStore raft.LogStore
	hub      *pubsub.StructuredHub
	agent    *mockAgent
	logger   loggo.Logger
	worker   worker.Worker
	stub     testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.raft = &raft.Raft{}
	type mockLogStore struct {
		raft.LogStore
	}
	s.logStore = &mockLogStore{}
	s.hub = &pubsub.StructuredHub{}
	s.stub.ResetCalls()

	type mockWorker struct {
		worker.Worker
	}
	s.worker = &mockWorker{}
	s.agent = &mockAgent{
		conf: mockAgentConfig{tag: names.NewMachineTag("3")},
	}
	s.logger = loggo.GetLogger("raftbackstop_test")

	s.context = s.newContext(nil)
	s.manifold = raftbackstop.Manifold(raftbackstop.ManifoldConfig{
		RaftName:       "raft",
		CentralHubName: "central-hub",
		AgentName:      "agent",
		NewWorker:      s.newWorker,
		Logger:         s.logger,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"raft":        []interface{}{s.raft, s.logStore},
		"central-hub": s.hub,
		"agent":       s.agent,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config raftbackstop.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

var expectedInputs = []string{
	"raft", "central-hub", "agent",
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

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, raftbackstop.Config{})
	config := args[0].(raftbackstop.Config)

	c.Assert(config, jc.DeepEquals, raftbackstop.Config{
		Raft:     s.raft,
		Hub:      s.hub,
		LogStore: s.logStore,
		LocalID:  "3",
		Logger:   s.logger,
	})
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, s.worker)
	return w
}

type mockAgent struct {
	agent.Agent
	conf mockAgentConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockAgentConfig struct {
	agent.Config
	tag names.Tag
}

func (c *mockAgentConfig) Tag() names.Tag {
	return c.tag
}
