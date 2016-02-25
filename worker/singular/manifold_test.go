// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
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
		AgentName:     "jenny",
	})
	expectInputs := []string{"harriet", "kim", "jenny"}
	c.Check(manifold.Inputs, jc.DeepEquals, expectInputs)
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{})
	var out dependency.Flag
	err := manifold.Output(&fakeWorker{}, &out)
	c.Check(err, gc.ErrorMatches, `expected in to be a \*FlagWorker, got a .*`)
	c.Check(out, gc.IsNil)
}

func (s *ManifoldSuite) TestOutputBadResult(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{})
	fix := newFixture(c)
	fix.Run(c, func(flag *singular.FlagWorker, _ *coretesting.Clock, _ func()) {
		var out interface{}
		err := manifold.Output(flag, &out)
		c.Check(err, gc.ErrorMatches, `expected out to be a \*dependency.Flag, got a .*`)
		c.Check(out, gc.IsNil)
	})
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{})
	fix := newFixture(c)
	fix.Run(c, func(flag *singular.FlagWorker, _ *coretesting.Clock, _ func()) {
		var out dependency.Flag
		err := manifold.Output(flag, &out)
		c.Check(err, jc.ErrorIsNil)
		c.Check(out, gc.Equals, flag)
	})
}

func (s *ManifoldSuite) TestStartMissingClock(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		AgentName:     "agent",
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"clock": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := manifold.Start(getResource)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAgent(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		AgentName:     "agent",
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"clock":      dt.StubResource{Output: &fakeClock{}},
		"api-caller": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := manifold.Start(getResource)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAPICaller(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		AgentName:     "agent",
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"clock":      dt.StubResource{Output: &fakeClock{}},
		"api-caller": dt.StubResource{Output: &fakeAPICaller{}},
		"agent":      dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := manifold.Start(getResource)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartWrongAgent(c *gc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		AgentName:     "agent",
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"clock":      dt.StubResource{Output: &fakeClock{}},
		"api-caller": dt.StubResource{Output: &fakeAPICaller{}},
		"agent":      dt.StubResource{Output: &mockAgent{wrongKind: true}},
	})

	worker, err := manifold.Start(getResource)
	c.Check(err, gc.ErrorMatches, "singular flag expected a machine agent")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartNewFacadeError(c *gc.C) {
	expectAPICaller := &fakeAPICaller{}
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		AgentName:     "agent",
		NewFacade: func(apiCaller base.APICaller, tag names.MachineTag) (singular.Facade, error) {
			c.Check(apiCaller, gc.Equals, expectAPICaller)
			c.Check(tag.String(), gc.Equals, "machine-123")
			return nil, errors.New("grark plop")
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"clock":      dt.StubResource{Output: &fakeClock{}},
		"api-caller": dt.StubResource{Output: expectAPICaller},
		"agent":      dt.StubResource{Output: &mockAgent{}},
	})

	worker, err := manifold.Start(getResource)
	c.Check(err, gc.ErrorMatches, "grark plop")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartNewWorkerError(c *gc.C) {
	expectFacade := &fakeFacade{}
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		AgentName:     "agent",
		Duration:      time.Minute,
		NewFacade: func(_ base.APICaller, _ names.MachineTag) (singular.Facade, error) {
			return expectFacade, nil
		},
		NewWorker: func(config singular.FlagConfig) (worker.Worker, error) {
			c.Check(config.Facade, gc.Equals, expectFacade)
			err := config.Validate()
			c.Check(err, jc.ErrorIsNil)
			return nil, errors.New("blomp tik")
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"clock":      dt.StubResource{Output: &fakeClock{}},
		"api-caller": dt.StubResource{Output: &fakeAPICaller{}},
		"agent":      dt.StubResource{Output: &mockAgent{}},
	})

	worker, err := manifold.Start(getResource)
	c.Check(err, gc.ErrorMatches, "blomp tik")
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	expectWorker := &fakeWorker{}
	manifold := singular.Manifold(singular.ManifoldConfig{
		ClockName:     "clock",
		APICallerName: "api-caller",
		AgentName:     "agent",
		NewFacade: func(_ base.APICaller, _ names.MachineTag) (singular.Facade, error) {
			return &fakeFacade{}, nil
		},
		NewWorker: func(_ singular.FlagConfig) (worker.Worker, error) {
			return expectWorker, nil
		},
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"clock":      dt.StubResource{Output: &fakeClock{}},
		"api-caller": dt.StubResource{Output: &fakeAPICaller{}},
		"agent":      dt.StubResource{Output: &mockAgent{}},
	})

	worker, err := manifold.Start(getResource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, expectWorker)
}
