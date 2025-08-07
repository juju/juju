// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"
	"errors"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	agent "github.com/juju/juju/agent"
	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	model "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaltesting "github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	workerErr chan error
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestObjectStoreDrainingNotDraining(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(make(chan []string)), nil
	})

	draining := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(draining)

	done := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).Return(watcher, nil)
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseUnknown, nil)
	s.guard.EXPECT().Unlock(gomock.Any()).DoAndReturn(func(context.Context) error {
		defer close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for worker to start")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestObjectStoreDrainingDraining(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(make(chan []string)), nil
	})

	draining := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)

	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{"model-uuid1"}, nil)
	s.objectStoreServicesGetter.EXPECT().ServicesForModel(model.UUID("model-uuid1")).Return(s.objectStoreService)
	s.objectStoreService.EXPECT().ObjectStore().Return(s.objectStoreMetadata)

	s.agentConfigSetter.EXPECT().ObjectStoreType().Return(objectstore.FileBackend)
	s.agentConfigSetter.EXPECT().SetObjectStoreType(objectstore.FileBackend)
	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(fn agent.ConfigMutator) error {
		return fn(s.agentConfigSetter)
	})

	s.objectStoreFlusher.EXPECT().FlushWorkers(gomock.Any()).Return(nil)

	done := make(chan struct{})
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseCompleted).DoAndReturn(func(ctx context.Context, p objectstore.Phase) error {
		defer close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for worker to start")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestObjectStoreDrainingNamespaceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(make(chan []string)), nil
	})

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)

	done := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).Return(watcher, nil)
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).DoAndReturn(func(ctx context.Context, p objectstore.Phase) error {
		defer close(done)
		return nil
	})
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{"model-uuid1"}, errors.New("boom"))

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for worker to start")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for worker to start")
	}

	workertest.DirtyKill(c, w)
}

func (s *workerSuite) TestObjectStoreDrainingDrainingChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)
	sync := make(chan struct{}, 1)

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(changes), nil
	})
	cfg := internaltesting.FakeControllerConfig()
	cfg[controller.ObjectStoreType] = objectstore.S3Backend.String()
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	draining := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseUnknown, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseDraining).DoAndReturn(func(ctx context.Context, p objectstore.Phase) error {
		close(sync)
		return nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)

	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{"model-uuid1"}, nil)
	s.objectStoreServicesGetter.EXPECT().ServicesForModel(model.UUID("model-uuid1")).Return(s.objectStoreService)
	s.objectStoreService.EXPECT().ObjectStore().Return(s.objectStoreMetadata)

	s.agentConfigSetter.EXPECT().ObjectStoreType().Return(objectstore.FileBackend)
	s.agentConfigSetter.EXPECT().SetObjectStoreType(objectstore.S3Backend)
	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(fn agent.ConfigMutator) error {
		return fn(s.agentConfigSetter)
	})

	s.objectStoreFlusher.EXPECT().FlushWorkers(gomock.Any()).Return(nil)

	done := make(chan struct{})
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseCompleted).DoAndReturn(func(ctx context.Context, p objectstore.Phase) error {
		defer close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case changes <- []string{""}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for worker to start")
	}

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for worker to start")
	}

	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for worker to start")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	s.workerErr = make(chan error, 1)

	w, err := NewWorker(s.getConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *workerSuite) getConfig(c *tc.C) Config {
	return Config{
		Agent:                        s.agent,
		Guard:                        s.guard,
		GuardService:                 s.guardService,
		ControllerService:            s.controllerService,
		ControllerConfigService:      s.controllerConfigService,
		ObjectStoreServicesGetter:    s.objectStoreServicesGetter,
		ControllerObjectStoreService: s.controllerObjectStoreMetadata,
		ObjectStoreFlusher:           s.objectStoreFlusher,
		ObjectStoreType:              objectstore.FileBackend,
		S3Client:                     s.s3Client,
		NewHashFileSystemAccessor: func(namespace, rootDir string, logger logger.Logger) HashFileSystemAccessor {
			return s.hashFileSystemAccessor
		},
		NewDrainerWorker: func(completed chan<- string, fileSystem HashFileSystemAccessor, client objectstore.Client, metadataService objectstore.ObjectStoreMetadata, rootBucket, namespace string, selectFileHash SelectFileHashFunc, logger logger.Logger) worker.Worker {
			return newTestWorker(completed)
		},
		SelectFileHash: func(m objectstore.Metadata) string {
			return m.SHA384
		},
		RootDir:        c.MkDir(),
		RootBucketName: "test-bucket",
		Logger:         loggertesting.WrapCheckLog(c),
		Clock:          clock.WallClock,
	}
}

func newTestWorker(ns chan<- string) worker.Worker {
	w := &errorWorker{}
	w.tomb.Go(func() error {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(100 * time.Millisecond):
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case ns <- "model-uuid1":
				return nil
			}
		}
	})
	return w
}

type errorWorker struct {
	tomb tomb.Tomb
}

// Kill is part of the worker.Worker interface.
func (w *errorWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *errorWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *errorWorker) Completed() bool {
	return true
}
