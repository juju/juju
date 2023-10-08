// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite

	states        chan string
	trackedTracer *MockTrackedTracer
	called        int64
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestKilledGetTracerErrDying(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	worker := w.(*tracerWorker)
	_, err := worker.GetTracer(context.Background(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, jc.ErrorIs, coretrace.ErrTracerDying)
}

func (s *workerSuite) TestGetTracer(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*tracerWorker)
	tracer, err := worker.GetTracer(context.Background(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, jc.ErrorIsNil)

	s.trackedTracer.EXPECT().Start(gomock.Any(), "foo")

	tracer.Start(context.Background(), "foo")

	close(done)
}

func (s *workerSuite) TestGetTracerIsCached(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*tracerWorker)
	for i := 0; i < 10; i++ {
		_, err := worker.GetTracer(context.Background(), coretrace.Namespace("agent", "anything"))
		c.Assert(err, jc.ErrorIsNil)
	}

	close(done)

	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(1))
}

func (s *workerSuite) TestGetTracerIsNotCachedForDifferentNamespaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*tracerWorker)
	for i := 0; i < 10; i++ {
		_, err := worker.GetTracer(context.Background(), coretrace.Namespace("agent", fmt.Sprintf("anything-%d", i)))
		c.Assert(err, jc.ErrorIsNil)
	}

	close(done)

	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(10))
}

func (s *workerSuite) TestGetTracerConcurrently(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	var wg sync.WaitGroup
	wg.Add(10)

	worker := w.(*tracerWorker)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := worker.GetTracer(context.Background(), coretrace.Namespace("agent", fmt.Sprintf("anything-%d", i)))
			c.Assert(err, jc.ErrorIsNil)
		}(i)
	}

	assertWait(c, wg.Wait)
	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(10))

	close(done)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Clock:    s.clock,
		Logger:   s.logger,
		Endpoint: "https://meshuggah.com",
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, bool, bool, Logger, NewClientFunc) (TrackedTracer, error) {
			atomic.AddInt64(&s.called, 1)
			return s.trackedTracer, nil
		},
		Tag: names.NewMachineTag("0"),
	}, s.states)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	s.states = make(chan string)
	atomic.StoreInt64(&s.called, 0)

	ctrl := s.baseSuite.setupMocks(c)

	s.trackedTracer = NewMockTrackedTracer(ctrl)
	s.trackedTracer.EXPECT().Enabled().Return(true).AnyTimes()

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
