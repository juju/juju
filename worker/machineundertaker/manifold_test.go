// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/machineundertaker"
)

type manifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (*manifoldSuite) TestMissingCaller(c *gc.C) {
	manifold := makeManifold(nil, nil)
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller":  dependency.ErrMissing,
		"the-environ": &fakeEnviron{},
	}))
	c.Assert(result, gc.IsNil)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*manifoldSuite) TestMissingEnviron(c *gc.C) {
	manifold := makeManifold(nil, nil)
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller":  &fakeAPICaller{},
		"the-environ": dependency.ErrMissing,
	}))
	c.Assert(result, gc.IsNil)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*manifoldSuite) TestAPIError(c *gc.C) {
	manifold := makeManifold(nil, nil)
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller":  &fakeAPICaller{},
		"the-environ": &fakeEnviron{},
	}))
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "machine undertaker client requires a model API connection")
}

func (*manifoldSuite) TestWorkerError(c *gc.C) {
	manifold := makeManifold(nil, errors.New("boglodite"))
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller":  apitesting.APICallerFunc(nil),
		"the-environ": &fakeEnviron{},
	}))
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "boglodite")
}

func (*manifoldSuite) TestSuccess(c *gc.C) {
	w := fakeWorker{name: "Boris"}
	manifold := makeManifold(&w, nil)
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller":  apitesting.APICallerFunc(nil),
		"the-environ": &fakeEnviron{},
	}))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, &w)
}

func makeManifold(workerResult worker.Worker, workerError error) dependency.Manifold {
	return machineundertaker.Manifold(machineundertaker.ManifoldConfig{
		APICallerName: "the-caller",
		EnvironName:   "the-environ",
		Logger:        loggo.GetLogger("test"),
		NewWorker: func(machineundertaker.Facade, environs.Environ, common.CredentialAPI, machineundertaker.Logger) (worker.Worker, error) {
			return workerResult, workerError
		},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) {
			return &fakeCredentialAPI{}, nil
		},
	})
}

type fakeAPICaller struct {
	base.APICaller
}

func (c *fakeAPICaller) ModelTag() (names.ModelTag, bool) {
	return names.ModelTag{}, false
}

type fakeWorker struct {
	worker.Worker
	name string
}
