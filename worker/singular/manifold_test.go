// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/singular"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "harriet",
		APICallerName: "kim",
	})
	expectInputs := []string{"harriet", "kim"}
	c.Check(manifold.Inputs, jc.DeepEquals, expectInputs)
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{})
	var out engine.Flag
	err := manifold.Output(&fakeWorker{}, &out)
	c.Check(err, gc.ErrorMatches, `expected in to implement Flag; got a .*`)
	c.Check(out, gc.IsNil)
}

func (s *ManifoldSuite) TestOutputBadResult(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{})
	fix := newFixture(c)
	fix.Run(c, func(flag *singular.FlagWorker, _ *testclock.Clock, _ func()) {
		var out interface{}
		err := manifold.Output(flag, &out)
		c.Check(err, gc.ErrorMatches, `expected out to be a \*Flag; got a .*`)
		c.Check(out, gc.IsNil)
	})
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{})
	fix := newFixture(c)
	fix.Run(c, func(flag *singular.FlagWorker, _ *testclock.Clock, _ func()) {
		var out engine.Flag
		err := manifold.Output(flag, &out)
		c.Check(err, jc.ErrorIsNil)
		c.Check(out, gc.Equals, flag)
	})
}

func (s *ManifoldSuite) TestStartMissingClock(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"clock": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAPICaller(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"clock":      &fakeClock{},
		"api-caller": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartNewFacadeError(c *gc.C) {
	expectAPICaller := &fakeAPICaller{}
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		Claimant:      names.NewMachineTag("123"),
		Entity:        coretesting.ModelTag,
		NewFacade: func(apiCaller base.APICaller, claimant names.MachineTag, entity names.Tag) (singular.Facade, error) {
			c.Check(apiCaller, gc.Equals, expectAPICaller)
			c.Check(claimant.String(), gc.Equals, "machine-123")
			c.Check(entity, gc.Equals, coretesting.ModelTag)
			return nil, errors.New("grark plop")
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"clock":      &fakeClock{},
		"api-caller": expectAPICaller,
	})

	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, "grark plop")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartNewWorkerError(c *gc.C) {
	expectFacade := &fakeFacade{}
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		Duration:      time.Minute,
		NewFacade: func(base.APICaller, names.MachineTag, names.Tag) (singular.Facade, error) {
			return expectFacade, nil
		},
		NewWorker: func(config singular.FlagConfig) (worker.Worker, error) {
			c.Check(config.Facade, gc.Equals, expectFacade)
			err := config.Validate()
			c.Check(err, jc.ErrorIsNil)
			return nil, errors.New("blomp tik")
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"clock":      &fakeClock{},
		"api-caller": &fakeAPICaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, "blomp tik")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	var stub testing.Stub
	expectWorker := newStubWorker(&stub)
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		NewFacade: func(base.APICaller, names.MachineTag, names.Tag) (singular.Facade, error) {
			return &fakeFacade{}, nil
		},
		NewWorker: func(_ singular.FlagConfig) (worker.Worker, error) {
			return expectWorker, nil
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"clock":      &fakeClock{},
		"api-caller": &fakeAPICaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)

	var out engine.Flag
	err = manifold.Output(worker, &out)
	c.Check(err, jc.ErrorIsNil)
	c.Check(out.Check(), jc.IsTrue)

	c.Check(worker.Wait(), jc.ErrorIsNil)
	stub.CheckCallNames(c, "Check", "Wait")
}

func (s *ManifoldSuite) TestWorkerBouncesOnRefresh(c *gc.C) {
	var stub testing.Stub
	stub.SetErrors(singular.ErrRefresh)
	errWorker := newStubWorker(&stub)

	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		NewFacade: func(base.APICaller, names.MachineTag, names.Tag) (singular.Facade, error) {
			return &fakeFacade{}, nil
		},
		NewWorker: func(_ singular.FlagConfig) (worker.Worker, error) {
			return errWorker, nil
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"clock":      &fakeClock{},
		"api-caller": &fakeAPICaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker.Wait(), gc.Equals, dependency.ErrBounce)
}
