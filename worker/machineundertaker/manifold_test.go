// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/machineundertaker"
)

type manifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (*manifoldSuite) TestMissingCaller(c *gc.C) {
	manifold := makeManifold(nil, nil)
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller": dependency.ErrMissing,
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
		NewWorker: func(machineundertaker.Facade, environs.Environ) (worker.Worker, error) {
			return workerResult, workerError
		},
	})
}

type fakeAPICaller struct {
	base.APICaller
}

func (c *fakeAPICaller) ModelTag() (names.ModelTag, bool) {
	return names.ModelTag{}, false
}

type fakeEnviron struct {
	environs.Environ
}

type fakeWorker struct {
	worker.Worker
	name string
}
