// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"context"
	"sync"
	"sync/atomic"
	time "time"

	clock "github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestWorkerConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	c.Assert(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.newConfig(c)
	cfg.ApplicationService = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.ModelConfigService = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.HTTPClientGetter = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewHTTPClient = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewDownloader = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Logger = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Clock = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) TestWorkerStart(c *gc.C) {
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

func (s *workerSuite) TestWorkerCreatesAsyncWorker(c *gc.C) {
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

func (s *workerSuite) TestWorkerCreatesAsyncWorkerWithSameAppID(c *gc.C) {
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
	c.Assert(atomic.LoadInt64(&called), gc.Equals, int64(1))

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerCreatesAsyncWorkerWithSameAppIDOverTwoChangeSet(c *gc.C) {
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
	c.Assert(atomic.LoadInt64(&called), gc.Equals, int64(1))

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerCreatesAsyncWorkerWithDifferentAppID(c *gc.C) {
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

	c.Assert(atomic.LoadInt64(&called), gc.Equals, int64(3))

	workertest.CleanKill(c, w)
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	return ctrl
}

func (s *workerSuite) newWorker(c *gc.C) *Worker {
	w, err := newWorker(s.newConfig(c), s.states)

	c.Assert(err, jc.ErrorIsNil)

	return w
}

func (s *workerSuite) newConfig(c *gc.C) Config {
	return Config{
		ApplicationService: s.applicationService,
		ModelConfigService: s.modelConfigService,
		HTTPClientGetter:   s.httpClientGetter,
		NewHTTPClient: func(ctx context.Context, hg http.HTTPClientGetter) (http.HTTPClient, error) {
			return hg.GetHTTPClient(ctx, http.CharmhubPurpose)
		},
		NewDownloader: func(charmhub.HTTPClient, ModelConfigService, logger.Logger) Downloader {
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

func (s *workerSuite) ensureStartup(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}
