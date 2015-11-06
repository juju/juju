// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionmanifold_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/charmrevision"
	"github.com/juju/juju/worker/charmrevision/charmrevisionmanifold"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestManifold(c *gc.C) {
	manifold := charmrevisionmanifold.Manifold(charmrevisionmanifold.ManifoldConfig{
		APICallerName: "billy",
		ClockName:     "bob",
	})

	c.Check(manifold.Inputs, jc.DeepEquals, []string{"billy", "bob"})
	c.Check(manifold.Start, gc.NotNil)
	c.Check(manifold.Output, gc.IsNil)
}

func (s *ManifoldSuite) TestMissingAPICaller(c *gc.C) {
	manifold := charmrevisionmanifold.Manifold(charmrevisionmanifold.ManifoldConfig{
		APICallerName: "api-caller",
		ClockName:     "clock",
	})

	_, err := manifold.Start(dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Error: dependency.ErrMissing},
		"clock":      dt.StubResource{Output: fakeClock{}},
	}))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingClock(c *gc.C) {
	manifold := charmrevisionmanifold.Manifold(charmrevisionmanifold.ManifoldConfig{
		APICallerName: "api-caller",
		ClockName:     "clock",
	})

	_, err := manifold.Start(dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Output: fakeAPICaller{}},
		"clock":      dt.StubResource{Error: dependency.ErrMissing},
	}))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNewFacadeError(c *gc.C) {
	fakeAPICaller := &fakeAPICaller{}

	stub := testing.Stub{}
	manifold := charmrevisionmanifold.Manifold(charmrevisionmanifold.ManifoldConfig{
		APICallerName: "api-caller",
		ClockName:     "clock",
		NewFacade: func(apiCaller base.APICaller) (charmrevisionmanifold.Facade, error) {
			stub.AddCall("NewFacade", apiCaller)
			return nil, errors.New("blefgh")
		},
	})

	_, err := manifold.Start(dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Output: fakeAPICaller},
		"clock":      dt.StubResource{Output: fakeClock{}},
	}))
	c.Check(err, gc.ErrorMatches, "cannot create facade: blefgh")
	stub.CheckCalls(c, []testing.StubCall{{
		"NewFacade", []interface{}{fakeAPICaller},
	}})
}

func (s *ManifoldSuite) TestNewWorkerError(c *gc.C) {
	fakeClock := &fakeClock{}
	fakeFacade := &fakeFacade{}
	fakeAPICaller := &fakeAPICaller{}

	stub := testing.Stub{}
	manifold := charmrevisionmanifold.Manifold(charmrevisionmanifold.ManifoldConfig{
		APICallerName: "api-caller",
		ClockName:     "clock",
		NewFacade: func(apiCaller base.APICaller) (charmrevisionmanifold.Facade, error) {
			stub.AddCall("NewFacade", apiCaller)
			return fakeFacade, nil
		},
		NewWorker: func(config charmrevision.Config) (worker.Worker, error) {
			stub.AddCall("NewWorker", config)
			return nil, errors.New("snrght")
		},
	})

	_, err := manifold.Start(dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Output: fakeAPICaller},
		"clock":      dt.StubResource{Output: fakeClock},
	}))
	c.Check(err, gc.ErrorMatches, "cannot create worker: snrght")
	stub.CheckCalls(c, []testing.StubCall{{
		"NewFacade", []interface{}{fakeAPICaller},
	}, {
		"NewWorker", []interface{}{charmrevision.Config{
			RevisionUpdater: fakeFacade,
			Clock:           fakeClock,
		}},
	}})
}

func (s *ManifoldSuite) TestSuccess(c *gc.C) {
	fakeClock := &fakeClock{}
	fakeFacade := &fakeFacade{}
	fakeWorker := &fakeWorker{}
	fakeAPICaller := &fakeAPICaller{}

	stub := testing.Stub{}
	manifold := charmrevisionmanifold.Manifold(charmrevisionmanifold.ManifoldConfig{
		APICallerName: "api-caller",
		ClockName:     "clock",
		Period:        10 * time.Minute,
		NewFacade: func(apiCaller base.APICaller) (charmrevisionmanifold.Facade, error) {
			stub.AddCall("NewFacade", apiCaller)
			return fakeFacade, nil
		},
		NewWorker: func(config charmrevision.Config) (worker.Worker, error) {
			stub.AddCall("NewWorker", config)
			return fakeWorker, nil
		},
	})

	w, err := manifold.Start(dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Output: fakeAPICaller},
		"clock":      dt.StubResource{Output: fakeClock},
	}))
	c.Check(w, gc.Equals, fakeWorker)
	c.Check(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []testing.StubCall{{
		"NewFacade", []interface{}{fakeAPICaller},
	}, {
		"NewWorker", []interface{}{charmrevision.Config{
			Period:          10 * time.Minute,
			RevisionUpdater: fakeFacade,
			Clock:           fakeClock,
		}},
	}})
}

type fakeAPICaller struct {
	base.APICaller
}

type fakeClock struct {
	clock.Clock
}

type fakeWorker struct {
	worker.Worker
}

type fakeFacade struct {
	charmrevisionmanifold.Facade
}
