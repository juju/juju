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
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testing"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.AgentName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.TraceName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ObjectStoreServicesName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.LeaseManagerName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.S3ClientName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
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
		AgentName:               "agent",
		TraceName:               "trace",
		ObjectStoreServicesName: "object-store-services",
		LeaseManagerName:        "lease-manager",
		S3ClientName:            "s3-client",
		Clock:                   s.clock,
		Logger:                  s.logger,
		NewObjectStoreWorker: func(context.Context, objectstore.BackendType, string, ...internalobjectstore.Option) (internalobjectstore.TrackedObjectStore, error) {
			return nil, nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetMetadataService: func(getter dependency.Getter, name string) (MetadataService, error) {
			return s.metadataService, nil
		},
		IsBootstrapController: func(dataDir string) bool {
			return false
		},
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"agent":                 s.agent,
		"trace":                 &stubTracerGetter{},
		"object-store-services": &stubObjectStoreServicesGetter{},
		"lease-manager":         s.leaseManager,
		"s3-client":             s.s3Client,
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"agent", "trace", "object-store-services", "lease-manager", "s3-client"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAgentConfig(c)
	s.expectControllerConfig()

	w, err := Manifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
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

// Note: This replicates the ability to get a controller domain services and
// a model domain services from the domain services getter.
type stubObjectStoreServicesGetter struct {
	services.ObjectStoreServices
	services.ObjectStoreServicesGetter
}

func (s *stubObjectStoreServicesGetter) ServicesForModel(model.UUID) services.ObjectStoreServices {
	return &stubObjectStoreServices{}
}

func (s *stubObjectStoreServicesGetter) ControllerConfig() *controllerconfigservice.WatchableService {
	return nil
}

type stubObjectStoreServices struct {
	services.ObjectStoreServices
}

func (s *stubObjectStoreServices) ControllerConfig() *controllerconfigservice.WatchableService {
	return nil
}
