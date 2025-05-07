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
	"github.com/juju/juju/internal/testhelpers"
)

type AgentManifoldSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = tc.Suite(&AgentManifoldSuite{})

func (s *AgentManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testhelpers.Stub{}
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

func (s *AgentManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold.Inputs, tc.DeepEquals, []string{"agent-name"})
}

func (s *AgentManifoldSuite) TestOutput(c *tc.C) {
	c.Check(s.manifold.Output, tc.IsNil)
}

func (s *AgentManifoldSuite) TestStartAgentMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *AgentManifoldSuite) TestStartFailure(c *tc.C) {
	expectAgent := &dummyAgent{}
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name": expectAgent,
	})
	s.SetErrors(errors.New("some error"))

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "some error")
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent},
	}})
}

func (s *AgentManifoldSuite) TestStartSuccess(c *tc.C) {
	expectAgent := &dummyAgent{}
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name": expectAgent,
	})

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker, tc.Equals, s.worker)
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent},
	}})
}
