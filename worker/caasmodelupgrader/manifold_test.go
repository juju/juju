// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelupgrader_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/caasmodelupgrader"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/gate"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (*ManifoldSuite) TestInputs(c *gc.C) {
	manifold := caasmodelupgrader.Manifold(caasmodelupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		GateName:      "gate",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"api-caller", "gate"})
}

func (*ManifoldSuite) TestMissingAPICaller(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
		"gate":       struct{ gate.Unlocker }{},
	})
	manifold := caasmodelupgrader.Manifold(caasmodelupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		GateName:      "gate",
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestMissingGateName(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"gate":       dependency.ErrMissing,
	})
	manifold := caasmodelupgrader.Manifold(caasmodelupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		GateName:      "gate",
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestNewFacadeError(c *gc.C) {
	expectAPICaller := struct{ base.APICaller }{}
	expectGate := struct{ gate.Unlocker }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": expectAPICaller,
		"gate":       expectGate,
	})
	manifold := caasmodelupgrader.Manifold(caasmodelupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		GateName:      "gate",
		NewFacade: func(actual base.APICaller) (caasmodelupgrader.Facade, error) {
			c.Check(actual, gc.Equals, expectAPICaller)
			return nil, errors.New("error")
		},
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "error")
}

func (*ManifoldSuite) TestNewWorkerError(c *gc.C) {
	expectFacade := struct{ caasmodelupgrader.Facade }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"gate":       struct{ gate.Unlocker }{},
	})
	manifold := caasmodelupgrader.Manifold(caasmodelupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		GateName:      "gate",
		NewFacade: func(_ base.APICaller) (caasmodelupgrader.Facade, error) {
			return expectFacade, nil
		},
		NewWorker: func(config caasmodelupgrader.Config) (worker.Worker, error) {
			c.Check(config.Facade, gc.Equals, expectFacade)
			return nil, errors.New("error")
		},
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "error")
}
