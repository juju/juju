// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/util"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type AgentApiManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = gc.Suite(&AgentApiManifoldSuite{})

func (s *AgentApiManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.worker = &dummyWorker{}
	s.manifold = util.AgentApiManifold(util.AgentApiManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "api-caller-name",
	}, s.newWorker)
}

func (s *AgentApiManifoldSuite) newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	s.AddCall("newWorker", a, apiCaller)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *AgentApiManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *AgentApiManifoldSuite) TestOutput(c *gc.C) {
	c.Check(s.manifold.Output, gc.IsNil)
}

func (s *AgentApiManifoldSuite) TestStartAgentMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentApiManifoldSuite) TestStartApiConnMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentApiManifoldSuite) TestStartFailure(c *gc.C) {
	expectAgent := &dummyAgent{}
	expectApiCaller := &dummyApiCaller{}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      expectAgent,
		"api-caller-name": expectApiCaller,
	})
	s.SetErrors(errors.New("some error"))

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "some error")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent, expectApiCaller},
	}})
}

func (s *AgentApiManifoldSuite) TestStartSuccess(c *gc.C) {
	expectAgent := &dummyAgent{}
	expectApiCaller := &dummyApiCaller{}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      expectAgent,
		"api-caller-name": expectApiCaller,
	})

	worker, err := s.manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.worker)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent, expectApiCaller},
	}})
}

type dummyApiCaller struct {
	base.APICaller
}

type dummyAgent struct {
	agent.Agent
}

type dummyWorker struct {
	worker.Worker
}
