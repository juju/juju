// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/undertaker"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite
	modelType string
}

type CAASManifoldSuite struct {
	manifoldSuite
}

type IAASManifoldSuite struct {
	manifoldSuite
}

var (
	_ = tc.Suite(&IAASManifoldSuite{})
	_ = tc.Suite(&CAASManifoldSuite{})
)

func (s *CAASManifoldSuite) SetUpTest(c *tc.C) {
	s.modelType = "caas"
}

func (s *IAASManifoldSuite) SetUpTest(c *tc.C) {
	s.modelType = "iaas"
}

func (s *manifoldSuite) namesConfig(c *tc.C) undertaker.ManifoldConfig {
	return undertaker.ManifoldConfig{
		APICallerName: "api-caller",
		Logger:        loggertesting.WrapCheckLog(c),
		NewCloudDestroyerFunc: func(ctx context.Context, params environs.OpenParams, _ environs.CredentialInvalidator) (environs.CloudDestroyer, error) {
			return &fakeEnviron{}, nil
		},
	}
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	manifold := undertaker.Manifold(s.namesConfig(c))
	c.Check(manifold.Inputs, tc.DeepEquals, []string{
		"api-caller",
	})
}

func (s *manifoldSuite) TestOutput(c *tc.C) {
	manifold := undertaker.Manifold(s.namesConfig(c))
	c.Check(manifold.Output, tc.IsNil)
}

func (s *manifoldSuite) TestAPICallerMissing(c *tc.C) {
	resources := resourcesMissing("api-caller")
	manifold := undertaker.Manifold(s.namesConfig(c))

	worker, err := manifold.Start(context.Background(), resources.Getter())
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
	c.Check(worker, tc.IsNil)
}

func (s *manifoldSuite) TestNewFacadeError(c *tc.C) {
	resources := resourcesMissing()
	config := s.namesConfig(c)
	config.NewFacade = func(apiCaller base.APICaller) (undertaker.Facade, error) {
		checkResource(c, apiCaller, resources, "api-caller")
		return nil, errors.New("blort")
	}
	manifold := undertaker.Manifold(config)

	worker, err := manifold.Start(context.Background(), resources.Getter())
	c.Check(err, tc.ErrorMatches, "blort")
	c.Check(worker, tc.IsNil)
}

func (s *manifoldSuite) TestNewWorkerError(c *tc.C) {
	resources := resourcesMissing()
	expectFacade := &fakeFacade{}
	config := s.namesConfig(c)
	config.NewFacade = func(_ base.APICaller) (undertaker.Facade, error) {
		return expectFacade, nil
	}
	config.NewWorker = func(cfg undertaker.Config) (worker.Worker, error) {
		c.Check(cfg.Facade, tc.Equals, expectFacade)
		return nil, errors.New("lhiis")
	}
	manifold := undertaker.Manifold(config)

	worker, err := manifold.Start(context.Background(), resources.Getter())
	c.Check(err, tc.ErrorMatches, "lhiis")
	c.Check(worker, tc.IsNil)
}

func (s *manifoldSuite) TestNewWorkerSuccess(c *tc.C) {
	expectWorker := &fakeWorker{}
	config := s.namesConfig(c)
	var gotConfig undertaker.Config
	config.NewFacade = func(_ base.APICaller) (undertaker.Facade, error) {
		return &fakeFacade{}, nil
	}
	config.NewWorker = func(workerConfig undertaker.Config) (worker.Worker, error) {
		gotConfig = workerConfig
		return expectWorker, nil
	}
	manifold := undertaker.Manifold(config)
	resources := resourcesMissing()

	worker, err := manifold.Start(context.Background(), resources.Getter())
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker, tc.Equals, expectWorker)
	c.Assert(gotConfig.Logger, tc.Equals, loggertesting.WrapCheckLog(c))
}

func resourcesMissing(missing ...string) dt.StubResources {
	resources := dt.StubResources{
		"api-caller": dt.NewStubResource(&fakeAPICaller{}),
		"environ":    dt.NewStubResource(&fakeEnviron{}),
		"broker":     dt.NewStubResource(&fakeBroker{}),
	}
	for _, name := range missing {
		resources[name] = dt.StubResource{Error: dependency.ErrMissing}
	}
	return resources
}

func checkResource(c *tc.C, actual interface{}, resources dt.StubResources, name string) {
	c.Check(actual, tc.Equals, resources[name].Outputs[0])
}

type fakeAPICaller struct {
	base.APICaller
}

type fakeEnviron struct {
	environs.Environ
}

type fakeBroker struct {
	caas.Broker
}

type fakeFacade struct {
	undertaker.Facade
}

type fakeWorker struct {
	worker.Worker
}
