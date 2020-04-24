// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/credentialvalidator"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (*ManifoldSuite) TestInputs(c *gc.C) {
	manifold := credentialvalidator.Manifold(validManifoldConfig())
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"api-caller"})
}

func (*ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	in := &struct{ worker.Worker }{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, gc.ErrorMatches, "expected in to implement Flag; got a .*")
}

func (*ManifoldSuite) TestFilterNil(c *gc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	err := manifold.Filter(nil)
	c.Check(err, jc.ErrorIsNil)
}

func (*ManifoldSuite) TestFilterErrChanged(c *gc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	err := manifold.Filter(credentialvalidator.ErrValidityChanged)
	c.Check(err, gc.Equals, dependency.ErrBounce)
}

func (*ManifoldSuite) TestFilterErrModelCredentialChanged(c *gc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	err := manifold.Filter(credentialvalidator.ErrModelCredentialChanged)
	c.Check(err, gc.Equals, dependency.ErrBounce)
}

func (*ManifoldSuite) TestFilterOther(c *gc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	expect := errors.New("whatever")
	actual := manifold.Filter(expect)
	c.Check(actual, gc.Equals, expect)
}

func (*ManifoldSuite) TestStartMissingAPICallerName(c *gc.C) {
	config := validManifoldConfig()
	config.APICallerName = ""
	checkManifoldNotValid(c, config, "empty APICallerName not valid")
}

func (*ManifoldSuite) TestStartMissingNewFacade(c *gc.C) {
	config := validManifoldConfig()
	config.NewFacade = nil
	checkManifoldNotValid(c, config, "nil NewFacade not valid")
}

func (*ManifoldSuite) TestStartMissingNewWorker(c *gc.C) {
	config := validManifoldConfig()
	config.NewWorker = nil
	checkManifoldNotValid(c, config, "nil NewWorker not valid")
}

func (*ManifoldSuite) TestStartMissingLogger(c *gc.C) {
	config := validManifoldConfig()
	config.Logger = nil
	checkManifoldNotValid(c, config, "nil Logger not valid")
}

func (*ManifoldSuite) TestStartMissingAPICaller(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
	})
	manifold := credentialvalidator.Manifold(validManifoldConfig())

	w, err := manifold.Start(context)
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestStartNewFacadeError(c *gc.C) {
	expectCaller := &stubCaller{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": expectCaller,
	})
	config := validManifoldConfig()
	config.NewFacade = func(caller base.APICaller) (credentialvalidator.Facade, error) {
		c.Check(caller, gc.Equals, expectCaller)
		return nil, errors.New("bort")
	}
	manifold := credentialvalidator.Manifold(config)

	w, err := manifold.Start(context)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "bort")
}

func (*ManifoldSuite) TestStartNewWorkerError(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &stubCaller{},
	})
	expectFacade := &struct{ credentialvalidator.Facade }{}
	config := validManifoldConfig()
	config.NewFacade = func(base.APICaller) (credentialvalidator.Facade, error) {
		return expectFacade, nil
	}
	config.NewWorker = func(workerConfig credentialvalidator.Config) (worker.Worker, error) {
		c.Check(workerConfig.Facade, gc.Equals, expectFacade)
		return nil, errors.New("snerk")
	}
	manifold := credentialvalidator.Manifold(config)

	w, err := manifold.Start(context)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "snerk")
}

func (*ManifoldSuite) TestStartSuccess(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &stubCaller{},
	})
	expectWorker := &struct{ worker.Worker }{}
	config := validManifoldConfig()
	config.NewFacade = func(base.APICaller) (credentialvalidator.Facade, error) {
		return &struct{ credentialvalidator.Facade }{}, nil
	}
	config.NewWorker = func(credentialvalidator.Config) (worker.Worker, error) {
		return expectWorker, nil
	}
	manifold := credentialvalidator.Manifold(config)

	w, err := manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(w, gc.Equals, expectWorker)
}
