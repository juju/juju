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
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/util"
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
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentApiManifoldSuite) TestStartApiConnMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: &dummyAgent{}},
		"api-caller-name": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentApiManifoldSuite) TestStartFailure(c *gc.C) {
	expectAgent := &dummyAgent{}
	expectApiCaller := &dummyApiCaller{}
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: expectAgent},
		"api-caller-name": dt.StubResource{Output: expectApiCaller},
	})
	s.SetErrors(errors.New("some error"))

	worker, err := s.manifold.Start(getResource)
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
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: expectAgent},
		"api-caller-name": dt.StubResource{Output: expectApiCaller},
	})

	worker, err := s.manifold.Start(getResource)
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
