// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/singular"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	config singular.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.config = singular.ManifoldConfig{
		Clock:         testclock.NewClock(time.Now()),
		APICallerName: "api-caller",
		Duration:      time.Minute,
		NewFacade: func(base.APICaller, names.Tag, names.Tag) (singular.Facade, error) {
			return nil, errors.NotImplementedf("NewFacade")
		},
		NewWorker: func(config singular.FlagConfig) (worker.Worker, error) {
			return nil, errors.NotImplementedf("NewWorker")
		},
	}
}

func (s *ManifoldSuite) TestValidate(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestValidateMissingClock(c *gc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), gc.Equals, "nil Clock not valid")
}

func (s *ManifoldSuite) TestValidateMissingAPICallerName(c *gc.C) {
	s.config.APICallerName = ""
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), gc.Equals, "missing APICallerName not valid")
}

func (s *ManifoldSuite) TestValidateMissingNewFacade(c *gc.C) {
	s.config.NewFacade = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), gc.Equals, "nil NewFacade not valid")
}

func (s *ManifoldSuite) TestValidateMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), gc.Equals, "nil NewWorker not valid")
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{
		APICallerName: "kim",
	})
	expectInputs := []string{"kim"}
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
		APICallerName: "api-caller",
	})
	context := dt.StubContext(nil, map[string]interface{}{})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.ErrorMatches, `nil Clock not valid`)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAPICaller(c *gc.C) {
	manifold := singular.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartNewFacadeError(c *gc.C) {
	expectAPICaller := &fakeAPICaller{}
	s.config.Claimant = names.NewMachineTag("123")
	s.config.Entity = coretesting.ModelTag
	s.config.NewFacade = func(apiCaller base.APICaller, claimant names.Tag, entity names.Tag) (singular.Facade, error) {
		c.Check(apiCaller, gc.Equals, expectAPICaller)
		c.Check(claimant.String(), gc.Equals, "machine-123")
		c.Check(entity, gc.Equals, coretesting.ModelTag)
		return nil, errors.New("grark plop")
	}
	manifold := singular.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": expectAPICaller,
	})

	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, "grark plop")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartNewWorkerError(c *gc.C) {
	expectFacade := &fakeFacade{}
	s.config.NewFacade = func(base.APICaller, names.Tag, names.Tag) (singular.Facade, error) {
		return expectFacade, nil
	}
	s.config.NewWorker = func(config singular.FlagConfig) (worker.Worker, error) {
		c.Check(config.Facade, gc.Equals, expectFacade)
		err := config.Validate()
		c.Check(err, jc.ErrorIsNil)
		return nil, errors.New("blomp tik")
	}
	manifold := singular.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &fakeAPICaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, "blomp tik")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	var stub testing.Stub
	expectWorker := newStubWorker(&stub)
	s.config.NewFacade = func(base.APICaller, names.Tag, names.Tag) (singular.Facade, error) {
		return &fakeFacade{}, nil
	}
	s.config.NewWorker = func(_ singular.FlagConfig) (worker.Worker, error) {
		return expectWorker, nil
	}
	manifold := singular.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
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
	s.config.NewFacade = func(base.APICaller, names.Tag, names.Tag) (singular.Facade, error) {
		return &fakeFacade{}, nil
	}
	s.config.NewWorker = func(_ singular.FlagConfig) (worker.Worker, error) {
		return errWorker, nil
	}

	manifold := singular.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &fakeAPICaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker.Wait(), gc.Equals, dependency.ErrBounce)
}
