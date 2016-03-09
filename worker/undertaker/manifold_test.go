// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/undertaker"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (*ManifoldSuite) TestInputs(c *gc.C) {
	manifold := undertaker.Manifold(namesConfig())
	c.Check(manifold.Inputs, jc.DeepEquals, []string{
		"api-caller", "environ", "clock",
	})
}

func (*ManifoldSuite) TestOutput(c *gc.C) {
	manifold := undertaker.Manifold(namesConfig())
	c.Check(manifold.Output, gc.IsNil)
}

func (*ManifoldSuite) TestAPICallerMissing(c *gc.C) {
	resources := resourcesMissing("api-caller")
	manifold := undertaker.Manifold(namesConfig())

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestEnvironMissing(c *gc.C) {
	resources := resourcesMissing("environ")
	manifold := undertaker.Manifold(namesConfig())

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestClockMissing(c *gc.C) {
	resources := resourcesMissing("clock")
	manifold := undertaker.Manifold(namesConfig())

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestNewFacadeError(c *gc.C) {
	resources := resourcesMissing()
	config := namesConfig()
	config.NewFacade = func(apiCaller base.APICaller) (undertaker.Facade, error) {
		checkResource(c, apiCaller, resources, "api-caller")
		return nil, errors.New("blort")
	}
	manifold := undertaker.Manifold(config)

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(err, gc.ErrorMatches, "blort")
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestNewWorkerError(c *gc.C) {
	resources := resourcesMissing()
	expectFacade := &fakeFacade{}
	config := namesConfig()
	config.NewFacade = func(_ base.APICaller) (undertaker.Facade, error) {
		return expectFacade, nil
	}
	config.NewWorker = func(cfg undertaker.Config) (worker.Worker, error) {
		c.Check(cfg.Facade, gc.Equals, expectFacade)
		checkResource(c, cfg.Environ, resources, "environ")
		checkResource(c, cfg.Clock, resources, "clock")
		return nil, errors.New("lhiis")
	}
	manifold := undertaker.Manifold(config)

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(err, gc.ErrorMatches, "lhiis")
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestNewWorkerSuccess(c *gc.C) {
	expectWorker := &fakeWorker{}
	config := namesConfig()
	config.NewFacade = func(_ base.APICaller) (undertaker.Facade, error) {
		return &fakeFacade{}, nil
	}
	config.NewWorker = func(_ undertaker.Config) (worker.Worker, error) {
		return expectWorker, nil
	}
	manifold := undertaker.Manifold(config)
	resources := resourcesMissing()

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, expectWorker)
}

func namesConfig() undertaker.ManifoldConfig {
	return undertaker.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		ClockName:     "clock",
	}
}

func resourcesMissing(missing ...string) dt.StubResources {
	resources := dt.StubResources{
		"api-caller": dt.StubResource{Output: &fakeAPICaller{}},
		"environ":    dt.StubResource{Output: &fakeEnviron{}},
		"clock":      dt.StubResource{Output: &fakeClock{}},
	}
	for _, name := range missing {
		resources[name] = dt.StubResource{Error: dependency.ErrMissing}
	}
	return resources
}

func checkResource(c *gc.C, actual interface{}, resources dt.StubResources, name string) {
	c.Check(actual, gc.Equals, resources[name].Output)
}

type fakeAPICaller struct {
	base.APICaller
}

type fakeEnviron struct {
	environs.Environ
}

type fakeClock struct {
	clock.Clock
}

type fakeFacade struct {
	undertaker.Facade
}

type fakeWorker struct {
	worker.Worker
}
