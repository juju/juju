// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type AgentManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = gc.Suite(&AgentManifoldSuite{})

func (s *AgentManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.worker = &dummyWorker{}
	s.manifold = engine.AgentManifold(engine.AgentManifoldConfig{
		AgentName: "agent-name",
	}, s.newWorker)
}

func (s *AgentManifoldSuite) newWorker(a agent.Agent) (worker.Worker, error) {
	s.AddCall("newWorker", a)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *AgentManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name"})
}

func (s *AgentManifoldSuite) TestOutput(c *gc.C) {
	c.Check(s.manifold.Output, gc.IsNil)
}

func (s *AgentManifoldSuite) TestStartAgentMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentManifoldSuite) TestStartFailure(c *gc.C) {
	expectAgent := &dummyAgent{}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name": expectAgent,
	})
	s.SetErrors(errors.New("some error"))

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "some error")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent},
	}})
}

func (s *AgentManifoldSuite) TestStartSuccess(c *gc.C) {
	expectAgent := &dummyAgent{}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name": expectAgent,
	})

	worker, err := s.manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.worker)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent},
	}})
}
