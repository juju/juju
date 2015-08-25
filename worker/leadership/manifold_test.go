// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
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
		LeadershipGuarantee: 123456 * time.Millisecond,
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *ManifoldSuite) TestStartAgentMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Error: dependency.ErrMissing},
		"api-caller-name": dt.StubResource{Output: &dummyApiCaller{}},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartApiCallerMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: &dummyAgent{}},
		"api-caller-name": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartError(c *gc.C) {
	dummyAgent := &dummyAgent{}
	dummyApiCaller := &dummyApiCaller{}
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: dummyAgent},
		"api-caller-name": dt.StubResource{Output: dummyApiCaller},
	})
	s.PatchValue(leadership.NewManifoldWorker, func(a agent.Agent, apiCaller base.APICaller, guarantee time.Duration) (worker.Worker, error) {
		s.AddCall("newManifoldWorker", a, apiCaller, guarantee)
		return nil, errors.New("blammo")
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "blammo")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newManifoldWorker",
		Args:     []interface{}{dummyAgent, dummyApiCaller, 123456 * time.Millisecond},
	}})
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	dummyAgent := &dummyAgent{}
	dummyApiCaller := &dummyApiCaller{}
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: dummyAgent},
		"api-caller-name": dt.StubResource{Output: dummyApiCaller},
	})
	dummyWorker := &dummyWorker{}
	s.PatchValue(leadership.NewManifoldWorker, func(a agent.Agent, apiCaller base.APICaller, guarantee time.Duration) (worker.Worker, error) {
		s.AddCall("newManifoldWorker", a, apiCaller, guarantee)
		return dummyWorker, nil
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, dummyWorker)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newManifoldWorker",
		Args:     []interface{}{dummyAgent, dummyApiCaller, 123456 * time.Millisecond},
	}})
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	var target interface{}
	err := s.manifold.Output(leadership.DummyTrackerWorker(), &target)
	c.Check(target, gc.IsNil)
	c.Check(err.Error(), gc.Equals, "expected *leadership.tracker->*leadership.Tracker; got *leadership.tracker->*interface {}")
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	var target leadership.Tracker
	err := s.manifold.Output(&dummyWorker{}, &target)
	c.Check(target, gc.IsNil)
	c.Check(err.Error(), gc.Equals, "expected *leadership.tracker->*leadership.Tracker; got *leadership_test.dummyWorker->*leadership.Tracker")
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	source := leadership.DummyTrackerWorker()
	var target leadership.Tracker
	err := s.manifold.Output(source, &target)
	c.Check(err, jc.ErrorIsNil)
	c.Check(target, gc.Equals, source)
}

type dummyAgent struct {
	agent.Agent
}

type dummyApiCaller struct {
	base.APICaller
}

type dummyWorker struct {
	worker.Worker
}
