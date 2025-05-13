// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	coreleadership "github.com/juju/juju/core/leadership"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/leadership"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub
	manifold dependency.Manifold
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testhelpers.Stub{}
	s.manifold = leadership.Manifold(leadership.ManifoldConfig{
		AgentName:           "agent-name",
		APICallerName:       "api-caller-name",
		Clock:               clock.WallClock,
		LeadershipGuarantee: 123456 * time.Millisecond,
	})
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold.Inputs, tc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *ManifoldSuite) TestStartClockMissing(c *tc.C) {
	manifold := leadership.Manifold(leadership.ManifoldConfig{})
	getter := dt.StubGetter(nil)
	worker, err := manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err.Error(), tc.Equals, "missing Clock not valid")
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestStartAgentMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name":      dependency.ErrMissing,
		"api-caller-name": &dummyAPICaller{},
	})

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartAPICallerMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartError(c *tc.C) {
	dummyAgent := &dummyAgent{}
	dummyAPICaller := &dummyAPICaller{}
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name":      dummyAgent,
		"api-caller-name": dummyAPICaller,
	})
	s.PatchValue(&leadership.NewManifoldWorker, func(a agent.Agent, apiCaller base.APICaller, clock clock.Clock, guarantee time.Duration) (worker.Worker, error) {
		s.AddCall("newManifoldWorker", a, apiCaller, clock, guarantee)
		return nil, errors.New("blammo")
	})

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "blammo")
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "newManifoldWorker",
		Args:     []interface{}{dummyAgent, dummyAPICaller, clock.WallClock, 123456 * time.Millisecond},
	}})
}

func (s *ManifoldSuite) TestStartSuccess(c *tc.C) {
	dummyAgent := &dummyAgent{}
	dummyAPICaller := &dummyAPICaller{}
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name":      dummyAgent,
		"api-caller-name": dummyAPICaller,
	})
	dummyWorker := &dummyWorker{}
	s.PatchValue(&leadership.NewManifoldWorker, func(a agent.Agent, apiCaller base.APICaller, clock clock.Clock, guarantee time.Duration) (worker.Worker, error) {
		s.AddCall("newManifoldWorker", a, apiCaller, clock, guarantee)
		return dummyWorker, nil
	})

	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker, tc.Equals, dummyWorker)
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "newManifoldWorker",
		Args:     []interface{}{dummyAgent, dummyAPICaller, clock.WallClock, 123456 * time.Millisecond},
	}})
}

func (s *ManifoldSuite) TestOutputBadTarget(c *tc.C) {
	var target interface{}
	err := s.manifold.Output(&leadership.Tracker{}, &target)
	c.Check(target, tc.IsNil)
	c.Check(err.Error(), tc.Equals, "expected *leadership.[Change]Tracker output; got *interface {}")
}

func (s *ManifoldSuite) TestOutputBadWorker(c *tc.C) {
	var target coreleadership.TrackerWorker
	err := s.manifold.Output(&dummyWorker{}, &target)
	c.Check(target, tc.IsNil)
	c.Check(err.Error(), tc.Equals, "expected *Tracker input; got *leadership_test.dummyWorker")
}

func (s *ManifoldSuite) TestOutputSuccess(c *tc.C) {
	source := &leadership.Tracker{}
	var target coreleadership.Tracker
	err := s.manifold.Output(source, &target)
	c.Check(err, tc.ErrorIsNil)
	c.Check(target, tc.Equals, source)
}

type dummyAgent struct {
	agent.Agent
}

type dummyAPICaller struct {
	base.APICaller
}

type dummyWorker struct {
	worker.Worker
}
