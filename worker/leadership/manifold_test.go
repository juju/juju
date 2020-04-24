// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	coreleadership "github.com/juju/juju/core/leadership"
	"github.com/juju/juju/worker/leadership"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold dependency.Manifold
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.manifold = leadership.Manifold(leadership.ManifoldConfig{
		AgentName:           "agent-name",
		APICallerName:       "api-caller-name",
		Clock:               clock.WallClock,
		LeadershipGuarantee: 123456 * time.Millisecond,
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *ManifoldSuite) TestStartClockMissing(c *gc.C) {
	manifold := leadership.Manifold(leadership.ManifoldConfig{})
	context := dt.StubContext(nil, nil)
	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err.Error(), gc.Equals, "missing Clock not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *ManifoldSuite) TestStartAgentMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      dependency.ErrMissing,
		"api-caller-name": &dummyAPICaller{},
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartAPICallerMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartError(c *gc.C) {
	dummyAgent := &dummyAgent{}
	dummyAPICaller := &dummyAPICaller{}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      dummyAgent,
		"api-caller-name": dummyAPICaller,
	})
	s.PatchValue(&leadership.NewManifoldWorker, func(a agent.Agent, apiCaller base.APICaller, clock clock.Clock, guarantee time.Duration) (worker.Worker, error) {
		s.AddCall("newManifoldWorker", a, apiCaller, clock, guarantee)
		return nil, errors.New("blammo")
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "blammo")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newManifoldWorker",
		Args:     []interface{}{dummyAgent, dummyAPICaller, clock.WallClock, 123456 * time.Millisecond},
	}})
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	dummyAgent := &dummyAgent{}
	dummyAPICaller := &dummyAPICaller{}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      dummyAgent,
		"api-caller-name": dummyAPICaller,
	})
	dummyWorker := &dummyWorker{}
	s.PatchValue(&leadership.NewManifoldWorker, func(a agent.Agent, apiCaller base.APICaller, clock clock.Clock, guarantee time.Duration) (worker.Worker, error) {
		s.AddCall("newManifoldWorker", a, apiCaller, clock, guarantee)
		return dummyWorker, nil
	})

	worker, err := s.manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, dummyWorker)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newManifoldWorker",
		Args:     []interface{}{dummyAgent, dummyAPICaller, clock.WallClock, 123456 * time.Millisecond},
	}})
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	var target interface{}
	err := s.manifold.Output(&leadership.Tracker{}, &target)
	c.Check(target, gc.IsNil)
	c.Check(err.Error(), gc.Equals, "expected *leadership.Tracker output; got *interface {}")
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	var target coreleadership.TrackerWorker
	err := s.manifold.Output(&dummyWorker{}, &target)
	c.Check(target, gc.IsNil)
	c.Check(err.Error(), gc.Equals, "expected *Tracker input; got *leadership_test.dummyWorker")
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	source := &leadership.Tracker{}
	var target coreleadership.TrackerWorker
	err := s.manifold.Output(source, &target)
	c.Check(err, jc.ErrorIsNil)
	c.Check(target, gc.Equals, source)
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
