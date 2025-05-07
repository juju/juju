// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageregistry

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	states        chan string
	trackedWorker *MockStorageRegistryWorker
	called        int64
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) TestKilledGetStorageRegistryErrDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	worker := w.(*storageRegistryWorker)
	_, err := worker.GetStorageRegistry(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, corestorage.ErrStorageRegistryDying)
}

func (s *workerSuite) TestGetStorageRegistry(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").Return(providerTrackerProvider{}, nil)

	done := make(chan struct{})
	s.trackedWorker.EXPECT().Kill().AnyTimes()
	s.trackedWorker.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*storageRegistryWorker)
	storageRegistry, err := worker.GetStorageRegistry(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(storageRegistry, tc.NotNil)

	close(done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestGetStorageRegistryIsCached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").DoAndReturn(func(context.Context, string) (providertracker.Provider, error) {
		atomic.AddInt64(&s.called, 1)
		return providerTrackerProvider{}, nil
	})

	done := make(chan struct{})
	s.trackedWorker.EXPECT().Kill().AnyTimes()
	s.trackedWorker.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*storageRegistryWorker)
	for i := 0; i < 10; i++ {

		_, err := worker.GetStorageRegistry(context.Background(), "foo")
		c.Assert(err, jc.ErrorIsNil)
	}

	close(done)

	workertest.CleanKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))
}

func (s *workerSuite) TestGetStorageRegistryIsNotCachedForDifferentNamespaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedWorker.EXPECT().Kill().AnyTimes()
	s.trackedWorker.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*storageRegistryWorker)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("anything-%d", i)

		s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), name).DoAndReturn(func(context.Context, string) (providertracker.Provider, error) {
			atomic.AddInt64(&s.called, 1)
			return providerTrackerProvider{}, nil
		})

		_, err := worker.GetStorageRegistry(context.Background(), name)
		c.Assert(err, jc.ErrorIsNil)
	}

	close(done)

	workertest.CleanKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(10))
}

func (s *workerSuite) TestGetStorageRegistryConcurrently(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedWorker.EXPECT().Kill().AnyTimes()
	s.trackedWorker.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	var wg sync.WaitGroup
	wg.Add(10)

	worker := w.(*storageRegistryWorker)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()

			name := fmt.Sprintf("anything-%d", i)

			s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), name).DoAndReturn(func(context.Context, string) (providertracker.Provider, error) {
				atomic.AddInt64(&s.called, 1)
				return providerTrackerProvider{}, nil
			})

			_, err := worker.GetStorageRegistry(context.Background(), name)
			c.Assert(err, jc.ErrorIsNil)
		}(i)
	}

	assertWait(c, wg.Wait)
	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(10))

	close(done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Clock:           s.clock,
		Logger:          s.logger,
		ProviderFactory: s.providerFactory,
		NewStorageRegistryWorker: func(storage.ProviderRegistry) (worker.Worker, error) {
			return s.trackedWorker, nil
		},
	}, s.states)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)
	atomic.StoreInt64(&s.called, 0)

	ctrl := s.baseSuite.setupMocks(c)

	s.trackedWorker = NewMockStorageRegistryWorker(ctrl)

	return ctrl
}

func (s *workerSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
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
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting")
	}
}

type providerTrackerProvider struct {
	providertracker.Provider
}
