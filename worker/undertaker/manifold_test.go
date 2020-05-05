// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"github.com/juju/errors"
	"github.com/juju/juju/caas"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/undertaker"
)

type manifoldSuite struct {
	testing.IsolationSuite
	modelType string
	logger    fakeLogger
}

type CAASManifoldSuite struct {
	manifoldSuite
}

type IAASManifoldSuite struct {
	manifoldSuite
}

var (
	_ = gc.Suite(&IAASManifoldSuite{})
	_ = gc.Suite(&CAASManifoldSuite{})
)

func (s *CAASManifoldSuite) SetUpTest(c *gc.C) {
	s.modelType = "caas"
}

func (s *IAASManifoldSuite) SetUpTest(c *gc.C) {
	s.modelType = "iaas"
}

func (s *manifoldSuite) destroyerName() string {
	if s.modelType == "iaas" {
		return "environ"
	}
	return "broker"
}

func (s *manifoldSuite) namesConfig() undertaker.ManifoldConfig {
	destroyerName := "environ"
	if s.modelType == "caas" {
		destroyerName = "broker"
	}
	return undertaker.ManifoldConfig{
		APICallerName:      "api-caller",
		CloudDestroyerName: destroyerName,
		Logger:             &s.logger,
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) {
			return &fakeCredentialAPI{}, nil
		},
	}
}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	manifold := undertaker.Manifold(s.namesConfig())
	c.Check(manifold.Inputs, jc.DeepEquals, []string{
		"api-caller", s.destroyerName(),
	})
}

func (s *manifoldSuite) TestOutput(c *gc.C) {
	manifold := undertaker.Manifold(s.namesConfig())
	c.Check(manifold.Output, gc.IsNil)
}

func (s *manifoldSuite) TestAPICallerMissing(c *gc.C) {
	resources := resourcesMissing("api-caller")
	manifold := undertaker.Manifold(s.namesConfig())

	worker, err := manifold.Start(resources.Context())
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *manifoldSuite) TestNewFacadeError(c *gc.C) {
	resources := resourcesMissing()
	config := s.namesConfig()
	config.NewFacade = func(apiCaller base.APICaller) (undertaker.Facade, error) {
		checkResource(c, apiCaller, resources, "api-caller")
		return nil, errors.New("blort")
	}
	manifold := undertaker.Manifold(config)

	worker, err := manifold.Start(resources.Context())
	c.Check(err, gc.ErrorMatches, "blort")
	c.Check(worker, gc.IsNil)
}

func (s *manifoldSuite) TestNewCredentialAPIError(c *gc.C) {
	config := s.namesConfig()
	config.NewFacade = func(_ base.APICaller) (undertaker.Facade, error) {
		return &fakeFacade{}, nil
	}
	config.NewCredentialValidatorFacade = func(apiCaller base.APICaller) (common.CredentialAPI, error) {
		return nil, errors.New("blort")
	}
	manifold := undertaker.Manifold(config)

	resources := resourcesMissing()
	worker, err := manifold.Start(resources.Context())
	c.Check(err, gc.ErrorMatches, "blort")
	c.Check(worker, gc.IsNil)
}

func (s *manifoldSuite) TestNewWorkerError(c *gc.C) {
	resources := resourcesMissing()
	expectFacade := &fakeFacade{}
	config := s.namesConfig()
	config.NewFacade = func(_ base.APICaller) (undertaker.Facade, error) {
		return expectFacade, nil
	}
	config.NewWorker = func(cfg undertaker.Config) (worker.Worker, error) {
		c.Check(cfg.Facade, gc.Equals, expectFacade)
		checkResource(c, cfg.Destroyer, resources, s.destroyerName())
		return nil, errors.New("lhiis")
	}
	manifold := undertaker.Manifold(config)

	worker, err := manifold.Start(resources.Context())
	c.Check(err, gc.ErrorMatches, "lhiis")
	c.Check(worker, gc.IsNil)
}

func (s *manifoldSuite) TestNewWorkerEnvironMissing(c *gc.C) {
	// If the environ tracker isn't available the undertaker can still
	// start, but the destroyer it gets passed will always return an
	// error.
	expectWorker := &fakeWorker{}
	config := s.namesConfig()
	var gotConfig undertaker.Config
	config.NewFacade = func(_ base.APICaller) (undertaker.Facade, error) {
		return &fakeFacade{}, nil
	}
	config.NewWorker = func(workerConfig undertaker.Config) (worker.Worker, error) {
		gotConfig = workerConfig
		return expectWorker, nil
	}

	resources := resourcesMissing("environ", "broker")
	manifold := undertaker.Manifold(config)

	worker, err := manifold.Start(resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.Equals, expectWorker)
	err = gotConfig.Destroyer.Destroy(nil)
	c.Assert(err, gc.ErrorMatches, "cloud environment unavailable")
}

func (s *manifoldSuite) TestNewWorkerSuccess(c *gc.C) {
	expectWorker := &fakeWorker{}
	config := s.namesConfig()
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

	worker, err := manifold.Start(resources.Context())
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, expectWorker)
	c.Assert(gotConfig.Logger, gc.Equals, &s.logger)
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

func checkResource(c *gc.C, actual interface{}, resources dt.StubResources, name string) {
	c.Check(actual, gc.Equals, resources[name].Outputs[0])
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

type fakeCredentialAPI struct{}

func (*fakeCredentialAPI) InvalidateModelCredential(reason string) error {
	return nil
}

type fakeLogger struct {
	stub testing.Stub
}

func (l *fakeLogger) Errorf(format string, args ...interface{}) {
	l.stub.AddCall("Errorf", format, args)
}
