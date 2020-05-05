// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/machineactions"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	context    dependency.Context
	fakeAgent  agent.Agent
	fakeCaller base.APICaller
	fakeFacade machineactions.Facade
	fakeWorker worker.Worker
	newFacade  func(machineactions.Facade) func(base.APICaller) machineactions.Facade
	newWorker  func(worker.Worker, error) func(machineactions.WorkerConfig) (worker.Worker, error)
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.fakeAgent = &fakeAgent{tag: fakeTag}
	s.fakeCaller = &fakeCaller{}

	s.context = dt.StubContext(nil, map[string]interface{}{
		"wut":     s.fakeAgent,
		"exactly": s.fakeCaller,
	})

	s.newFacade = func(facade machineactions.Facade) func(base.APICaller) machineactions.Facade {
		s.fakeFacade = facade
		return func(apiCaller base.APICaller) machineactions.Facade {
			c.Assert(apiCaller, gc.Equals, s.fakeCaller)
			return facade
		}
	}
	s.newWorker = func(w worker.Worker, err error) func(machineactions.WorkerConfig) (worker.Worker, error) {
		s.fakeWorker = w
		return func(wc machineactions.WorkerConfig) (worker.Worker, error) {
			c.Assert(wc.Facade, gc.Equals, s.fakeFacade)
			c.Assert(wc.MachineTag, gc.Equals, fakeTag)
			c.Assert(wc.HandleAction, gc.Equals, fakeHandleAction)
			return w, err
		}
	}
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	manifold := machineactions.Manifold(machineactions.ManifoldConfig{
		AgentName:     "wut",
		APICallerName: "exactly",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"wut", "exactly"})
}

func (s *ManifoldSuite) TestStartMissingAgent(c *gc.C) {
	manifold := machineactions.Manifold(machineactions.ManifoldConfig{
		AgentName:     "wut",
		APICallerName: "exactly",
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"wut": dependency.ErrMissing,
	})

	w, err := manifold.Start(context)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAPI(c *gc.C) {
	manifold := machineactions.Manifold(machineactions.ManifoldConfig{
		AgentName:     "wut",
		APICallerName: "exactly",
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"wut":     &fakeAgent{},
		"exactly": dependency.ErrMissing,
	})

	w, err := manifold.Start(context)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartWorkerError(c *gc.C) {
	manifold := machineactions.Manifold(machineactions.ManifoldConfig{
		AgentName:     "wut",
		APICallerName: "exactly",
		NewFacade:     s.newFacade(&fakeFacade{}),
		NewWorker:     s.newWorker(nil, errors.New("blam")),
	})

	w, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "blam")
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	fakeWorker := &fakeWorker{}
	manifold := machineactions.Manifold(machineactions.ManifoldConfig{
		AgentName:     "wut",
		APICallerName: "exactly",
		NewFacade:     s.newFacade(&fakeFacade{}),
		NewWorker:     s.newWorker(fakeWorker, nil),
	})

	w, err := manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, fakeWorker)
}

func (s *ManifoldSuite) TestInvalidTag(c *gc.C) {
	fakeWorker := &fakeWorker{}
	manifold := machineactions.Manifold(machineactions.ManifoldConfig{
		AgentName:     "wut",
		APICallerName: "exactly",
		NewFacade:     s.newFacade(&fakeFacade{}),
		NewWorker:     s.newWorker(fakeWorker, nil),
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"wut":     &fakeAgent{tag: fakeTagErr},
		"exactly": s.fakeCaller,
	})

	w, err := manifold.Start(context)
	c.Assert(err, gc.ErrorMatches, "this manifold can only be used inside a machine")
	c.Assert(w, gc.IsNil)
}

var fakeTag = names.NewMachineTag("4")
var fakeTagErr = names.NewUnitTag("whatatag/0")

type fakeAgent struct {
	agent.Agent
	tag names.Tag
}

func (mock *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{tag: mock.tag}
}

type fakeConfig struct {
	agent.Config
	tag names.Tag
}

func (mock *fakeConfig) Tag() names.Tag {
	return mock.tag
}

type fakeCaller struct {
	base.APICaller
}

type fakeFacade struct {
	machineactions.Facade
}

type fakeWorker struct {
	worker.Worker
}

var fakeHandleAction = func(name string, params map[string]interface{}) (results map[string]interface{}, err error) {
	return nil, nil
}
