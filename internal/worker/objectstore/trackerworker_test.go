// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	context "context"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	watcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	modelerrors "github.com/juju/juju/domain/model/errors"
)

type trackerWorkerSuite struct {
	baseSuite
}

var _ objectstore.ObjectStore = (*trackerWorker)(nil)

func TestTrackerWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &trackerWorkerSuite{})
}

func (s *trackerWorkerSuite) TestWorkerStartup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.modelService.EXPECT().WatchModel(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		defer close(done)
		return watchertest.NewMockNotifyWatcher(make(chan struct{})), nil
	})

	w, err := s.newWorker()
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *trackerWorkerSuite) TestWorkerNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.modelService.EXPECT().WatchModel(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		ch := make(chan struct{}, 1)
		ch <- struct{}{}
		return watchertest.NewMockNotifyWatcher(ch), nil
	})
	s.modelService.EXPECT().Model(gomock.Any()).DoAndReturn(func(ctx context.Context) (model.ModelInfo, error) {
		return model.ModelInfo{}, modelerrors.NotFound
	})
	s.trackedObjectStore.EXPECT().RemoveAll(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		defer close(done)
		return nil
	})

	w, err := s.newWorker()
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to start")
	}

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *trackerWorkerSuite) TestWorkerDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.modelService.EXPECT().WatchModel(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		ch := make(chan struct{}, 1)
		ch <- struct{}{}
		return watchertest.NewMockNotifyWatcher(ch), nil
	})
	s.modelService.EXPECT().Model(gomock.Any()).DoAndReturn(func(ctx context.Context) (model.ModelInfo, error) {
		return model.ModelInfo{
			Life: life.Dead,
		}, nil
	})
	s.trackedObjectStore.EXPECT().RemoveAll(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		defer close(done)
		return nil
	})

	w, err := s.newWorker()
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to start")
	}

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *trackerWorkerSuite) newWorker() (*trackerWorker, error) {
	return newTrackerWorker(
		model.UUID("test-model-uuid"),
		s.modelService,
		newStubTrackedObjectStore(s.trackedObjectStore),
		trace.NoopTracer{},
		s.logger,
	)
}

type stubTrackedObjectStore struct {
	TrackedObjectStore
	tomb tomb.Tomb
}

func newStubTrackedObjectStore(o TrackedObjectStore) *stubTrackedObjectStore {
	w := &stubTrackedObjectStore{
		TrackedObjectStore: o,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (s *stubTrackedObjectStore) Kill() {
	s.tomb.Kill(nil)
}

func (s *stubTrackedObjectStore) Wait() error {
	return s.tomb.Wait()
}
