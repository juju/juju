// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/applicationscaler"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{
		APICallerName: "washington the terrible",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"washington the terrible"})
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{})
	c.Check(manifold.Output, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAPICaller(c *gc.C) {
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{
		APICallerName: "api-caller",
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartFacadeError(c *gc.C) {
	expectCaller := &fakeCaller{}
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade: func(apiCaller base.APICaller) (applicationscaler.Facade, error) {
			c.Check(apiCaller, gc.Equals, expectCaller)
			return nil, errors.New("blort")
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": expectCaller,
	})

	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, "blort")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartWorkerError(c *gc.C) {
	expectFacade := &fakeFacade{}
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade: func(_ base.APICaller) (applicationscaler.Facade, error) {
			return expectFacade, nil
		},
		NewWorker: func(config applicationscaler.Config) (worker.Worker, error) {
			c.Check(config.Validate(), jc.ErrorIsNil)
			c.Check(config.Facade, gc.Equals, expectFacade)
			return nil, errors.New("splot")
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &fakeCaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, "splot")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestSuccess(c *gc.C) {
	expectWorker := &fakeWorker{}
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade: func(_ base.APICaller) (applicationscaler.Facade, error) {
			return &fakeFacade{}, nil
		},
		NewWorker: func(_ applicationscaler.Config) (worker.Worker, error) {
			return expectWorker, nil
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &fakeCaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, expectWorker)
}

type fakeCaller struct {
	base.APICaller
}

type fakeFacade struct {
	applicationscaler.Facade
}

type fakeWorker struct {
	worker.Worker
}
