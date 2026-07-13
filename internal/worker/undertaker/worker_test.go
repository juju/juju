// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"
	stdtesting "testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &workerSuite{})
	})
}

func (s *workerSuite) TestRemoveDeadModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{}, 1)
	s.expectWatchModels(ch)
	s.expectIdleDeletionWatcher()

	s.controllerModelService.EXPECT().GetDeadModels(gomock.Any()).Return([]model.UUID{model.UUID("model-1")}, nil)

	s.removalServiceGetter.EXPECT().GetRemovalService(gomock.Any(), model.UUID("model-1")).Return(s.removalService, nil)
	s.removalService.EXPECT().DeleteModel(gomock.Any()).Return(nil)

	done := make(chan struct{})
	s.dbDeleter.EXPECT().DeleteDB("model-1").DoAndReturn(func(s string) error {
		close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange(c, ch)
	s.waitDone(c, done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestRemoveDeadModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{}, 1)
	s.expectWatchModels(ch)
	s.expectIdleDeletionWatcher()

	s.controllerModelService.EXPECT().GetDeadModels(gomock.Any()).Return([]model.UUID{model.UUID("model-1")}, nil)

	s.removalServiceGetter.EXPECT().GetRemovalService(gomock.Any(), model.UUID("model-1")).Return(s.removalService, nil)
	s.removalService.EXPECT().DeleteModel(gomock.Any()).Return(modelerrors.NotFound)

	done := make(chan struct{})
	s.dbDeleter.EXPECT().DeleteDB("model-1").DoAndReturn(func(s string) error {
		close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange(c, ch)
	s.waitDone(c, done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestRemoveDeadModelDBNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{}, 1)
	s.expectWatchModels(ch)
	s.expectIdleDeletionWatcher()

	s.controllerModelService.EXPECT().GetDeadModels(gomock.Any()).Return([]model.UUID{model.UUID("model-1")}, nil)

	s.removalServiceGetter.EXPECT().GetRemovalService(gomock.Any(), model.UUID("model-1")).Return(s.removalService, nil)
	s.removalService.EXPECT().DeleteModel(gomock.Any()).Return(coredatabase.ErrDBNotFound)

	done := make(chan struct{})
	s.dbDeleter.EXPECT().DeleteDB("model-1").DoAndReturn(func(s string) error {
		close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange(c, ch)
	s.waitDone(c, done)

	workertest.CleanKill(c, w)
}

// TestDeletesPendingDatabase asserts a staged deletion results in the database
// being deleted and the staged row removed.
func (s *workerSuite) TestDeletesPendingDatabase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIdleModelWatcher()

	ch := make(chan struct{}, 1)
	s.expectWatchModelDatabaseDeletions(ch)

	s.controllerModelService.EXPECT().GetPendingModelDatabaseDeletions(gomock.Any()).Return([]string{"ns1"}, nil)
	s.dbDeleter.EXPECT().DeleteDB("ns1").Return(nil)

	done := make(chan struct{})
	s.controllerModelService.EXPECT().RemoveModelDatabaseDeletion(gomock.Any(), "ns1").DoAndReturn(func(ctx context.Context, namespace string) error {
		close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange(c, ch)
	s.waitDone(c, done)

	workertest.CleanKill(c, w)
}

// TestDeleteDBNotFoundStillCompletes asserts a not-found database is treated as
// already deleted and the staged row is still removed.
func (s *workerSuite) TestDeleteDBNotFoundStillCompletes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIdleModelWatcher()

	ch := make(chan struct{}, 1)
	s.expectWatchModelDatabaseDeletions(ch)

	s.controllerModelService.EXPECT().GetPendingModelDatabaseDeletions(gomock.Any()).Return([]string{"ns1"}, nil)
	s.dbDeleter.EXPECT().DeleteDB("ns1").Return(jujuerrors.NotFoundf("database ns1"))

	done := make(chan struct{})
	s.controllerModelService.EXPECT().RemoveModelDatabaseDeletion(gomock.Any(), "ns1").DoAndReturn(func(ctx context.Context, namespace string) error {
		close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange(c, ch)
	s.waitDone(c, done)

	workertest.CleanKill(c, w)
}

// TestDeleteFailureDoesNotRemoveRow asserts that when the database deletion
// fails, the staged row is not removed (so the deletion is retried) and the
// main worker keeps running.
func (s *workerSuite) TestDeleteFailureDoesNotRemoveRow(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIdleModelWatcher()

	ch := make(chan struct{}, 1)
	s.expectWatchModelDatabaseDeletions(ch)

	s.controllerModelService.EXPECT().GetPendingModelDatabaseDeletions(gomock.Any()).Return([]string{"ns1"}, nil)

	// The failing per-namespace worker is restarted by the runner, so the
	// deletion is attempted at least once. RemoveModelDatabaseDeletion is
	// never expected: a failed deletion must leave the staged row in place.
	done := make(chan struct{})
	var closed bool
	s.dbDeleter.EXPECT().DeleteDB("ns1").DoAndReturn(func(namespace string) error {
		if !closed {
			closed = true
			close(done)
		}
		return errors.Errorf("boom")
	}).MinTimes(1)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.sendChange(c, ch)
	s.waitDone(c, done)

	// The main worker stays alive despite the per-namespace failure.
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

// TestNoPendingDeletions asserts a change event with no staged deletions does
// nothing.
func (s *workerSuite) TestNoPendingDeletions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIdleModelWatcher()

	ch := make(chan struct{}, 1)
	s.expectWatchModelDatabaseDeletions(ch)

	done := make(chan struct{})
	s.controllerModelService.EXPECT().GetPendingModelDatabaseDeletions(gomock.Any()).DoAndReturn(func(ctx context.Context) ([]string, error) {
		close(done)
		return nil, nil
	})

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange(c, ch)
	s.waitDone(c, done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) expectWatchModels(ch chan struct{}) {
	s.controllerModelService.EXPECT().WatchModels(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
		return watchertest.NewMockNotifyWatcher(ch), nil
	})
}

func (s *workerSuite) expectWatchModelDatabaseDeletions(ch chan struct{}) {
	s.controllerModelService.EXPECT().WatchModelDatabaseDeletions(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
		return watchertest.NewMockNotifyWatcher(ch), nil
	})
}

// expectIdleModelWatcher sets up a model watcher that never fires, for tests
// that only exercise the database deletion path.
func (s *workerSuite) expectIdleModelWatcher() {
	s.expectWatchModels(make(chan struct{}))
}

// expectIdleDeletionWatcher sets up a database deletion watcher that never
// fires, for tests that only exercise the dead-model path.
func (s *workerSuite) expectIdleDeletionWatcher() {
	s.expectWatchModelDatabaseDeletions(make(chan struct{}))
}

func (s *workerSuite) sendChange(c *tc.C, ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatal("timed out waiting to send change")
	}
}

func (s *workerSuite) waitDone(c *tc.C, done chan struct{}) {
	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for work to complete")
	}
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	cfg := s.getConfig()

	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)

	return w
}

func (s *workerSuite) getConfig() Config {
	return Config{
		DBDeleter:              s.dbDeleter,
		ControllerModelService: s.controllerModelService,
		RemovalServiceGetter:   s.removalServiceGetter,
		Clock:                  clock.WallClock,
		Logger:                 s.logger,
	}
}
