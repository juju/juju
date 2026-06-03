// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"
	"errors"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	agent "github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	model "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
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

func (s *workerSuite) TestObjectStoreDrainingAlreadyExistsIsFatal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	draining := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	// Pre-register a worker with name "controller" in the runner to simulate
	// a prior invocation that is still alive.
	internalW := w.(*Worker)
	err := internalW.runner.StartWorker(context.Background(), "controller", func(ctx context.Context) (worker.Worker, error) {
		return newBlockingWorker(), nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Trigger draining - drainAgentBinaries will get AlreadyExists from runner.
	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event to be consumed")
	}

	// The worker should die with ErrWorkerInUnknownState.
	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, ErrWorkerInUnknownState)
}

func newBlockingWorker() worker.Worker {
	w := &errorWorker{}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (s *workerSuite) TestObjectStoreDrainingModelAlreadyExistsIsFatal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	draining := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{"model-uuid1"}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	// Pre-register a worker with name "model-uuid1" in the runner to simulate
	// a prior invocation that is still alive.
	internalW := w.(*Worker)
	err := internalW.runner.StartWorker(context.Background(), "model-uuid1", func(ctx context.Context) (worker.Worker, error) {
		return newBlockingWorker(), nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Trigger draining - drainModels will get AlreadyExists from runner.
	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event to be consumed")
	}

	// The worker should die with ErrWorkerInUnknownState.
	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, ErrWorkerInUnknownState)
}

func (s *workerSuite) TestObjectStoreDrainingNamespaceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

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

func (s *workerSuite) TestDrainAgentBinariesTimeout(c *tc.C) {
	defer s.setupMocks(c).Finish()

	draining := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	// We use testclock rather than a mock clock because the same clock is
	// shared with the internal worker.Runner (for restart delays). A mock
	// clock would require coupling to Runner's internal After/NewTimer
	// call patterns, which are implementation details.
	clk := testclock.NewClock(time.Now())
	cfg := s.getConfig(c)
	cfg.Clock = clk
	// Use a drain worker that never completes.
	cfg.NewDrainerWorker = func(completed chan<- drainResult, fileSystem HashFileSystemAccessor, client objectstore.Client, metadataService objectstore.ObjectStoreMetadata, rootBucket, namespace string, selectFileHash SelectFileHashFunc, clk clock.Clock, logger logger.Logger) worker.Worker {
		return newBlockingWorker()
	}

	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Trigger draining.
	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	// Wait for the clock.After to be registered, then advance past timeout.
	err = clk.WaitAdvance(defaultDrainTimeout+time.Second, 5*time.Second, 1)
	c.Assert(err, tc.ErrorIsNil)

	// The worker should die with a timeout error.
	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, ".*timeout waiting for controller agent binaries to drain.*")
}

func (s *workerSuite) TestWaitForDrainingTimeout(c *tc.C) {
	defer s.setupMocks(c).Finish()

	draining := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{"model-uuid1"}, nil)
	s.objectStoreServicesGetter.EXPECT().ServicesForModel(model.UUID("model-uuid1")).Return(s.objectStoreService)
	s.objectStoreService.EXPECT().ObjectStore().Return(s.objectStoreMetadata)

	// We use testclock rather than a mock clock because the same clock is
	// shared with the internal worker.Runner (for restart delays). A mock
	// clock would require coupling to Runner's internal After/NewTimer
	// call patterns, which are implementation details.
	clk := testclock.NewClock(time.Now())
	cfg := s.getConfig(c)
	cfg.Clock = clk

	callCount := 0
	cfg.NewDrainerWorker = func(completed chan<- drainResult, fileSystem HashFileSystemAccessor, client objectstore.Client, metadataService objectstore.ObjectStoreMetadata, rootBucket, namespace string, selectFileHash SelectFileHashFunc, clk clock.Clock, logger logger.Logger) worker.Worker {
		callCount++
		if callCount == 1 {
			// Controller drain completes immediately.
			return newTestWorkerWithNamespace(completed, "controller")
		}
		// Model drain worker never completes.
		return newBlockingWorker()
	}

	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Trigger draining.
	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	// Wait for both timers (drainAgentBinaries + waitForDraining) and
	// advance past timeout. The first timer (drainAgentBinaries) will be
	// satisfied by the fast controller worker. We need to wait for the
	// second timer.
	err = clk.WaitAdvance(defaultDrainTimeout+time.Second, 5*time.Second, 2)
	c.Assert(err, tc.ErrorIsNil)

	// The worker should die with a timeout error about drain workers.
	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, ".*timeout waiting for .* drain workers to complete.*")
}

func (s *workerSuite) TestWaitForDrainingModelFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	draining := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{"model-uuid1"}, nil)
	s.objectStoreServicesGetter.EXPECT().ServicesForModel(model.UUID("model-uuid1")).Return(s.objectStoreService)
	s.objectStoreService.EXPECT().ObjectStore().Return(s.objectStoreMetadata)

	cfg := s.getConfig(c)
	callCount := 0
	cfg.NewDrainerWorker = func(completed chan<- drainResult, fileSystem HashFileSystemAccessor, client objectstore.Client, metadataService objectstore.ObjectStoreMetadata, rootBucket, namespace string, selectFileHash SelectFileHashFunc, clk clock.Clock, logger logger.Logger) worker.Worker {
		callCount++
		if callCount == 1 {
			// Controller drain completes immediately.
			return newTestWorkerWithNamespace(completed, "controller")
		}
		// Model drain worker reports failure.
		return newFailingTestWorker(completed, namespace, errors.New("s3 upload failed"))
	}

	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Trigger draining.
	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	// The worker should die with a model failure error.
	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, `.*drain worker for model "model-uuid1" failed.*s3 upload failed.*`)
}

func (s *workerSuite) TestCompleteDrainingChangeConfigError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	draining := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	// GetModelNamespaces returns empty so completeDraining is called directly.
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	// ChangeConfig fails — SetDrainingPhase(PhaseCompleted) is never reached
	// because it comes after ChangeConfig in the new ordering.
	s.agent.EXPECT().ChangeConfig(gomock.Any()).Return(errors.New("disk full"))

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	// Trigger draining.
	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, ".*disk full.*")
}

func (s *workerSuite) TestCompleteDrainingFlushWorkersError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	draining := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	s.agentConfigSetter.EXPECT().ObjectStoreType().Return(objectstore.FileBackend)
	s.agentConfigSetter.EXPECT().SetObjectStoreType(objectstore.FileBackend)
	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(fn agent.ConfigMutator) error {
		return fn(s.agentConfigSetter)
	})

	// FlushWorkers fails — SetDrainingPhase(PhaseCompleted) is never reached.
	s.objectStoreFlusher.EXPECT().FlushWorkers(gomock.Any()).Return(errors.New("flush failed"))

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	// Trigger draining.
	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, ".*flush failed.*")
}

// TestRecoverFromCrashDuringChangeConfig verifies that if the worker crashes
// (hard kill) during ChangeConfig, on restart the phase is still Draining and
// the worker re-enters the draining flow and completes successfully.
func (s *workerSuite) TestRecoverFromCrashDuringChangeConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// --- First worker: crashes during ChangeConfig ---

	draining1 := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining1), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	// ChangeConfig fails (simulating a crash scenario where error path runs).
	s.agent.EXPECT().ChangeConfig(gomock.Any()).Return(errors.New("disk full"))

	w1 := s.newWorker(c)

	select {
	case draining1 <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	err := workertest.CheckKilled(c, w1)
	c.Assert(err, tc.ErrorMatches, ".*disk full.*")

	// --- Second worker: simulates restart, phase is still Draining ---

	draining2 := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining2), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	// This time ChangeConfig succeeds.
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

	w2 := s.newWorker(c)
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

// TestRecoverFromCrashDuringFlushWorkers verifies that if the worker crashes
// during FlushWorkers, on restart the phase is still Draining and the worker
// re-enters the draining flow and completes successfully. ChangeConfig is
// idempotent so re-applying it on the second attempt is harmless.
func (s *workerSuite) TestRecoverFromCrashDuringFlushWorkers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// --- First worker: crashes during FlushWorkers ---

	draining1 := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining1), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	// ChangeConfig succeeds but FlushWorkers fails.
	s.agentConfigSetter.EXPECT().ObjectStoreType().Return(objectstore.FileBackend)
	s.agentConfigSetter.EXPECT().SetObjectStoreType(objectstore.FileBackend)
	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(fn agent.ConfigMutator) error {
		return fn(s.agentConfigSetter)
	})
	s.objectStoreFlusher.EXPECT().FlushWorkers(gomock.Any()).Return(errors.New("flush timeout"))

	w1 := s.newWorker(c)

	select {
	case draining1 <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	err := workertest.CheckKilled(c, w1)
	c.Assert(err, tc.ErrorMatches, ".*flush timeout.*")

	// --- Second worker: simulates restart, phase is still Draining ---

	draining2 := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining2), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	// Both ChangeConfig and FlushWorkers succeed on retry.
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

	w2 := s.newWorker(c)
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

// TestRecoverFromCrashDuringSetDrainingPhaseCompleted verifies that if the
// worker crashes after ChangeConfig and FlushWorkers succeed but before
// SetDrainingPhase(PhaseCompleted) is persisted, on restart the phase is still
// Draining and the worker can complete the transition. Both ChangeConfig and
// FlushWorkers are idempotent so re-applying them is safe.
func (s *workerSuite) TestRecoverFromCrashDuringSetDrainingPhaseCompleted(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// --- First worker: ChangeConfig and FlushWorkers succeed, but
	// SetDrainingPhase(PhaseCompleted) fails (simulating crash before DB write) ---

	draining1 := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining1), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseError).Return(nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	s.agentConfigSetter.EXPECT().ObjectStoreType().Return(objectstore.FileBackend)
	s.agentConfigSetter.EXPECT().SetObjectStoreType(objectstore.FileBackend)
	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(fn agent.ConfigMutator) error {
		return fn(s.agentConfigSetter)
	})
	s.objectStoreFlusher.EXPECT().FlushWorkers(gomock.Any()).Return(nil)
	// SetDrainingPhase(PhaseCompleted) fails — simulates crash/network error.
	s.guardService.EXPECT().SetDrainingPhase(gomock.Any(), objectstore.PhaseCompleted).Return(errors.New("connection reset"))

	w1 := s.newWorker(c)

	select {
	case draining1 <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	err := workertest.CheckKilled(c, w1)
	c.Assert(err, tc.ErrorMatches, ".*connection reset.*")

	// --- Second worker: simulates restart, phase is still Draining ---

	draining2 := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining2), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

	// All steps succeed on retry (idempotent).
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

	w2 := s.newWorker(c)
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

func (s *workerSuite) TestDrainingPhaseError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	draining := make(chan struct{}, 1)
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	// Return PhaseError — the worker should log and continue (not die).
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseError, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	// Trigger draining watcher.
	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	// Give the worker time to process. If it didn't crash, it's still alive.
	// Send another event with PhaseCompleted to verify it kept looping.
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseCompleted, nil)
	done := make(chan struct{})
	s.guard.EXPECT().Unlock(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		defer close(done)
		return nil
	})

	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout sending second draining event")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for unlock")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestDrainingNoModelsCompletesDirectly(c *tc.C) {
	defer s.setupMocks(c).Finish()

	draining := make(chan struct{})
	s.guardService.EXPECT().WatchDraining(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(draining), nil
	})
	s.guardService.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).Return(nil)

	// No models — completeDraining called directly after agent binaries.
	s.controllerService.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, nil)

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

	// Trigger draining.
	select {
	case draining <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for draining event")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timeout waiting for flush")
	}

	workertest.CleanKill(c, w)
}

func newTestWorkerWithNamespace(ns chan<- drainResult, namespace string) worker.Worker {
	w := &errorWorker{}
	w.tomb.Go(func() error {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(50 * time.Millisecond):
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case ns <- drainResult{Namespace: namespace}:
				return nil
			}
		}
	})
	return w
}

func newFailingTestWorker(ns chan<- drainResult, namespace string, err error) worker.Worker {
	w := &errorWorker{}
	w.tomb.Go(func() error {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(50 * time.Millisecond):
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case ns <- drainResult{Namespace: namespace, Err: err}:
				return nil
			}
		}
	})
	return w
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
		DrainingService:              s.guardService,
		ControllerService:            s.controllerService,
		ObjectStoreServicesGetter:    s.objectStoreServicesGetter,
		ControllerObjectStoreService: s.controllerObjectStoreMetadata,
		ObjectStoreFlusher:           s.objectStoreFlusher,
		ObjectStoreType:              objectstore.FileBackend,
		S3Client:                     s.s3Client,
		NewHashFileSystemAccessor: func(namespace, rootDir string, logger logger.Logger) HashFileSystemAccessor {
			return s.hashFileSystemAccessor
		},
		NewDrainerWorker: func(completed chan<- drainResult, fileSystem HashFileSystemAccessor, client objectstore.Client, metadataService objectstore.ObjectStoreMetadata, rootBucket, namespace string, selectFileHash SelectFileHashFunc, clk clock.Clock, logger logger.Logger) worker.Worker {
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

func newTestWorker(ns chan<- drainResult) worker.Worker {
	w := &errorWorker{}
	w.tomb.Go(func() error {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(100 * time.Millisecond):
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case ns <- drainResult{Namespace: "model-uuid1"}:
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
