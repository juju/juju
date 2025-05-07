// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/http"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	modelservice "github.com/juju/juju/domain/model/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
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
	leaseManager             lease.Manager
	httpClientGetter         http.HTTPClientGetter

	logger logger.Logger

	state *state.State
	pool  *state.StatePool

	stub testhelpers.Stub
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *tc.C) {

	var err error
	s.authority, err = pkitest.NewTestAuthority()
	c.Assert(err, tc.ErrorIsNil)

	s.BaseSuite.SetUpTest(c)

	s.state = &state.State{}
	s.pool = &state.StatePool{}

	s.leaseManager = stubLeaseManager{}
	s.domainServicesGetter = stubDomainServicesGetter{}
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
		LeaseManagerName:             "lease-manager",
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
		GetControllerConfig: func(ctx context.Context, domainServices services.DomainServices) (controller.Config, error) {
			return jujutesting.FakeControllerConfig(), nil
		},
	})
}

func (s *ManifoldSuite) newGetter(overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"authority":         s.authority,
		"log-sink":          s.logSinkGetter,
		"lease-manager":     s.leaseManager,
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

var expectedInputs = []string{"authority", "lease-manager", "log-sink", "domain-services", "provider-services", "http-client"}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *tc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(map[string]any{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context.Background(), getter)
		c.Assert(errors.Cause(err), tc.Equals, dependency.ErrMissing, tc.Commentf("failed for input: %v", input))
	}
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, tc.HasLen, 1)
	c.Assert(args[0], tc.FitsTypeOf, modelworkermanager.Config{})
	config := args[0].(modelworkermanager.Config)
	config.Authority = s.authority

	c.Assert(config.NewModelWorker, tc.NotNil)
	modelConfig := modelworkermanager.NewModelConfig{
		Authority:    s.authority,
		ModelName:    "test",
		ModelOwner:   "owner",
		ModelUUID:    "foo",
		ModelType:    coremodel.IAAS,
		ModelMetrics: dummyMetricSink{},
	}
	mw, err := config.NewModelWorker(modelConfig)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, mw)
	s.stub.CheckCallNames(c, "NewWorker", "NewModelWorker")
	s.stub.CheckCall(c, 1, "NewModelWorker", modelConfig)

	config.NewModelWorker = nil
	config.GetControllerConfig = nil

	c.Assert(config, tc.DeepEquals, modelworkermanager.Config{
		Authority:              s.authority,
		ModelMetrics:           dummyModelMetrics{},
		ErrorDelay:             jworker.RestartDelay,
		LeaseManager:           s.leaseManager,
		Logger:                 s.logger,
		LogSinkGetter:          dummyLogSinkGetter{},
		ProviderServicesGetter: providerServicesGetter{},
		ModelService:           s.controllerDomainServices.Model(),
		DomainServicesGetter:   s.domainServicesGetter,
		HTTPClientGetter:       s.httpClientGetter,
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *tc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) startWorkerClean(c *tc.C) worker.Worker {
	w, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

type stubLeaseManager struct {
	lease.Manager
}

type stubDomainServicesGetter struct {
	services.DomainServicesGetter
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
