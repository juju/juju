// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestObjectStoreDrainingNotDraining(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)

	done := make(chan struct{})
	s.service.EXPECT().WatchDraining(gomock.Any()).Return(watcher, nil)
	s.service.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseUnknown, nil)
	s.guard.EXPECT().Unlock(gomock.Any()).DoAndReturn(func(context.Context) error {
		defer close(done)
		return nil
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

func (s *workerSuite) TestObjectStoreDrainingDraining(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)

	done := make(chan struct{})
	s.service.EXPECT().WatchDraining(gomock.Any()).Return(watcher, nil)
	s.service.EXPECT().GetDrainingPhase(gomock.Any()).Return(objectstore.PhaseDraining, nil)
	s.guard.EXPECT().Lockdown(gomock.Any()).DoAndReturn(func(context.Context) error {
		defer close(done)
		return nil
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

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	w, err := NewWorker(c.Context(), Config{
		ObjectStoreService: s.service,
		Guard:              s.guard,
		Logger:             loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return w
}
