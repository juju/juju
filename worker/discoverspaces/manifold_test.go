// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/discoverspaces"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (*ManifoldSuite) TestInputsWithUnlocker(c *gc.C) {
	config := namesConfig()
	manifold := discoverspaces.Manifold(config)
	c.Check(manifold.Inputs, jc.SameContents, []string{
		"api-caller", "environ", "unlocker",
	})
}

func (*ManifoldSuite) TestInputsWithoutUnlocker(c *gc.C) {
	config := namesConfig()
	config.UnlockerName = ""
	manifold := discoverspaces.Manifold(config)
	c.Check(manifold.Inputs, jc.SameContents, []string{
		"api-caller", "environ",
	})
}

func (*ManifoldSuite) TestOutput(c *gc.C) {
	manifold := discoverspaces.Manifold(namesConfig())
	c.Check(manifold.Output, gc.IsNil)
}

func (*ManifoldSuite) TestAPICallerMissing(c *gc.C) {
	resources := resourcesMissing("api-caller")
	manifold := discoverspaces.Manifold(namesConfig())

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestEnvironMissing(c *gc.C) {
	resources := resourcesMissing("environ")
	manifold := discoverspaces.Manifold(namesConfig())

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestUnlockerMissing(c *gc.C) {
	resources := resourcesMissing("unlocker")
	manifold := discoverspaces.Manifold(namesConfig())

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestNewFacadeError(c *gc.C) {
	resources := resourcesMissing()
	config := namesConfig()
	config.NewFacade = func(apiCaller base.APICaller) (discoverspaces.Facade, error) {
		checkResource(c, apiCaller, resources, "api-caller")
		return nil, errors.New("blort")
	}
	manifold := discoverspaces.Manifold(config)

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(err, gc.ErrorMatches, "blort")
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestNewWorkerError(c *gc.C) {
	resources := resourcesMissing()
	expectFacade := &fakeFacade{}
	config := namesConfig()
	config.NewFacade = func(_ base.APICaller) (discoverspaces.Facade, error) {
		return expectFacade, nil
	}
	config.NewWorker = func(cfg discoverspaces.Config) (worker.Worker, error) {
		c.Check(cfg.Facade, gc.Equals, expectFacade)
		checkResource(c, cfg.Environ, resources, "environ")
		c.Check(cfg.NewName, gc.NotNil) // uncomparable
		checkResource(c, cfg.Unlocker, resources, "unlocker")
		return nil, errors.New("lhiis")
	}
	manifold := discoverspaces.Manifold(config)

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(err, gc.ErrorMatches, "lhiis")
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestNewWorkerNoUnlocker(c *gc.C) {
	resources := resourcesMissing()
	config := namesConfig()
	config.UnlockerName = ""
	config.NewFacade = func(_ base.APICaller) (discoverspaces.Facade, error) {
		return &fakeFacade{}, nil
	}
	config.NewWorker = func(cfg discoverspaces.Config) (worker.Worker, error) {
		c.Check(cfg.Unlocker, gc.IsNil)
		return nil, errors.New("mrrg")
	}
	manifold := discoverspaces.Manifold(config)

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(err, gc.ErrorMatches, "mrrg")
	c.Check(worker, gc.IsNil)
}

func (*ManifoldSuite) TestNewWorkerSuccess(c *gc.C) {
	expectWorker := &fakeWorker{}
	config := namesConfig()
	config.NewFacade = func(_ base.APICaller) (discoverspaces.Facade, error) {
		return &fakeFacade{}, nil
	}
	config.NewWorker = func(_ discoverspaces.Config) (worker.Worker, error) {
		return expectWorker, nil
	}
	manifold := discoverspaces.Manifold(config)
	resources := resourcesMissing()

	worker, err := manifold.Start(dt.StubGetResource(resources))
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, expectWorker)
}

func namesConfig() discoverspaces.ManifoldConfig {
	return discoverspaces.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		UnlockerName:  "unlocker",
	}
}

func resourcesMissing(missing ...string) dt.StubResources {
	resources := dt.StubResources{
		"api-caller": dt.StubResource{Output: &fakeAPICaller{}},
		"environ":    dt.StubResource{Output: &fakeEnviron{}},
		"unlocker":   dt.StubResource{Output: &fakeUnlocker{}},
	}
	for _, name := range missing {
		resources[name] = dt.StubResource{Error: dependency.ErrMissing}
	}
	return resources
}

func checkResource(c *gc.C, actual interface{}, resources dt.StubResources, name string) {
	c.Check(actual, gc.Equals, resources[name].Output)
}
