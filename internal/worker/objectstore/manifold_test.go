// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.AgentName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.StateName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.TraceName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.ServiceFactoryName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.LeaseManagerName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.S3ClientName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewObjectStoreWorker = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:          "agent",
		StateName:          "state",
		TraceName:          "trace",
		ServiceFactoryName: "service-factory",
		LeaseManagerName:   "lease-manager",
		S3ClientName:       "s3-client",
		Clock:              s.clock,
		Logger:             s.logger,
		NewObjectStoreWorker: func(context.Context, objectstore.BackendType, string, ...internalobjectstore.Option) (internalobjectstore.TrackedObjectStore, error) {
			return nil, nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetMetadataService: func(getter dependency.Getter, name string) (MetadataService, error) {
			return s.metadataService, nil
		},
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"agent":           s.agent,
		"trace":           &stubTracerGetter{},
		"state":           s.stateTracker,
		"service-factory": &stubServiceFactoryGetter{},
		"lease-manager":   s.leaseManager,
		"s3-client":       s.s3Client,
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"agent", "state", "trace", "service-factory", "lease-manager", "s3-client"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStateTracker()
	s.expectAgentConfig(c)
	s.expectControllerConfig()

	w, err := Manifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) expectStateTracker() {
	s.stateTracker.EXPECT().Use().Return(&state.StatePool{}, &state.State{}, nil)
	s.stateTracker.EXPECT().Done()
}

func (s *manifoldSuite) expectAgentConfig(c *gc.C) {
	s.agentConfig.EXPECT().DataDir().Return(c.MkDir())
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig)
}

func (s *manifoldSuite) expectControllerConfig() {
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(testing.FakeControllerConfig(), nil)
}

type stubTracerGetter struct{}

func (s *stubTracerGetter) GetTracer(ctx context.Context, namespace trace.TracerNamespace) (trace.Tracer, error) {
	return trace.NoopTracer{}, nil
}

// Note: This replicates the ability to get a controller service factory and
// a model service factory from the service factory getter.
type stubServiceFactoryGetter struct {
	servicefactory.ServiceFactory
	servicefactory.ServiceFactoryGetter
}

func (s *stubServiceFactoryGetter) FactoryForModel(modelUUID string) servicefactory.ServiceFactory {
	return &stubServiceFactory{}
}

func (s *stubServiceFactoryGetter) ControllerConfig() *controllerconfigservice.Service {
	return nil
}

type stubServiceFactory struct {
	servicefactory.ServiceFactory
}

func (s *stubServiceFactory) ControllerConfig() *controllerconfigservice.WatchableService {
	return nil
}
