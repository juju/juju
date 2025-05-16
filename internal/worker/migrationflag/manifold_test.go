// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/migrationflag"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *stdtesting.T) { tc.Run(t, &ManifoldSuite{}) }
func (*ManifoldSuite) TestInputs(c *tc.C) {
	manifold := migrationflag.Manifold(validManifoldConfig())
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"api-caller"})
}

func (*ManifoldSuite) TestOutputBadWorker(c *tc.C) {
	manifold := migrationflag.Manifold(migrationflag.ManifoldConfig{})
	in := &struct{ worker.Worker }{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorMatches, "expected in to implement Flag; got a .*")
}

func (*ManifoldSuite) TestOutputBadTarget(c *tc.C) {
	manifold := migrationflag.Manifold(migrationflag.ManifoldConfig{})
	in := &migrationflag.Worker{}
	var out bool
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorMatches, "expected out to be a \\*Flag; got a .*")
}

func (*ManifoldSuite) TestOutputBadInput(c *tc.C) {
	manifold := migrationflag.Manifold(migrationflag.ManifoldConfig{})
	in := &migrationflag.Worker{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorIsNil)
	c.Check(out, tc.Equals, in)
}

func (*ManifoldSuite) TestFilterNil(c *tc.C) {
	manifold := migrationflag.Manifold(migrationflag.ManifoldConfig{})
	err := manifold.Filter(nil)
	c.Check(err, tc.ErrorIsNil)
}

func (*ManifoldSuite) TestFilterErrChanged(c *tc.C) {
	manifold := migrationflag.Manifold(migrationflag.ManifoldConfig{})
	err := manifold.Filter(migrationflag.ErrChanged)
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*ManifoldSuite) TestFilterOther(c *tc.C) {
	manifold := migrationflag.Manifold(migrationflag.ManifoldConfig{})
	expect := errors.New("whatever")
	actual := manifold.Filter(expect)
	c.Check(actual, tc.Equals, expect)
}

func (*ManifoldSuite) TestStartMissingAPICallerName(c *tc.C) {
	config := validManifoldConfig()
	config.APICallerName = ""
	checkManifoldNotValid(c, config, "empty APICallerName not valid")
}

func (*ManifoldSuite) TestStartMissingCheck(c *tc.C) {
	config := validManifoldConfig()
	config.Check = nil
	checkManifoldNotValid(c, config, "nil Check not valid")
}

func (*ManifoldSuite) TestStartMissingNewFacade(c *tc.C) {
	config := validManifoldConfig()
	config.NewFacade = nil
	checkManifoldNotValid(c, config, "nil NewFacade not valid")
}

func (*ManifoldSuite) TestStartMissingNewWorker(c *tc.C) {
	config := validManifoldConfig()
	config.NewWorker = nil
	checkManifoldNotValid(c, config, "nil NewWorker not valid")
}

func (*ManifoldSuite) TestStartMissingAPICaller(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller": dependency.ErrMissing,
	})
	manifold := migrationflag.Manifold(validManifoldConfig())

	worker, err := manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestStartNewFacadeError(c *tc.C) {
	expectCaller := &stubCaller{}
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller": expectCaller,
	})
	config := validManifoldConfig()
	config.NewFacade = func(caller base.APICaller) (migrationflag.Facade, error) {
		c.Check(caller, tc.Equals, expectCaller)
		return nil, errors.New("bort")
	}
	manifold := migrationflag.Manifold(config)

	worker, err := manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "bort")
}

func (*ManifoldSuite) TestStartNewWorkerError(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller": &stubCaller{},
	})
	expectFacade := &struct{ migrationflag.Facade }{}
	config := validManifoldConfig()
	config.NewFacade = func(base.APICaller) (migrationflag.Facade, error) {
		return expectFacade, nil
	}
	config.NewWorker = func(ctx context.Context, workerConfig migrationflag.Config) (worker.Worker, error) {
		c.Check(workerConfig.Facade, tc.Equals, expectFacade)
		c.Check(workerConfig.Model, tc.Equals, validUUID)
		c.Check(workerConfig.Check, tc.NotNil) // uncomparable
		return nil, errors.New("snerk")
	}
	manifold := migrationflag.Manifold(config)

	worker, err := manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "snerk")
}

func (*ManifoldSuite) TestStartSuccess(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller": &stubCaller{},
	})
	expectWorker := &struct{ worker.Worker }{}
	config := validManifoldConfig()
	config.NewFacade = func(base.APICaller) (migrationflag.Facade, error) {
		return &struct{ migrationflag.Facade }{}, nil
	}
	config.NewWorker = func(context.Context, migrationflag.Config) (worker.Worker, error) {
		return expectWorker, nil
	}
	manifold := migrationflag.Manifold(config)

	worker, err := manifold.Start(c.Context(), getter)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker, tc.Equals, expectWorker)
}
