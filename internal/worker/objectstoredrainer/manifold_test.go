// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	internaltesting "github.com/juju/juju/internal/testing"
)

type manifoldSuite struct {
	baseSuite
}

type stubRootDirReader struct {
	rootDir string
	err     error
}

func (s stubRootDirReader) ObjectStoreRootDir() (string, error) {
	return s.rootDir, s.err
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
	cfg.GetControllerObjectStoreService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetControllerConfigService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetDrainingService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewHashFileSystemAccessor = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.SelectFileHash = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.RootDirReader = nil
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

	cfg = s.getConfig(c)
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
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
		GetDrainingService: func(dependency.Getter, string) (DrainingService, error) {
			return s.guardService, nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetControllerObjectStoreService: func(getter dependency.Getter, name string) (objectstore.ObjectStoreMetadata, error) {
			return nil, nil
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
		RootDirReader: stubRootDirReader{rootDir: "/var/lib/juju"},
		Clock:         clock.WallClock,
		Logger:        loggertesting.WrapCheckLog(c),
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"fortress":              s.guard,
		"s3-client":             s.s3Client,
		"object-store":          s.objectStoreFlusher,
		"object-store-services": &stubObjectStoreServicesGetter{},
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"fortress", "s3-client", "object-store-services", "object-store"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig(c)).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := internaltesting.FakeControllerConfig()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	w, err := Manifold(s.getConfig(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
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

// getConfigWithRealWorker returns a ManifoldConfig that uses the real NewWorker
// constructor, allowing manifold-level tests to exercise the full startup and
// crash-recovery flow.
func (s *manifoldSuite) getConfigWithRealWorker(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		FortressName:            "fortress",
		ObjectStoreServicesName: "object-store-services",
		ObjectStoreName:         "object-store",
		S3ClientName:            "s3-client",
		GetControllerService: func(g dependency.Getter, name string) (ControllerService, error) {
			return s.controllerService, nil
		},
		GeObjectStoreServices: func(g dependency.Getter, name string) (ObjectStoreServicesGetter, error) {
			return s.objectStoreServicesGetter, nil
		},
		GetDrainingService: func(dependency.Getter, string) (DrainingService, error) {
			return s.guardService, nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetControllerObjectStoreService: func(getter dependency.Getter, name string) (objectstore.ObjectStoreMetadata, error) {
			return s.controllerObjectStoreMetadata, nil
		},
		NewHashFileSystemAccessor: func(namespace, rootDir string, logger logger.Logger) HashFileSystemAccessor {
			return s.hashFileSystemAccessor
		},
		SelectFileHash: func(m objectstore.Metadata) string {
			return m.SHA384
		},
		NewDrainerWorker: func(completed chan<- drainResult, fileSystem HashFileSystemAccessor, client objectstore.Client, metadataService objectstore.ObjectStoreMetadata, rootBucket, namespace string, selectFileHash SelectFileHashFunc, clk clock.Clock, logger logger.Logger) worker.Worker {
			return newTestWorker(completed)
		},
		NewWorker:     NewWorker,
		RootDirReader: stubRootDirReader{rootDir: "/var/lib/juju"},
		Clock:         clock.WallClock,
		Logger:        loggertesting.WrapCheckLog(c),
	}
}

// setupManifoldStartExpectations sets up the common expectations for the
// manifold start method.
func (s *manifoldSuite) setupManifoldStartExpectations(c *tc.C) {
	cfg := internaltesting.FakeControllerConfig()
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)
}

func (s *manifoldSuite) TestStartUsesExplicitRootDirAndWallClock(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rootDir := c.MkDir()
	cfg := s.getConfig(c)
	cfg.RootDirReader = stubRootDirReader{rootDir: rootDir}

	newWorkerCalled := false
	cfg.NewWorker = func(workerConfig Config) (worker.Worker, error) {
		newWorkerCalled = true
		c.Check(workerConfig.RootDir, tc.Equals, rootDir)
		c.Check(workerConfig.Clock, tc.Equals, clock.WallClock)
		return workertest.NewErrorWorker(nil), nil
	}

	controllerCfg := internaltesting.FakeControllerConfig()
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controllerCfg, nil)

	w, err := Manifold(cfg).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(newWorkerCalled, tc.IsTrue)
	workertest.CleanKill(c, w)
}

// TestManifoldRecoverFromFlushWorkersCrash exercises the full manifold start →
// worker crash → manifold restart → worker succeeds cycle when the crash occurs
// during FlushWorkers in completeDraining.
func (s *manifoldSuite) TestManifoldRecoverFromFlushWorkersCrash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// --- First start: worker enters draining, FlushWorkers fails ---

	s.setupManifoldStartExpectations(c)

	draining1 := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining1), nil
	})
	// Worker main loop reads phase from watcher.
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	s.objectStoreFlusher.EXPECT().FlushWorkers(gomock.Any()).Return(errors.New("flush timeout"))

	w1, err := Manifold(s.getConfigWithRealWorker(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)

	select {
	case draining1 <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	err = workertest.CheckKilled(c, w1)
	c.Assert(err, tc.ErrorMatches, ".*flush timeout.*")

	// --- Second start: simulates dependency engine restart ---
	// The manifold re-creates the worker and the idempotent completion path
	// succeeds on retry.

	s.setupManifoldStartExpectations(c)

	draining2 := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining2), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	s.objectStoreFlusher.EXPECT().FlushWorkers(gomock.Any()).Return(nil)

	done := make(chan struct{})
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseCompleted).DoAndReturn(func(ctx context.Context, p objectstore.Phase) error {
		defer close(done)
		return nil
	})

	w2, err := Manifold(s.getConfigWithRealWorker(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w2)

	select {
	case draining2 <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for completion")
	}

	workertest.CleanKill(c, w2)
}

// TestManifoldRecoverFromSetPhaseCrash exercises the full manifold start →
// worker crash → manifold restart → worker succeeds cycle when the crash occurs
// during SetDrainingPhase(PhaseCompleted) in completeDraining.
func (s *manifoldSuite) TestManifoldRecoverFromSetPhaseCrash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// --- First start: all steps succeed except the final commit ---

	s.setupManifoldStartExpectations(c)

	draining1 := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining1), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	s.objectStoreFlusher.EXPECT().FlushWorkers(gomock.Any()).Return(nil)
	// SetDrainingPhase(PhaseCompleted) fails, simulating a crash before commit.
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseCompleted).Return(errors.New("connection reset"))

	w1, err := Manifold(s.getConfigWithRealWorker(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)

	select {
	case draining1 <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	err = workertest.CheckKilled(c, w1)
	c.Assert(err, tc.ErrorMatches, ".*connection reset.*")

	// --- Second start: recovery succeeds ---

	s.setupManifoldStartExpectations(c)

	draining2 := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining2), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	s.objectStoreFlusher.EXPECT().FlushWorkers(gomock.Any()).Return(nil)

	done := make(chan struct{})
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseCompleted).DoAndReturn(func(ctx context.Context, p objectstore.Phase) error {
		defer close(done)
		return nil
	})

	w2, err := Manifold(s.getConfigWithRealWorker(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w2)

	select {
	case draining2 <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for completion")
	}

	workertest.CleanKill(c, w2)
}
