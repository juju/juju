// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite

	states             chan string
	trackedObjectStore *MockTrackedObjectStore
	called             int64
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestKilledGetObjectStoreErrDying(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	worker := w.(*objectStoreWorker)
	_, err := worker.GetObjectStore(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, coreobjectstore.ErrObjectStoreDying)
}

func (s *workerSuite) TestGetObjectStore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedObjectStore.EXPECT().Kill().AnyTimes()
	s.trackedObjectStore.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*objectStoreWorker)
	objectStore, err := worker.GetObjectStore(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(objectStore, gc.NotNil)

	close(done)
}

func (s *workerSuite) TestGetObjectStoreIsCached(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedObjectStore.EXPECT().Kill().AnyTimes()
	s.trackedObjectStore.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*objectStoreWorker)
	for i := 0; i < 10; i++ {
		_, err := worker.GetObjectStore(context.Background(), "foo")
		c.Assert(err, jc.ErrorIsNil)
	}

	close(done)

	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(1))
}

func (s *workerSuite) TestGetObjectStoreIsNotCachedForDifferentNamespaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedObjectStore.EXPECT().Kill().AnyTimes()
	s.trackedObjectStore.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*objectStoreWorker)
	for i := 0; i < 10; i++ {
		_, err := worker.GetObjectStore(context.Background(), fmt.Sprintf("anything-%d", i))
		c.Assert(err, jc.ErrorIsNil)
	}

	close(done)

	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(10))
}

func (s *workerSuite) TestGetObjectStoreConcurrently(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedObjectStore.EXPECT().Kill().AnyTimes()
	s.trackedObjectStore.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	var wg sync.WaitGroup
	wg.Add(10)

	worker := w.(*objectStoreWorker)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := worker.GetObjectStore(context.Background(), fmt.Sprintf("anything-%d", i))
			c.Assert(err, jc.ErrorIsNil)
		}(i)
	}

	assertWait(c, wg.Wait)
	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(10))

	close(done)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Clock:  s.clock,
		Logger: s.logger,
		NewObjectStoreWorker: func(context.Context, string, MongoSession, Logger) (TrackedObjectStore, error) {
			atomic.AddInt64(&s.called, 1)
			return s.trackedObjectStore, nil
		},
	}, s.states)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	s.states = make(chan string)
	atomic.StoreInt64(&s.called, 0)

	ctrl := s.baseSuite.setupMocks(c)

	s.trackedObjectStore = NewMockTrackedObjectStore(ctrl)

	return ctrl
}

func (s *workerSuite) ensureStartup(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func assertWait(c *gc.C, wait func()) {
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
