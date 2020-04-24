// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

type AgentAPIManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = gc.Suite(&AgentAPIManifoldSuite{})

func (s *AgentAPIManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.worker = &dummyWorker{}
	s.manifold = engine.AgentAPIManifold(engine.AgentAPIManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "api-caller-name",
	}, s.newWorker)
}

func (s *AgentAPIManifoldSuite) newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	s.AddCall("newWorker", a, apiCaller)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *AgentAPIManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *AgentAPIManifoldSuite) TestOutput(c *gc.C) {
	c.Check(s.manifold.Output, gc.IsNil)
}

func (s *AgentAPIManifoldSuite) TestStartAgentMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentAPIManifoldSuite) TestStartAPIConnMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentAPIManifoldSuite) TestStartFailure(c *gc.C) {
	expectAgent := &dummyAgent{}
	expectAPICaller := &dummyAPICaller{}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      expectAgent,
		"api-caller-name": expectAPICaller,
	})
	s.SetErrors(errors.New("some error"))

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "some error")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent, expectAPICaller},
	}})
}

func (s *AgentAPIManifoldSuite) TestStartSuccess(c *gc.C) {
	expectAgent := &dummyAgent{}
	expectAPICaller := &dummyAPICaller{}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      expectAgent,
		"api-caller-name": expectAPICaller,
	})

	worker, err := s.manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.worker)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent, expectAPICaller},
	}})
}

type dummyAPICaller struct {
	base.APICaller
}

type dummyAgent struct {
	agent.Agent
}

type dummyWorker struct {
	worker.Worker
}
