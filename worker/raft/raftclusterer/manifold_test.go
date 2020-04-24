// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftclusterer_test

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/raft/raftclusterer"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold dependency.Manifold
	context  dependency.Context
	raft     *raft.Raft
	hub      *pubsub.StructuredHub
	worker   worker.Worker
	stub     testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.raft = &raft.Raft{}
	s.hub = &pubsub.StructuredHub{}
	s.stub.ResetCalls()

	type mockWorker struct {
		worker.Worker
	}
	s.worker = &mockWorker{}

	s.context = s.newContext(nil)
	s.manifold = raftclusterer.Manifold(raftclusterer.ManifoldConfig{
		RaftName:       "raft",
		CentralHubName: "central-hub",
		NewWorker:      s.newWorker,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"raft":        s.raft,
		"central-hub": s.hub,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config raftclusterer.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

var expectedInputs = []string{
	"raft", "central-hub",
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
	c.Assert(args[0], gc.FitsTypeOf, raftclusterer.Config{})
	config := args[0].(raftclusterer.Config)

	c.Assert(config, jc.DeepEquals, raftclusterer.Config{
		Raft: s.raft,
		Hub:  s.hub,
	})
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, s.worker)
	return w
}
