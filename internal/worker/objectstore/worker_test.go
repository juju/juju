// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	model "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	watcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type workerSuite struct {
	baseSuite

	states                     chan string
	trackedObjectStore         *MockTrackedObjectStore
	controllerMetadataService  *MockMetadataService
	modelMetadataServiceGetter *MockMetadataServiceGetter
	modelServiceGetter         *MockModelServiceGetter
	modelClaimGetter           *MockModelClaimGetter
	modelMetadataService       *MockMetadataService
	modelServices              *MockModelServices
	called                     int64
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestKilledGetObjectStoreErrDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	_, err := w.GetObjectStore(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, objectstore.ErrObjectStoreDying)
}

func (s *workerSuite) TestGetObjectStore(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	objectStore, err := w.GetObjectStore(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(objectStore, tc.NotNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestGetObjectStoreNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	_, err := w.GetObjectStore(c.Context(), "denied")
	c.Assert(err, tc.ErrorIs, objectstore.ErrObjectStoreNotFound)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestGetObjectStoreIsCached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	for range 10 {
		_, err := w.GetObjectStore(c.Context(), "foo")
		c.Assert(err, tc.ErrorIsNil)
	}

	workertest.CleanKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))
}

func (s *workerSuite) TestGetObjectStoreIsNotCachedForDifferentNamespaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	for i := range 10 {
		name := fmt.Sprintf("anything-%d", i)

		_, err := w.GetObjectStore(c.Context(), name)
		c.Assert(err, tc.ErrorIsNil)
	}

	workertest.CleanKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(10))
}

func (s *workerSuite) TestGetObjectStoreConcurrently(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	var wg sync.WaitGroup
	wg.Add(10)

	for i := range 10 {
		go func(i int) {
			defer wg.Done()

			name := fmt.Sprintf("anything-%d", i)

			_, err := w.GetObjectStore(c.Context(), name)
			c.Assert(err, tc.ErrorIsNil)
		}(i)
	}

	assertWait(c, wg.Wait)
	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(10))

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestFlushWorkersNoWorkers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	err := w.FlushWorkers(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestFlushWorkersRemovesCachedWorkers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	// Create a worker by requesting an object store.
	_, err := w.GetObjectStore(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(atomic.LoadInt64(&s.called), tc.Equals, int64(1))

	// Flush should remove all workers.
	err = w.FlushWorkers(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Getting the same namespace again should create a new worker,
	// proving the cache was cleared.
	_, err = w.GetObjectStore(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(atomic.LoadInt64(&s.called), tc.Equals, int64(2))

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestFlushWorkersErrDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	err := w.FlushWorkers(c.Context())
	c.Assert(err, tc.ErrorIs, objectstore.ErrObjectStoreDying)
}

func (s *workerSuite) TestFlushWorkersCancelledContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	// Cancel the context before calling FlushWorkers. The worker loop
	// is blocked processing the startup state, and the send on the
	// flushWorkers channel will race with the cancelled context.
	// To guarantee the context branch wins, we don't use ensureStartup's
	// guarantee of the loop being ready — instead we cancel first.
	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	err := w.FlushWorkers(ctx)
	c.Assert(err, tc.ErrorIs, context.Canceled)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *tc.C) *objectStoreWorker {
	w, err := newWorker(WorkerConfig{
		Clock:           s.clock,
		Logger:          s.logger,
		TracerGetter:    &stubTracerGetter{},
		S3Client:        s.s3Client,
		APIRemoteCaller: s.apiRemoteCaller,
		NewObjectStoreWorker: func(_ context.Context, _ objectstore.BackendType, ns string, _ ...internalobjectstore.Option) (internalobjectstore.TrackedObjectStore, error) {
			if ns == "denied" {
				return nil, database.ErrDBNotFound
			}
			atomic.AddInt64(&s.called, 1)
			return newStubTrackedObjectStore(s.trackedObjectStore), nil
		},
		NewTrackerWorker: func(modelUUID model.UUID, modelService ModelService, objectStore TrackedObjectStore, tracer trace.Tracer, logger logger.Logger) (worker.Worker, error) {
			return objectStore, nil
		},
		NewControllerWorker: func(objectStore TrackedObjectStore, tracer trace.Tracer) (worker.Worker, error) {
			panic("should not be called")
		},
		ControllerMetadataService:  s.controllerMetadataService,
		ObjectStoreService:         s.objectStoreService,
		ModelMetadataServiceGetter: s.modelMetadataServiceGetter,
		ModelServiceGetter:         s.modelServiceGetter,
		ModelClaimGetter:           s.modelClaimGetter,
		RootDir:                    c.MkDir(),
		RootBucket:                 uuid.MustNewUUID().String(),
		ControllerNodeID:           "0",
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)
	atomic.StoreInt64(&s.called, 0)

	ctrl := s.baseSuite.setupMocks(c)

	s.trackedObjectStore = NewMockTrackedObjectStore(ctrl)
	s.controllerMetadataService = NewMockMetadataService(ctrl)
	s.modelMetadataService = NewMockMetadataService(ctrl)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(testing.FakeControllerConfig(), nil).AnyTimes()

	s.modelMetadataServiceGetter = NewMockMetadataServiceGetter(ctrl)
	s.modelMetadataServiceGetter.EXPECT().ForModelUUID(gomock.Any()).Return(s.modelMetadataService).AnyTimes()

	s.modelService.EXPECT().WatchModel(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[struct{}], error) {
		return watchertest.NewMockNotifyWatcher(make(chan struct{})), nil
	}).AnyTimes()

	s.modelServices = NewMockModelServices(ctrl)
	s.modelServices.EXPECT().ModelService().Return(s.modelService).AnyTimes()

	s.modelServiceGetter = NewMockModelServiceGetter(ctrl)
	s.modelServiceGetter.EXPECT().ForModelUUID(gomock.Any()).Return(s.modelServices).AnyTimes()

	s.modelClaimGetter = NewMockModelClaimGetter(ctrl)
	s.modelClaimGetter.EXPECT().ForModelUUID(gomock.Any()).Return(s.claimer, nil).AnyTimes()

	s.objectStoreService.EXPECT().GetActiveObjectStoreBackend(gomock.Any()).Return(objectstoreservice.BackendInfo{
		Type: objectstore.FileBackend,
	}, nil).AnyTimes()

	c.Cleanup(func() {
		s.trackedObjectStore = nil
		s.controllerMetadataService = nil
		s.modelMetadataServiceGetter = nil
		s.modelServiceGetter = nil
		s.modelClaimGetter = nil
		s.modelMetadataService = nil
		s.modelServices = nil
	})

	return ctrl
}

func (s *workerSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for startup")
	}
}

func assertWait(c *tc.C, wait func()) {
	done := make(chan struct{})

	go func() {
		defer close(done)
		wait()
	}()

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting")
	}
}
