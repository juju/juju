// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

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

	corehttp "github.com/juju/juju/core/http"
	internalhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	newHTTPClient func() *internalhttp.Client

	states        chan string
	trackedWorker *MockHTTPClientWorker
	called        int64
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) TestKilledGetHTTPClientErrDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	worker := w.(*httpClientWorker)
	_, err := worker.GetHTTPClient(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, corehttp.ErrHTTPClientDying)
}

func (s *workerSuite) TestGetHTTPClient(c *tc.C) {
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

	worker := w.(*httpClientWorker)
	httpClient, err := worker.GetHTTPClient(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(httpClient, tc.NotNil)

	close(done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestGetHTTPClientIsCached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	s.newHTTPClient = func() *internalhttp.Client {
		atomic.AddInt64(&s.called, 1)
		return internalhttp.NewClient()
	}

	done := make(chan struct{})
	s.trackedWorker.EXPECT().Kill().AnyTimes()
	s.trackedWorker.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*httpClientWorker)
	for i := 0; i < 10; i++ {

		_, err := worker.GetHTTPClient(context.Background(), "foo")
		c.Assert(err, jc.ErrorIsNil)
	}

	close(done)

	workertest.CleanKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))
}

func (s *workerSuite) TestGetHTTPClientIsNotCachedForDifferentNamespaces(c *tc.C) {
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

	worker := w.(*httpClientWorker)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("anything-%d", i)

		s.newHTTPClient = func() *internalhttp.Client {
			atomic.AddInt64(&s.called, 1)
			return internalhttp.NewClient()
		}

		_, err := worker.GetHTTPClient(context.Background(), corehttp.Purpose(name))
		c.Assert(err, jc.ErrorIsNil)
	}

	close(done)

	workertest.CleanKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(10))
}

func (s *workerSuite) TestGetHTTPClientConcurrently(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	s.newHTTPClient = func() *internalhttp.Client {
		atomic.AddInt64(&s.called, 1)
		return internalhttp.NewClient()
	}

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

	worker := w.(*httpClientWorker)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()

			name := fmt.Sprintf("anything-%d", i)

			_, err := worker.GetHTTPClient(context.Background(), corehttp.Purpose(name))
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
		Clock:  s.clock,
		Logger: s.logger,
		NewHTTPClient: func(corehttp.Purpose, ...internalhttp.Option) *internalhttp.Client {
			if s.newHTTPClient == nil {
				return internalhttp.NewClient()
			}
			return s.newHTTPClient()
		},
		NewHTTPClientWorker: func(c *internalhttp.Client) (worker.Worker, error) {
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

	s.trackedWorker = NewMockHTTPClientWorker(ctrl)

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
