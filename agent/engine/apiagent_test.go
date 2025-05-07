// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
)

type AgentAPIManifoldSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = tc.Suite(&AgentAPIManifoldSuite{})

func (s *AgentAPIManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testhelpers.Stub{}
	s.worker = &dummyWorker{}
	s.manifold = engine.AgentAPIManifold(engine.AgentAPIManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "api-caller-name",
	}, s.newWorker)
}

func (s *AgentAPIManifoldSuite) newWorker(_ context.Context, a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	s.AddCall("newWorker", a, apiCaller)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *AgentAPIManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold.Inputs, tc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *AgentAPIManifoldSuite) TestOutput(c *tc.C) {
	c.Check(s.manifold.Output, tc.IsNil)
}

func (s *AgentAPIManifoldSuite) TestStartAgentMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *AgentAPIManifoldSuite) TestStartAPIConnMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *AgentAPIManifoldSuite) TestStartFailure(c *tc.C) {
	expectAgent := &dummyAgent{}
	expectAPICaller := &dummyAPICaller{}
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name":      expectAgent,
		"api-caller-name": expectAPICaller,
	})
	s.SetErrors(errors.New("some error"))

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "some error")
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent, expectAPICaller},
	}})
}

func (s *AgentAPIManifoldSuite) TestStartSuccess(c *tc.C) {
	expectAgent := &dummyAgent{}
	expectAPICaller := &dummyAPICaller{}
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name":      expectAgent,
		"api-caller-name": expectAPICaller,
	})

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker, tc.Equals, s.worker)
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent, expectAPICaller},
	}})
}

type dummyAgent struct {
	agent.Agent
}
