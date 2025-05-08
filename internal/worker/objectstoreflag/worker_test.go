// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreflag

import (
	"context"
	time "time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestObjectStoreFlag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)

	done := make(chan struct{})
	s.service.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseUnknown, nil)
	s.service.EXPECT().WatchDraining(gomock.Any()).Return(watcher, nil)
	s.service.EXPECT().GetDrainingPhase(gomock.Any()).DoAndReturn(func(ctx context.Context) (objectstore.Phase, error) {
		defer close(done)
		return objectstore.PhaseDraining, nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- struct{}{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("timeout waiting for worker to start")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timeout waiting for worker to start")
	}

	err := workertest.CheckKill(c, w)
	c.Assert(err, jc.ErrorIs, ErrChanged)
}

func (s *workerSuite) TestObjectStoreFlagNoChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)

	done := make(chan struct{})
	s.service.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseUnknown, nil)
	s.service.EXPECT().WatchDraining(gomock.Any()).Return(watcher, nil)
	s.service.EXPECT().GetDrainingPhase(gomock.Any()).DoAndReturn(func(ctx context.Context) (objectstore.Phase, error) {
		defer close(done)
		return objectstore.PhaseUnknown, nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- struct{}{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("timeout waiting for worker to start")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timeout waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := NewWorker(context.Background(), Config{
		ModelUUID:          modeltesting.GenModelUUID(c),
		ObjectStoreService: s.service,
		Check: func(p objectstore.Phase) bool {
			return p == objectstore.PhaseDraining
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	return w
}
