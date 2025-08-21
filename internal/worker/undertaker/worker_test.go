// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
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

	ch := make(chan struct{}, 1)
	s.controllerModelService.EXPECT().WatchModels(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
		return watchertest.NewMockNotifyWatcher(ch), nil
	})

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

	select {
	case ch <- struct{}{}:
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
