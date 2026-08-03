// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/credentialvalidator"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (*ManifoldSuite) TestInputs(c *tc.C) {
	manifold := credentialvalidator.Manifold(validManifoldConfig(c))
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"api-caller"})
}

func (*ManifoldSuite) TestOutputBadWorker(c *tc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	in := &struct{ worker.Worker }{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorMatches, "expected in to implement Flag; got a .*")
}

func (*ManifoldSuite) TestFilterNil(c *tc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	err := manifold.Filter(nil)
	c.Check(err, tc.ErrorIsNil)
}

func (*ManifoldSuite) TestFilterErrChanged(c *tc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	err := manifold.Filter(credentialvalidator.ErrValidityChanged)
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*ManifoldSuite) TestFilterErrModelCredentialChanged(c *tc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	err := manifold.Filter(credentialvalidator.ErrModelCredentialChanged)
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*ManifoldSuite) TestFilterOther(c *tc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	expect := errors.New("whatever")
	actual := manifold.Filter(expect)
	c.Check(actual, tc.Equals, expect)
}

func (*ManifoldSuite) TestStartMissingAPICallerName(c *tc.C) {
	config := validManifoldConfig(c)
	config.APICallerName = ""
	checkManifoldNotValid(c, config, "empty APICallerName not valid")
}

func (*ManifoldSuite) TestStartMissingNewFacade(c *tc.C) {
	config := validManifoldConfig(c)
	config.NewFacade = nil
	checkManifoldNotValid(c, config, "nil NewFacade not valid")
}

func (*ManifoldSuite) TestStartMissingNewWorker(c *tc.C) {
	config := validManifoldConfig(c)
	config.NewWorker = nil
	checkManifoldNotValid(c, config, "nil NewWorker not valid")
}

func (*ManifoldSuite) TestStartMissingLogger(c *tc.C) {
	config := validManifoldConfig(c)
	config.Logger = nil
	checkManifoldNotValid(c, config, "nil Logger not valid")
}

func (*ManifoldSuite) TestStartMissingAPICaller(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"api-caller": dependency.ErrMissing,
	})
	manifold := credentialvalidator.Manifold(validManifoldConfig(c))

	w, err := manifold.Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestStartNewFacadeError(c *tc.C) {
	expectCaller := &stubCaller{}
	getter := dt.StubGetter(map[string]any{
		"api-caller": expectCaller,
	})
	config := validManifoldConfig(c)
	config.NewFacade = func(caller base.APICaller) (credentialvalidator.Facade, error) {
		c.Check(caller, tc.Equals, expectCaller)
		return nil, errors.New("bort")
	}
	manifold := credentialvalidator.Manifold(config)

	w, err := manifold.Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "bort")
}

func (*ManifoldSuite) TestStartNewWorkerError(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"api-caller": &stubCaller{},
	})
	expectFacade := &struct{ credentialvalidator.Facade }{}
	config := validManifoldConfig(c)
	config.NewFacade = func(base.APICaller) (credentialvalidator.Facade, error) {
		return expectFacade, nil
	}
	config.NewWorker = func(_ context.Context, workerConfig credentialvalidator.Config) (worker.Worker, error) {
		c.Check(workerConfig.Facade, tc.Equals, expectFacade)
		return nil, errors.New("snerk")
	}
	manifold := credentialvalidator.Manifold(config)

	w, err := manifold.Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "snerk")
}

func (*ManifoldSuite) TestStartSuccess(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"api-caller": &stubCaller{},
	})
	expectWorker := &struct{ worker.Worker }{}
	config := validManifoldConfig(c)
	config.NewFacade = func(base.APICaller) (credentialvalidator.Facade, error) {
		return &struct{ credentialvalidator.Facade }{}, nil
	}
	config.NewWorker = func(context.Context, credentialvalidator.Config) (worker.Worker, error) {
		return expectWorker, nil
	}
	manifold := credentialvalidator.Manifold(config)

	w, err := manifold.Start(c.Context(), getter)
	c.Check(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, expectWorker)
}

func (*ManifoldSuite) TestModelManifoldInputs(c *tc.C) {
	manifold := credentialvalidator.ModelManifold(validModelManifoldConfig(c))
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"domain-services"})
}

func (*ManifoldSuite) TestModelManifoldOutputBadWorker(c *tc.C) {
	manifold := credentialvalidator.ModelManifold(credentialvalidator.ModelManifoldConfig{})
	in := &struct{ worker.Worker }{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorMatches, "expected in to implement Flag; got a .*")
}

func (*ManifoldSuite) TestModelManifoldFilterNil(c *tc.C) {
	manifold := credentialvalidator.ModelManifold(credentialvalidator.ModelManifoldConfig{})
	err := manifold.Filter(nil)
	c.Check(err, tc.ErrorIsNil)
}

func (*ManifoldSuite) TestModelManifoldFilterErrChanged(c *tc.C) {
	manifold := credentialvalidator.ModelManifold(credentialvalidator.ModelManifoldConfig{})
	err := manifold.Filter(credentialvalidator.ErrValidityChanged)
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*ManifoldSuite) TestModelManifoldFilterErrModelCredentialChanged(c *tc.C) {
	manifold := credentialvalidator.ModelManifold(credentialvalidator.ModelManifoldConfig{})
	err := manifold.Filter(credentialvalidator.ErrModelCredentialChanged)
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*ManifoldSuite) TestModelManifoldFilterOther(c *tc.C) {
	manifold := credentialvalidator.ModelManifold(credentialvalidator.ModelManifoldConfig{})
	expect := errors.New("whatever")
	actual := manifold.Filter(expect)
	c.Check(actual, tc.Equals, expect)
}

func (*ManifoldSuite) TestModelManifoldStartMissingDomainServicesName(c *tc.C) {
	config := validModelManifoldConfig(c)
	config.DomainServicesName = ""
	checkModelManifoldNotValid(c, config, "empty DomainServicesName not valid")
}

func (*ManifoldSuite) TestModelManifoldStartMissingModelUUID(c *tc.C) {
	config := validModelManifoldConfig(c)
	config.ModelUUID = ""
	checkModelManifoldNotValid(c, config, "empty ModelUUID not valid")
}

func (*ManifoldSuite) TestModelManifoldStartMissingNewWorker(c *tc.C) {
	config := validModelManifoldConfig(c)
	config.NewWorker = nil
	checkModelManifoldNotValid(c, config, "nil NewWorker not valid")
}

func (*ManifoldSuite) TestModelManifoldStartMissingLogger(c *tc.C) {
	config := validModelManifoldConfig(c)
	config.Logger = nil
	checkModelManifoldNotValid(c, config, "nil Logger not valid")
}

func (*ManifoldSuite) TestModelManifoldStartMissingDomainServices(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": dependency.ErrMissing,
	})
	manifold := credentialvalidator.ModelManifold(validModelManifoldConfig(c))

	w, err := manifold.Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestModelManifoldStartNewWorkerError(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": &stubDomainServices{},
	})
	config := validModelManifoldConfig(c)
	config.NewWorker = func(_ context.Context, workerConfig credentialvalidator.Config) (worker.Worker, error) {
		c.Check(workerConfig.Facade, tc.NotNil)
		return nil, errors.New("snerk")
	}
	manifold := credentialvalidator.ModelManifold(config)

	w, err := manifold.Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "snerk")
}

func (*ManifoldSuite) TestModelManifoldStartSuccess(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": &stubDomainServices{},
	})
	expectWorker := &struct{ worker.Worker }{}
	config := validModelManifoldConfig(c)
	config.NewWorker = func(context.Context, credentialvalidator.Config) (worker.Worker, error) {
		return expectWorker, nil
	}
	manifold := credentialvalidator.ModelManifold(config)

	w, err := manifold.Start(c.Context(), getter)
	c.Check(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, expectWorker)
}

func (*ManifoldSuite) TestModelManifoldStartDoesNotRequestAPICaller(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": &stubDomainServices{},
	})
	config := validModelManifoldConfig(c)
	config.NewWorker = func(_ context.Context, workerConfig credentialvalidator.Config) (worker.Worker, error) {
		c.Check(workerConfig.Facade, tc.NotNil)
		return &struct{ worker.Worker }{}, nil
	}
	manifold := credentialvalidator.ModelManifold(config)

	_, err := manifold.Start(c.Context(), getter)
	c.Check(err, tc.ErrorIsNil)
}
