// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicescaler_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/servicescaler"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	manifold := servicescaler.Manifold(servicescaler.ManifoldConfig{
		APICallerName: "washington the terrible",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"washington the terrible"})
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	manifold := servicescaler.Manifold(servicescaler.ManifoldConfig{})
	c.Check(manifold.Output, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAPICaller(c *gc.C) {
	manifold := servicescaler.Manifold(servicescaler.ManifoldConfig{
		APICallerName: "api-caller",
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := manifold.Start(getResource)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartFacadeError(c *gc.C) {
	expectCaller := &fakeCaller{}
	manifold := servicescaler.Manifold(servicescaler.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade: func(apiCaller base.APICaller) (servicescaler.Facade, error) {
			c.Check(apiCaller, gc.Equals, expectCaller)
			return nil, errors.New("blort")
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Output: expectCaller},
	})

	worker, err := manifold.Start(getResource)
	c.Check(err, gc.ErrorMatches, "blort")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartWorkerError(c *gc.C) {
	expectFacade := &fakeFacade{}
	manifold := servicescaler.Manifold(servicescaler.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade: func(_ base.APICaller) (servicescaler.Facade, error) {
			return expectFacade, nil
		},
		NewWorker: func(config servicescaler.Config) (worker.Worker, error) {
			c.Check(config.Validate(), jc.ErrorIsNil)
			c.Check(config.Facade, gc.Equals, expectFacade)
			return nil, errors.New("splot")
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Output: &fakeCaller{}},
	})

	worker, err := manifold.Start(getResource)
	c.Check(err, gc.ErrorMatches, "splot")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestSuccess(c *gc.C) {
	expectWorker := &fakeWorker{}
	manifold := servicescaler.Manifold(servicescaler.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade: func(_ base.APICaller) (servicescaler.Facade, error) {
			return &fakeFacade{}, nil
		},
		NewWorker: func(_ servicescaler.Config) (worker.Worker, error) {
			return expectWorker, nil
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Output: &fakeCaller{}},
	})

	worker, err := manifold.Start(getResource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, expectWorker)
}

type fakeCaller struct {
	base.APICaller
}

type fakeFacade struct {
	servicescaler.Facade
}

type fakeWorker struct {
	worker.Worker
}
