// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftforwarder_test

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"

	"github.com/juju/juju/worker/raft/raftforwarder"
)

type manifoldSuite struct {
	testing.IsolationSuite

	context  dependency.Context
	manifold dependency.Manifold
	raft     *raft.Raft
	hub      *pubsub.StructuredHub
	logger   loggo.Logger
	worker   worker.Worker
	stub     testing.Stub
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.raft = &raft.Raft{}
	s.hub = &pubsub.StructuredHub{}

	type mockWorker struct {
		worker.Worker
	}
	s.worker = &mockWorker{}
	s.logger = loggo.GetLogger("juju.worker.raftforwarder_test")

	s.context = s.newContext(nil)
	s.manifold = raftforwarder.Manifold(raftforwarder.ManifoldConfig{
		RaftName:       "raft",
		CentralHubName: "hub",
		RequestTopic:   "test.request",
		Logger:         &s.logger,
		NewWorker:      s.newWorker,
	})
}

func (s *manifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"raft": s.raft,
		"hub":  s.hub,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *manifoldSuite) newWorker(config raftforwarder.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

var expectedInputs = []string{
	"raft", "hub",
}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	s.startWorkerClean(c)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, raftforwarder.Config{})
	config := args[0].(raftforwarder.Config)

	c.Assert(config, jc.DeepEquals, raftforwarder.Config{
		Raft:   s.raft,
		Hub:    s.hub,
		Logger: &s.logger,
		Topic:  "test.request",
	})
}

func (s *manifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, s.worker)
	return w
}
