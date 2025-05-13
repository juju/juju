// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	states        chan string
	trackedTracer *MockTrackedTracer
	called        int64
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) TestKilledGetTracerErrDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	worker := w.(*tracerWorker)
	_, err := worker.GetTracer(context.Background(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIs, coretrace.ErrTracerDying)
}

func (s *workerSuite) TestGetTracer(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	s.trackedTracer.EXPECT().Start(gomock.Any(), "foo")

	tracer.Start(context.Background(), "foo")

	close(done)
}

func (s *workerSuite) TestGetTracerIsCached(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
	}

	close(done)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))
}

func (s *workerSuite) TestGetTracerIsNotCachedForDifferentNamespaces(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
	}

	close(done)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))
}

func (s *workerSuite) TestGetTracerConcurrently(c *tc.C) {
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
			c.Assert(err, tc.ErrorIsNil)
		}(i)
	}

	assertWait(c, wg.Wait)
	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))

	close(done)
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Clock:    s.clock,
		Logger:   s.logger,
		Endpoint: "https://meshuggah.com",
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
			atomic.AddInt64(&s.called, 1)
			return s.trackedTracer, nil
		},
		Tag:  names.NewMachineTag("0"),
		Kind: coretrace.KindController,
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	s.states = make(chan string)
	atomic.StoreInt64(&s.called, 0)

	ctrl := s.baseSuite.setupMocks(c)

	s.trackedTracer = NewMockTrackedTracer(ctrl)
	s.trackedTracer.EXPECT().Enabled().Return(true).AnyTimes()

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
