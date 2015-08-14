// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filter_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/uniter/filter"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	filter      filter.Filter
	manifold    dependency.Manifold
	agent       agent.Agent
	apiCaller   base.APICaller
	getResource dependency.GetResourceFunc
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.filter = filter.DummyFilter()
	s.PatchValue(filter.NewWorker, func(agent agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
		s.AddCall("newWorker", agent, apiCaller)
		if err := s.NextErr(); err != nil {
			return nil, err
		}
		return s.filter, nil
	})
	s.manifold = filter.Manifold(filter.ManifoldConfig{
		AgentName:     "agent-name",
		ApiCallerName: "api-caller-name",
	})
	s.agent = &dummyAgent{}
	s.apiCaller = &dummyApiCaller{}
	s.getResource = dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: s.agent},
		"api-caller-name": dt.StubResource{Output: s.apiCaller},
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *ManifoldSuite) TestStartMissingApiCaller(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller-name": dt.StubResource{Error: dependency.ErrMissing},
		"agent-name":      dt.StubResource{Output: s.agent},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
	s.CheckCalls(c, nil)
}

func (s *ManifoldSuite) TestStartMissingAgent(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Error: dependency.ErrMissing},
		"api-caller-name": dt.StubResource{Output: s.apiCaller},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
	s.CheckCalls(c, nil)
}

func (s *ManifoldSuite) TestStartError(c *gc.C) {
	s.SetErrors(errors.New("no filter for you"))
	worker, err := s.manifold.Start(s.getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "no filter for you")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{s.agent, s.apiCaller},
	}})
}

func (s *ManifoldSuite) setupWorkerTest(c *gc.C) worker.Worker {
	worker, err := s.manifold.Start(s.getResource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.filter)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{s.agent, s.apiCaller},
	}})
	return worker
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	s.setupWorkerTest(c)
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	worker := s.setupWorkerTest(c)
	var filter filter.Filter
	err := s.manifold.Output(worker, &filter)
	c.Check(err, jc.ErrorIsNil)
	c.Check(filter, gc.Equals, s.filter)
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	var filter filter.Filter
	err := s.manifold.Output(&dummyWorker{}, &filter)
	c.Check(err.Error(), gc.Equals, "expected *filter.filter->*filter.Filter; got *filter_test.dummyWorker->*filter.Filter")
	c.Check(filter, gc.IsNil)
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	worker := s.setupWorkerTest(c)
	var filter interface{}
	err := s.manifold.Output(worker, &filter)
	c.Check(err.Error(), gc.Equals, "expected *filter.filter->*filter.Filter; got *filter.filter->*interface {}")
	c.Check(filter, gc.IsNil)
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
