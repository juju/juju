// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	modelerrors "github.com/juju/juju/domain/model/errors"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestRemoveDeadModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	s.controllerModelService.EXPECT().WatchActivatedModels(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(ch), nil
	})

	s.controllerModelService.EXPECT().GetModelLife(gomock.Any(), model.UUID("model-1")).Return(life.Dead, nil)

	s.removalServiceGetter.EXPECT().GetRemovalService(gomock.Any(), model.UUID("model-1")).Return(s.removalService, nil)
	s.removalService.EXPECT().DeleteModel(gomock.Any()).Return(nil)

	done := make(chan struct{})
	s.dbDeleter.EXPECT().DeleteDB("model-1").DoAndReturn(func(s string) error {
		close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	select {
	case ch <- []string{"model-1"}:
	case <-c.Context().Done():
		c.Fatal("timed out waiting to send model names")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for model deletion")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestRemoveNotFoundModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	s.controllerModelService.EXPECT().WatchActivatedModels(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(ch), nil
	})

	s.controllerModelService.EXPECT().GetModelLife(gomock.Any(), model.UUID("model-1")).Return(life.Dead, modelerrors.NotFound)

	done := make(chan struct{})
	s.dbDeleter.EXPECT().DeleteDB("model-1").DoAndReturn(func(s string) error {
		close(done)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	select {
	case ch <- []string{"model-1"}:
	case <-c.Context().Done():
		c.Fatal("timed out waiting to send model names")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for model deletion")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestRemoveAliveModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	s.controllerModelService.EXPECT().WatchActivatedModels(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(ch), nil
	})

	s.controllerModelService.EXPECT().GetModelLife(gomock.Any(), model.UUID("model-1")).Return(life.Alive, nil)

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	select {
	case ch <- []string{"model-1"}:
	case <-c.Context().Done():
		c.Fatal("timed out waiting to send model names")
	}

	<-time.After(time.Millisecond * 500)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestRemoveDyingModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	s.controllerModelService.EXPECT().WatchActivatedModels(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(ch), nil
	})

	s.controllerModelService.EXPECT().GetModelLife(gomock.Any(), model.UUID("model-1")).Return(life.Dying, nil)

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	select {
	case ch <- []string{"model-1"}:
	case <-c.Context().Done():
		c.Fatal("timed out waiting to send model names")
	}

	<-time.After(time.Millisecond * 500)

	workertest.CleanKill(c, w)
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
