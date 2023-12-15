// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environupgrader_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/environupgrader"
	"github.com/juju/juju/internal/worker/gate"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (*ManifoldSuite) TestInputs(c *gc.C) {
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "boris",
		EnvironName:   "nikolayevich",
		GateName:      "yeltsin",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"boris", "nikolayevich", "yeltsin"})
}

func (*ManifoldSuite) TestMissingAPICaller(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
		"environ":    struct{ environs.Environ }{},
		"gate":       struct{ gate.Unlocker }{},
	})
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestMissingGateName(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"environ":    struct{ environs.Environ }{},
		"gate":       dependency.ErrMissing,
	})
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestNewFacadeError(c *gc.C) {
	expectAPICaller := struct{ base.APICaller }{}
	expectEnviron := struct{ environs.Environ }{}
	expectGate := struct{ gate.Unlocker }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": expectAPICaller,
		"environ":    expectEnviron,
		"gate":       expectGate,
	})
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
		NewFacade: func(actual base.APICaller) (environupgrader.Facade, error) {
			c.Check(actual, gc.Equals, expectAPICaller)
			return nil, errors.New("splort")
		},
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "splort")
}

func (*ManifoldSuite) TestNewWorkerError(c *gc.C) {
	expectFacade := struct{ environupgrader.Facade }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"environ":    struct{ environs.Environ }{},
		"gate":       struct{ gate.Unlocker }{},
	})
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
		NewFacade: func(_ base.APICaller) (environupgrader.Facade, error) {
			return expectFacade, nil
		},
		NewWorker: func(config environupgrader.Config) (worker.Worker, error) {
			c.Check(config.Facade, gc.Equals, expectFacade)
			return nil, errors.New("boof")
		},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boof")
}

func (*ManifoldSuite) TestNewWorkerSuccessWithEnviron(c *gc.C) {
	expectWorker := &struct{ worker.Worker }{}
	expectEnviron := struct{ environs.Environ }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"environ":    expectEnviron,
		"gate":       struct{ gate.Unlocker }{},
	})
	var newWorkerConfig environupgrader.Config
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
		NewFacade: func(_ base.APICaller) (environupgrader.Facade, error) {
			return struct{ environupgrader.Facade }{}, nil
		},
		NewWorker: func(config environupgrader.Config) (worker.Worker, error) {
			newWorkerConfig = config
			return expectWorker, nil
		},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.Equals, expectWorker)
	c.Check(err, jc.ErrorIsNil)
	c.Check(newWorkerConfig.Environ, gc.Equals, expectEnviron)
}

func (*ManifoldSuite) TestNewWorkerSuccessWithoutEnviron(c *gc.C) {
	expectWorker := &struct{ worker.Worker }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"environ":    dependency.ErrMissing,
		"gate":       struct{ gate.Unlocker }{},
	})
	var newWorkerConfig environupgrader.Config
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
		NewFacade: func(_ base.APICaller) (environupgrader.Facade, error) {
			return struct{ environupgrader.Facade }{}, nil
		},
		NewWorker: func(config environupgrader.Config) (worker.Worker, error) {
			newWorkerConfig = config
			return expectWorker, nil
		},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.Equals, expectWorker)
	c.Check(err, jc.ErrorIsNil)
	c.Check(newWorkerConfig.Environ, gc.IsNil)
}

func (*ManifoldSuite) TestFilterNil(c *gc.C) {
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{})
	err := manifold.Filter(nil)
	c.Check(err, jc.ErrorIsNil)
}

func (*ManifoldSuite) TestFilterErrModelRemoved(c *gc.C) {
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{})
	err := manifold.Filter(environupgrader.ErrModelRemoved)
	c.Check(err, gc.Equals, dependency.ErrUninstall)
}
