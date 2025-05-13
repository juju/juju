// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"context"
	"sync"
	"sync/atomic"
	time "time"

	clock "github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/charmhub"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	states         chan string
	newAsyncWorker func() worker.Worker
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) TestWorkerConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	c.Assert(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.newConfig(c)
	cfg.ApplicationService = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.HTTPClientGetter = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewHTTPClient = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewDownloader = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Logger = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Clock = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) TestWorkerStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string)

	done := make(chan struct{})
	s.applicationService.EXPECT().WatchApplicationsWithPendingCharms(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		close(done)
		return watchertest.NewMockStringsWatcher(changes), nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerCreatesAsyncWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)

	changes := make(chan []string)

	done := make(chan struct{})
	s.applicationService.EXPECT().WatchApplicationsWithPendingCharms(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(changes), nil
	})
	s.httpClientGetter.EXPECT().GetHTTPClient(gomock.Any(), http.CharmhubPurpose).Return(s.httpClient, nil)

	s.newAsyncWorker = func() worker.Worker {
		close(done)
		return workertest.NewErrorWorker(nil)
	}

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	go func() {
		select {
		case changes <- []string{appID.String()}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for worker to finish")
		}
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerCreatesAsyncWorkerWithSameAppID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Using the same App ID should not cause a new worker to be created.

	appID := applicationtesting.GenApplicationUUID(c)

	changes := make(chan []string)

	s.applicationService.EXPECT().WatchApplicationsWithPendingCharms(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(changes), nil
	})
	s.httpClientGetter.EXPECT().GetHTTPClient(gomock.Any(), http.CharmhubPurpose).Return(s.httpClient, nil)

	done := make(chan struct{})

	var do sync.Once
	var called int64
	s.newAsyncWorker = func() worker.Worker {
		do.Do(func() {
			close(done)
		})

		atomic.AddInt64(&called, 1)
		return workertest.NewErrorWorker(nil)
	}

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	sent := make(chan struct{})
	go func() {
		defer close(sent)

		select {
		case changes <- []string{appID.String(), appID.String()}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for worker to finish")
		}
	}()

	// Ensure that we've sent all the changes.

	select {
	case <-sent:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for messages to be sent")
	}

	// Ensure that we've at least called the new worker once.

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for new worker to be called")
	}

	// Wait for a bit to ensure that we're not creating a new worker.

	<-time.After(time.Millisecond * 500)
	c.Assert(atomic.LoadInt64(&called), tc.Equals, int64(1))

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerCreatesAsyncWorkerWithSameAppIDOverTwoChangeSet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Using the same App ID should not cause a new worker to be created.

	appID := applicationtesting.GenApplicationUUID(c)

	changes := make(chan []string)

	s.applicationService.EXPECT().WatchApplicationsWithPendingCharms(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(changes), nil
	})
	s.httpClientGetter.EXPECT().GetHTTPClient(gomock.Any(), http.CharmhubPurpose).Return(s.httpClient, nil).Times(2)

	var called int64
	s.newAsyncWorker = func() worker.Worker {
		atomic.AddInt64(&called, 1)
		return workertest.NewErrorWorker(nil)
	}

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	sent := make(chan struct{})
	go func() {
		defer close(sent)

		select {
		case changes <- []string{appID.String()}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for worker to finish")
		}

		select {
		case changes <- []string{appID.String()}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for worker to finish")
		}
	}()

	// Ensure that we've sent all the changes.

	select {
	case <-sent:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for messages to be sent")
	}

	// Wait for a bit to ensure that we're not creating a new worker.

	<-time.After(time.Millisecond * 500)
	c.Assert(atomic.LoadInt64(&called), tc.Equals, int64(1))

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerCreatesAsyncWorkerWithDifferentAppID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Using the same App ID should not cause a new worker to be created.

	var apps [3]application.ID
	for i := range apps {
		apps[i] = applicationtesting.GenApplicationUUID(c)
	}

	changes := make(chan []string)

	s.applicationService.EXPECT().WatchApplicationsWithPendingCharms(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(changes), nil
	})
	s.httpClientGetter.EXPECT().GetHTTPClient(gomock.Any(), http.CharmhubPurpose).Return(s.httpClient, nil).Times(2)

	done := make(chan struct{})

	var called int64
	s.newAsyncWorker = func() worker.Worker {
		v := atomic.AddInt64(&called, 1)
		if v == 3 {
			close(done)
		}

		return workertest.NewErrorWorker(nil)
	}

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	go func() {
		select {
		case changes <- []string{apps[0].String(), apps[1].String()}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for worker to finish")
		}

		select {
		case changes <- []string{apps[2].String()}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for worker to finish")
		}
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for new worker to be called")
	}

	c.Assert(atomic.LoadInt64(&called), tc.Equals, int64(3))

	workertest.CleanKill(c, w)
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	return ctrl
}

func (s *workerSuite) newWorker(c *tc.C) *Worker {
	w, err := newWorker(s.newConfig(c), s.states)

	c.Assert(err, tc.ErrorIsNil)

	return w
}

func (s *workerSuite) newConfig(c *tc.C) Config {
	return Config{
		ApplicationService: s.applicationService,
		HTTPClientGetter:   s.httpClientGetter,
		NewHTTPClient: func(ctx context.Context, hg http.HTTPClientGetter) (http.HTTPClient, error) {
			return hg.GetHTTPClient(ctx, http.CharmhubPurpose)
		},
		NewDownloader: func(charmhub.HTTPClient, logger.Logger) Downloader {
			return s.downloader
		},
		NewAsyncDownloadWorker: func(appID application.ID, applicationService ApplicationService, downloader Downloader, clock clock.Clock, logger logger.Logger) worker.Worker {
			if s.newAsyncWorker == nil {
				return workertest.NewErrorWorker(nil)
			}
			return s.newAsyncWorker()
		},
		Clock:  s.clock,
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *workerSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}
