// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modellife

import (
	"context"
	"sync/atomic"
	"time"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

type workerSuite struct {
	testing.IsolationSuite

	modelService *MockModelService

	modelUUID model.UUID
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	cfg := s.getConfig()
	c.Check(cfg.Validate(), gc.IsNil)

	cfg = s.getConfig()
	cfg.ModelUUID = ""
	c.Check(cfg.Validate(), jc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig()
	cfg.Result = nil
	c.Check(cfg.Validate(), jc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig()
	cfg.ModelService = nil
	c.Check(cfg.Validate(), jc.ErrorIs, jujuerrors.NotValid)
}

func (s *workerSuite) TestStartAlive(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelLife(gomock.Any(), s.modelUUID).Return(life.Alive, nil)

	done := make(chan struct{})
	s.modelService.EXPECT().WatchModel(gomock.Any(), s.modelUUID).DoAndReturn(func(ctx context.Context, u model.UUID) (watcher.Watcher[struct{}], error) {
		close(done)
		return watchertest.NewMockNotifyWatcher(make(<-chan struct{})), nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for worker to start")
	}

	c.Assert(w.Check(), jc.IsTrue)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestStartDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelLife(gomock.Any(), s.modelUUID).Return(life.Dead, nil)

	done := make(chan struct{})
	s.modelService.EXPECT().WatchModel(gomock.Any(), s.modelUUID).DoAndReturn(func(ctx context.Context, u model.UUID) (watcher.Watcher[struct{}], error) {
		close(done)
		return watchertest.NewMockNotifyWatcher(make(<-chan struct{})), nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for worker to start")
	}

	c.Assert(w.Check(), jc.IsFalse)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestStartError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelLife(gomock.Any(), s.modelUUID).Return(life.Alive, modelerrors.NotFound)

	cfg := s.getConfig()

	_, err := NewWorker(context.Background(), cfg)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *workerSuite) TestWatchModelError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelLife(gomock.Any(), s.modelUUID).Return(life.Alive, nil)

	done := make(chan struct{})

	s.modelService.EXPECT().WatchModel(gomock.Any(), s.modelUUID).DoAndReturn(func(ctx context.Context, u model.UUID) (watcher.Watcher[struct{}], error) {
		defer close(done)
		return nil, errors.Errorf("boom")
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for worker to start")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *workerSuite) TestWatchModelStillAlive(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelLife(gomock.Any(), s.modelUUID).Return(life.Alive, nil)

	done := make(chan struct{})

	ch := make(chan struct{})
	s.modelService.EXPECT().WatchModel(gomock.Any(), s.modelUUID).DoAndReturn(func(ctx context.Context, u model.UUID) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(ch), nil
	})
	s.modelService.EXPECT().GetModelLife(gomock.Any(), s.modelUUID).DoAndReturn(func(ctx context.Context, u model.UUID) (life.Value, error) {
		close(done)
		return life.Alive, nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- struct{}{}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for worker to start")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for worker to start")
	}

	c.Assert(w.Check(), jc.IsTrue)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWatchModelTransitionAliveToDying(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelLife(gomock.Any(), s.modelUUID).Return(life.Alive, nil)

	done := make(chan struct{})

	ch := make(chan struct{})
	s.modelService.EXPECT().WatchModel(gomock.Any(), s.modelUUID).DoAndReturn(func(ctx context.Context, u model.UUID) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(ch), nil
	})
	s.modelService.EXPECT().GetModelLife(gomock.Any(), s.modelUUID).DoAndReturn(func(ctx context.Context, u model.UUID) (life.Value, error) {
		close(done)
		return life.Dying, nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- struct{}{}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for worker to start")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for worker to start")
	}

	c.Assert(w.Check(), jc.IsTrue)

	err := workertest.CheckKilled(c, w)
	c.Assert(err, jc.ErrorIs, dependency.ErrBounce)
}

func (s *workerSuite) TestWatchModelTransitionDyingToDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelLife(gomock.Any(), s.modelUUID).Return(life.Dying, nil)

	done := make(chan struct{})

	ch := make(chan struct{}, 2)
	s.modelService.EXPECT().WatchModel(gomock.Any(), s.modelUUID).DoAndReturn(func(ctx context.Context, u model.UUID) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(ch), nil
	})

	var count int64
	s.modelService.EXPECT().GetModelLife(gomock.Any(), s.modelUUID).DoAndReturn(func(ctx context.Context, u model.UUID) (life.Value, error) {
		if c := atomic.AddInt64(&count, 1); c > 1 {
			close(done)
		}
		return life.Dead, nil
	}).Times(2)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	for i := 0; i < 2; i++ {
		select {
		case ch <- struct{}{}:
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for worker to start")
		}
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for worker to start")
	}

	c.Assert(w.Check(), jc.IsFalse)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) getConfig() Config {
	return Config{
		ModelUUID:    s.modelUUID,
		ModelService: s.modelService,
		Result:       life.IsAlive,
	}
}

func (s *workerSuite) newWorker(c *gc.C) *Worker {
	cfg := s.getConfig()

	w, err := NewWorker(context.Background(), cfg)
	c.Assert(err, jc.ErrorIsNil)

	return w
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelUUID = modeltesting.GenModelUUID(c)

	s.modelService = NewMockModelService(ctrl)

	return ctrl
}
