// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/retrystrategy"
	"github.com/juju/juju/worker/util"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentApiManifoldConfig: util.AgentApiManifoldConfig{
			AgentName:     "wut",
			APICallerName: "exactly",
		},
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"wut", "exactly"})
}

func (s *ManifoldSuite) TestStartMissingAgent(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentApiManifoldConfig: util.AgentApiManifoldConfig{
			AgentName:     "wut",
			APICallerName: "exactly",
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"wut": dt.StubResource{Error: dependency.ErrMissing},
	})

	w, err := manifold.Start(getResource)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAPI(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentApiManifoldConfig: util.AgentApiManifoldConfig{
			AgentName:     "wut",
			APICallerName: "exactly",
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"wut":     dt.StubResource{Output: &fakeAgent{}},
		"exactly": dt.StubResource{Error: dependency.ErrMissing},
	})

	w, err := manifold.Start(getResource)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartFacadeValueError(c *gc.C) {
	fakeAgent := &fakeAgent{}
	fakeCaller := &fakeCaller{}
	fakeFacade := &fakeFacadeErr{err: errors.New("blop")}
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentApiManifoldConfig: util.AgentApiManifoldConfig{
			AgentName:     "wut",
			APICallerName: "exactly",
		},
		NewFacade: func(apicaller base.APICaller) retrystrategy.Facade {
			c.Assert(apicaller, gc.Equals, fakeCaller)
			return fakeFacade
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"wut":     dt.StubResource{Output: fakeAgent},
		"exactly": dt.StubResource{Output: fakeCaller},
	})

	w, err := manifold.Start(getResource)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "blop")
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartWorkerError(c *gc.C) {
	fakeAgent := &fakeAgent{}
	fakeCaller := &fakeCaller{}
	fakeFacade := &fakeFacade{}
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentApiManifoldConfig: util.AgentApiManifoldConfig{
			AgentName:     "wut",
			APICallerName: "exactly",
		},
		NewFacade: func(apicaller base.APICaller) retrystrategy.Facade {
			c.Assert(apicaller, gc.Equals, fakeCaller)
			return fakeFacade
		},
		NewWorker: func(wc retrystrategy.WorkerConfig) (worker.Worker, error) {
			c.Assert(wc.Facade, gc.Equals, fakeFacade)
			c.Assert(wc.AgentTag, gc.Equals, fakeTag)
			c.Assert(wc.RetryStrategy, gc.Equals, fakeStrategy)
			return nil, errors.New("blam")
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"wut":     dt.StubResource{Output: fakeAgent},
		"exactly": dt.StubResource{Output: fakeCaller},
	})

	w, err := manifold.Start(getResource)
	c.Assert(err, gc.ErrorMatches, "blam")
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	fakeWorker := &fakeWorker{}
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentApiManifoldConfig: util.AgentApiManifoldConfig{
			AgentName:     "wut",
			APICallerName: "exactly",
		},
		NewFacade: func(_ base.APICaller) retrystrategy.Facade {
			return &fakeFacade{}
		},
		NewWorker: func(_ retrystrategy.WorkerConfig) (worker.Worker, error) {
			return fakeWorker, nil
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"wut":     dt.StubResource{Output: &fakeAgent{}},
		"exactly": dt.StubResource{Output: &fakeCaller{}},
	})

	w, err := manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, fakeWorker)
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentApiManifoldConfig: util.AgentApiManifoldConfig{
			AgentName:     "wut",
			APICallerName: "exactly",
		},
		NewFacade: func(_ base.APICaller) retrystrategy.Facade {
			return &fakeFacade{}
		},
		NewWorker: retrystrategy.NewRetryStrategyWorker,
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"wut":     dt.StubResource{Output: &fakeAgent{}},
		"exactly": dt.StubResource{Output: &fakeCaller{}},
	})

	w, err := manifold.Start(getResource)
	s.AddCleanup(func(c *gc.C) { w.Kill() })
	c.Assert(err, jc.ErrorIsNil)

	var out params.RetryStrategy
	err = manifold.Output(w, &out)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, fakeStrategy)
}

func (s *ManifoldSuite) TestOutputBadInput(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentApiManifoldConfig: util.AgentApiManifoldConfig{
			AgentName:     "wut",
			APICallerName: "exactly",
		},
		NewFacade: func(_ base.APICaller) retrystrategy.Facade {
			return &fakeFacade{}
		},
		NewWorker: func(_ retrystrategy.WorkerConfig) (worker.Worker, error) {
			return &fakeWorker{}, nil
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"wut":     dt.StubResource{Output: &fakeAgent{}},
		"exactly": dt.StubResource{Output: &fakeCaller{}},
	})

	w, err := manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)

	var out params.RetryStrategy
	err = manifold.Output(w, &out)
	c.Assert(out, gc.Equals, params.RetryStrategy{})
	c.Assert(err.Error(), gc.Equals, "in should be a *retryStrategyWorker; is *retrystrategy_test.fakeWorker")
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentApiManifoldConfig: util.AgentApiManifoldConfig{
			AgentName:     "wut",
			APICallerName: "exactly",
		},
		NewFacade: func(_ base.APICaller) retrystrategy.Facade {
			return &fakeFacade{}
		},
		NewWorker: retrystrategy.NewRetryStrategyWorker,
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"wut":     dt.StubResource{Output: &fakeAgent{}},
		"exactly": dt.StubResource{Output: &fakeCaller{}},
	})

	w, err := manifold.Start(getResource)
	s.AddCleanup(func(c *gc.C) { w.Kill() })
	c.Assert(err, jc.ErrorIsNil)

	var out interface{}
	err = manifold.Output(w, &out)
	c.Assert(err.Error(), gc.Equals, "out should be a *params.RetryStrategy; is *interface {}")
}

var fakeTag = names.NewUnitTag("whatatag/0")

var fakeStrategy = params.RetryStrategy{
	ShouldRetry:  false,
	MinRetryTime: 2 * time.Second,
}

type fakeAgent struct {
	agent.Agent
}

func (mock *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{}
}

type fakeConfig struct {
	agent.Config
}

func (mock *fakeConfig) Tag() names.Tag {
	return fakeTag
}

type fakeCaller struct {
	base.APICaller
}

type fakeFacade struct {
	retrystrategy.Facade
}

func (mock *fakeFacade) RetryStrategy(agentTag names.Tag) (params.RetryStrategy, error) {
	return fakeStrategy, nil
}

func (mock *fakeFacade) WatchRetryStrategy(agentTag names.Tag) (watcher.NotifyWatcher, error) {
	return newStubWatcher(), nil
}

type fakeFacadeErr struct {
	retrystrategy.Facade
	err error
}

func (mock *fakeFacadeErr) RetryStrategy(agentTag names.Tag) (params.RetryStrategy, error) {
	return params.RetryStrategy{}, mock.err
}

type fakeWorker struct {
	worker.Worker
}
