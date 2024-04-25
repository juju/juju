// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	corelogger "github.com/juju/juju/core/logger"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/servicefactory"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/modelworkermanager"
	"github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
)

type ManifoldSuite struct {
	jujutesting.BaseSuite

	authority                    pki.Authority
	manifold                     dependency.Manifold
	getter                       dependency.Getter
	stateTracker                 stubStateTracker
	modelLogger                  dummyModelLogger
	serviceFactory               servicefactory.ServiceFactory
	providerServiceFactoryGetter servicefactory.ProviderServiceFactoryGetter
	controllerConfigGetter       *controllerconfigservice.WatchableService

	state *state.State
	pool  *state.StatePool

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	var err error
	s.authority, err = pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	s.BaseSuite.SetUpTest(c)

	s.state = &state.State{}
	s.pool = &state.StatePool{}
	s.stateTracker = stubStateTracker{pool: s.pool, state: s.state}
	s.controllerConfigGetter = &controllerconfigservice.WatchableService{}
	s.serviceFactory = stubServiceFactory{
		controllerConfigGetter: s.controllerConfigGetter,
	}
	s.providerServiceFactoryGetter = stubProviderServiceFactoryGetter{}
	s.stub.ResetCalls()

	s.modelLogger = dummyModelLogger{}

	s.getter = s.newGetter(nil)
	s.manifold = modelworkermanager.Manifold(modelworkermanager.ManifoldConfig{
		AgentName:                    "agent",
		AuthorityName:                "authority",
		StateName:                    "state",
		LogSinkName:                  "log-sink",
		ServiceFactoryName:           "service-factory",
		ProviderServiceFactoriesName: "provider-service-factory",
		NewWorker:                    s.newWorker,
		NewModelWorker:               s.newModelWorker,
		ModelMetrics:                 dummyModelMetrics{},
		Logger:                       loggo.GetLogger("test"),
		GetProviderServiceFactoryGetter: func(getter dependency.Getter, name string) (modelworkermanager.ProviderServiceFactoryGetter, error) {
			var a any
			if err := getter.Get(name, &a); err != nil {
				return nil, errors.Trace(err)
			}
			return providerServiceFactoryGetter{}, nil
		},
	})
}

func (s *ManifoldSuite) newGetter(overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"agent":                    &fakeAgent{},
		"authority":                s.authority,
		"state":                    &s.stateTracker,
		"log-sink":                 s.modelLogger,
		"service-factory":          s.serviceFactory,
		"provider-service-factory": s.providerServiceFactoryGetter,
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

var expectedInputs = []string{"agent", "authority", "state", "log-sink", "service-factory", "provider-service-factory"}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(map[string]any{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context.Background(), getter)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
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

	c.Assert(config, jc.DeepEquals, modelworkermanager.Config{
		Authority:    s.authority,
		ModelWatcher: s.state,
		ModelMetrics: dummyModelMetrics{},
		Controller: modelworkermanager.StatePoolController{
			StatePool: s.pool,
		},
		ControllerConfigGetter:       s.controllerConfigGetter,
		ErrorDelay:                   jworker.RestartDelay,
		Logger:                       loggo.GetLogger("test"),
		MachineID:                    "1",
		LogSink:                      dummyModelLogger{},
		ProviderServiceFactoryGetter: providerServiceFactoryGetter{},
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	s.stateTracker.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

type stubStateTracker struct {
	testing.Stub
	pool  *state.StatePool
	state *state.State
}

func (s *stubStateTracker) Use() (*state.StatePool, *state.State, error) {
	s.MethodCall(s, "Use")
	return s.pool, s.state, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}

func (s *stubStateTracker) Report() map[string]any {
	s.MethodCall(s, "Report")
	return nil
}

type fakeAgent struct {
	agent.Agent
	agent.Config
}

func (f *fakeAgent) CurrentConfig() agent.Config {
	return f
}

func (f *fakeAgent) Tag() names.Tag {
	return names.NewMachineTag("1")
}

type stubLogger struct {
	corelogger.LogWriterCloser
}

type stubServiceFactory struct {
	servicefactory.ServiceFactory
	controllerConfigGetter *controllerconfigservice.WatchableService
}

func (s stubServiceFactory) ControllerConfig() *controllerconfigservice.WatchableService {
	return s.controllerConfigGetter
}

type stubProviderServiceFactoryGetter struct {
	servicefactory.ProviderServiceFactoryGetter
}

type providerServiceFactoryGetter struct {
	modelworkermanager.ProviderServiceFactoryGetter
}

func (s providerServiceFactoryGetter) FactoryForModel(_ string) modelworkermanager.ProviderServiceFactory {
	return nil
}
