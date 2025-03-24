// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	modelservice "github.com/juju/juju/domain/model/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/services"
	jujutesting "github.com/juju/juju/internal/testing"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/modelworkermanager"
	"github.com/juju/juju/state"
)

type ManifoldSuite struct {
	jujutesting.BaseSuite

	authority                pki.Authority
	manifold                 dependency.Manifold
	getter                   dependency.Getter
	logSinkGetter            dummyLogSinkGetter
	domainServicesGetter     services.DomainServicesGetter
	controllerDomainServices services.ControllerDomainServices
	providerServicesGetter   services.ProviderServicesGetter
	httpClientGetter         http.HTTPClientGetter

	logger logger.Logger

	state *state.State
	pool  *state.StatePool

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	ctrl := gomock.NewController(c)
	var err error
	s.authority, err = pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	s.BaseSuite.SetUpTest(c)

	s.state = &state.State{}
	s.pool = &state.StatePool{}
	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.controllerDomainServices = stubControllerDomainServices{}
	s.providerServicesGetter = stubProviderServicesGetter{}
	s.httpClientGetter = stubHTTPClientGetter{}
	s.stub.ResetCalls()

	s.logSinkGetter = dummyLogSinkGetter{}
	s.logger = loggertesting.WrapCheckLog(c)

	s.getter = s.newGetter(nil)
	s.manifold = modelworkermanager.Manifold(modelworkermanager.ManifoldConfig{
		AuthorityName:                "authority",
		LogSinkName:                  "log-sink",
		DomainServicesName:           "domain-services",
		ProviderServiceFactoriesName: "provider-services",
		HTTPClientName:               "http-client",
		NewWorker:                    s.newWorker,
		NewModelWorker:               s.newModelWorker,
		ModelMetrics:                 dummyModelMetrics{},
		Logger:                       s.logger,
		GetProviderServicesGetter: func(getter dependency.Getter, name string) (modelworkermanager.ProviderServicesGetter, error) {
			var a any
			if err := getter.Get(name, &a); err != nil {
				return nil, errors.Trace(err)
			}
			return providerServicesGetter{}, nil
		},
		GetControllerConfig: func(ctx context.Context, controllerConfigService modelworkermanager.ControllerConfigService) (controller.Config, error) {
			return jujutesting.FakeControllerConfig(), nil
		},
	})
}

func (s *ManifoldSuite) newGetter(overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"authority":         s.authority,
		"log-sink":          s.logSinkGetter,
		"domain-services":   []any{s.domainServicesGetter, s.controllerDomainServices},
		"provider-services": s.providerServicesGetter,
		"http-client":       s.httpClientGetter,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubGetter(resources)
}

func (s *ManifoldSuite) newWorker(config modelworkermanager.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return worker.NewRunner(worker.RunnerParams{}), nil
}

func (s *ManifoldSuite) newModelWorker(config modelworkermanager.NewModelConfig) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewModelWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return worker.NewRunner(worker.RunnerParams{}), nil
}

var expectedInputs = []string{"authority", "log-sink", "domain-services", "provider-services", "http-client"}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(map[string]any{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context.Background(), getter)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing, gc.Commentf("failed for input: %v", input))
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, modelworkermanager.Config{})
	config := args[0].(modelworkermanager.Config)
	config.Authority = s.authority

	c.Assert(config.NewModelWorker, gc.NotNil)
	modelConfig := modelworkermanager.NewModelConfig{
		Authority:    s.authority,
		ModelName:    "test",
		ModelOwner:   "owner",
		ModelUUID:    "foo",
		ModelType:    state.ModelTypeIAAS,
		ModelMetrics: dummyMetricSink{},
	}
	mw, err := config.NewModelWorker(modelConfig)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, mw)
	s.stub.CheckCallNames(c, "NewWorker", "NewModelWorker")
	s.stub.CheckCall(c, 1, "NewModelWorker", modelConfig)

	config.NewModelWorker = nil
	config.GetControllerConfig = nil

	c.Assert(config, jc.DeepEquals, modelworkermanager.Config{
		Authority:              s.authority,
		ModelMetrics:           dummyModelMetrics{},
		ErrorDelay:             jworker.RestartDelay,
		Logger:                 s.logger,
		LogSinkGetter:          dummyLogSinkGetter{},
		ProviderServicesGetter: providerServicesGetter{},
		ModelService:           s.controllerDomainServices.Model(),
		DomainServicesGetter:   s.domainServicesGetter,
		HTTPClientGetter:       s.httpClientGetter,
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

type stubProviderServicesGetter struct {
	services.ProviderServicesGetter
}

type providerServicesGetter struct {
	modelworkermanager.ProviderServicesGetter
}

func (s providerServicesGetter) ServicesForModel(_ string) modelworkermanager.ProviderServices {
	return nil
}

type stubHTTPClientGetter struct {
	http.HTTPClientGetter
}

type stubControllerDomainServices struct {
	services.ControllerDomainServices
}

func (s stubControllerDomainServices) Model() *modelservice.WatchableService {
	return &modelservice.WatchableService{}
}
