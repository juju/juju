// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	agent "github.com/juju/juju/agent"
	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	internaltesting "github.com/juju/juju/internal/testing"
	internalworker "github.com/juju/juju/internal/worker"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.AgentName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.FortressName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.ObjectStoreServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.S3ClientName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetControllerService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GeObjectStoreServices = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetGuardService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetControllerConfigService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewHashFileSystemAccessor = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.SelectFileHash = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewDrainerWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		AgentName:               "agent",
		FortressName:            "fortress",
		ObjectStoreServicesName: "object-store-services",
		ObjectStoreName:         "object-store",
		S3ClientName:            "s3-client",
		GetControllerService: func(g dependency.Getter, s string) (ControllerService, error) {
			return nil, nil
		},
		GeObjectStoreServices: func(g dependency.Getter, s string) (ObjectStoreServicesGetter, error) {
			return nil, nil
		},
		GetGuardService: func(dependency.Getter, string) (GuardService, error) {
			return s.guardService, nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		NewHashFileSystemAccessor: func(namespace, rootDir string, logger logger.Logger) HashFileSystemAccessor {
			return nil
		},
		SelectFileHash: func(m objectstore.Metadata) string {
			return m.SHA384
		},
		NewDrainerWorker: NewDrainWorker,
		NewWorker: func(config Config) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
		Logger: loggertesting.WrapCheckLog(c),
		Clock:  clock.WallClock,
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"agent":                 s.agent,
		"fortress":              s.guard,
		"s3-client":             s.s3Client,
		"object-store":          s.objectStoreFlusher,
		"object-store-services": &stubObjectStoreServicesGetter{},
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"agent", "fortress", "s3-client", "object-store-services", "object-store"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig(c)).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfig.EXPECT().DataDir().Return(c.MkDir())
	s.agentConfigSetter.EXPECT().ObjectStoreType().Return(objectstore.FileBackend)
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig)

	cfg := internaltesting.FakeControllerConfig()
	cfg[controller.ObjectStoreType] = objectstore.FileBackend.String()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseUnknown, nil)

	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(fn agent.ConfigMutator) error {
		return fn(s.agentConfigSetter)
	})

	w, err := Manifold(s.getConfig(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestStartObjectStoreTypeChangedWhilstDraining(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfig.EXPECT().DataDir().Return(c.MkDir())
	s.agentConfigSetter.EXPECT().ObjectStoreType().Return(objectstore.FileBackend)
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig)

	cfg := internaltesting.FakeControllerConfig()
	cfg[controller.ObjectStoreType] = objectstore.S3Backend.String()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)

	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(fn agent.ConfigMutator) error {
		return fn(s.agentConfigSetter)
	})

	w, err := Manifold(s.getConfig(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestStartUpdatesObjectStoreType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfig.EXPECT().DataDir().Return(c.MkDir())
	s.agentConfigSetter.EXPECT().ObjectStoreType().Return(objectstore.FileBackend)
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig)

	cfg := internaltesting.FakeControllerConfig()
	cfg[controller.ObjectStoreType] = objectstore.S3Backend.String()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseUnknown, nil)

	s.agentConfigSetter.EXPECT().SetObjectStoreType(objectstore.S3Backend)

	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(fn agent.ConfigMutator) error {
		return fn(s.agentConfigSetter)
	})

	_, err := Manifold(s.getConfig(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIs, internalworker.ErrRestartAgent)
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

type stubObjectStoreServices struct {
	services.ObjectStoreServices
}

func (s *stubObjectStoreServices) ObjectStore() *objectstoreservice.WatchableService {
	return nil
}
