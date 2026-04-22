// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	"context"
	"sync/atomic"
	stdtesting "testing"
	time "time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	sessionRefCount int64
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestCleanKill(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackend(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestCleanKillFileBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetActiveBackendFile(c)
	s.expectWatchObjectStoreBackend(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackend(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	var session objectstore.Session
	err := worker.Session(c.Context(), func(_ context.Context, sess objectstore.Session) error {
		session = sess
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(session, tc.Equals, s.session)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionNotSupportedForFileBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetActiveBackendFile(c)
	s.expectWatchObjectStoreBackend(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	err := worker.Session(c.Context(), func(context.Context, objectstore.Session) error {
		c.Fatalf("unexpected call to Session")
		return nil
	})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionIsRetried(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackend(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	var attempt int
	var session objectstore.Session
	err := worker.Session(c.Context(), func(_ context.Context, sess objectstore.Session) error {
		session = sess

		attempt++
		if attempt == 1 {
			return errors.Forbiddenf("not today")
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(session, tc.Equals, s.session)
	c.Check(attempt, tc.Equals, 2)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionIsNotRetried(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackend(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	var attempt int
	err := worker.Session(c.Context(), func(context.Context, objectstore.Session) error {
		attempt++
		return errors.Errorf("boom")
	})
	c.Assert(err, tc.ErrorMatches, `boom`)
	c.Check(attempt, tc.Equals, 1)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionIsChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string)

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackendWithChanges(changes)

	triggerDone := make(chan struct{})
	// Now wait for the change to be processed and a new backend
	// to be fetched.
	s.expectGetActiveBackendS3WithDone(triggerDone)

	// triggerChange sends a change to the watcher and waits for the
	// worker to process it.
	triggerChange := func() {
		// Send a change to the watcher.
		go func() {
			select {
			case changes <- []string{"backend-changed"}:
			case <-c.Context().Done():
				c.Fatalf("timed out sending change")
			}
		}()

		select {
		case <-triggerDone:
		case <-c.Context().Done():
		}
	}

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	// Wait for the initial startup to complete.
	s.ensureStartup(c)

	var attempt atomic.Int32
	err := worker.Session(c.Context(), func(ctx context.Context, session objectstore.Session) error {
		if attempt.Add(1) == 1 {
			triggerChange()
			return errors.Forbiddenf("try again")
		}
		return nil
	})

	s.ensureClientUpdated(c)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(attempt.Load(), tc.Equals, int32(2))

	// Ensure we called new client twice.
	c.Check(atomic.LoadInt64(&s.sessionRefCount), tc.Equals, int64(2))

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionIsNotChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string)

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackendWithChanges(changes)

	triggerDone := make(chan struct{})
	// The watcher fires, and the worker will re-fetch the backend.
	s.expectGetActiveBackendFileWithDone(triggerDone)

	// triggerChange sends a change to the watcher and waits for the
	// worker to process it. This sends a generic change that triggers
	// the worker to re-fetch the backend info.
	triggerChange := func() {
		// Send a change to the watcher.
		go func() {
			select {
			case changes <- []string{"something-changed"}:
			case <-c.Context().Done():
				c.Fatalf("timed out sending change")
			}
		}()

		select {
		case <-triggerDone:
		case <-c.Context().Done():
		}
	}

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	// Wait for the initial startup to complete.
	s.ensureStartup(c)

	// Done is to ensure we actively retry all the scenario before allowing
	// the test to finish.
	done := make(chan struct{})

	var attempt atomic.Int32
	err := worker.Session(c.Context(), func(ctx context.Context, session objectstore.Session) error {
		if attempt.Add(1) == 1 {
			triggerChange()
			return errors.Forbiddenf("try again")
		}

		// We're done, the test can complete.
		defer close(done)

		return nil
	})

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for session retry to complete")
	}

	c.Assert(err, tc.ErrorIsNil)
	c.Check(attempt.Load(), tc.Equals, int32(2))

	// The client was refreshed since the watcher fired.
	c.Check(atomic.LoadInt64(&s.sessionRefCount), tc.Equals, int64(1))

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestGetActiveBackendError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStoreService.EXPECT().GetActiveObjectStoreBackend(gomock.Any()).
		Return(objectstoreservice.BackendInfo{}, errors.Errorf("backend error"))

	_, err := newWorker(s.getConfig(), s.states)
	c.Assert(err, tc.ErrorMatches, `backend error`)
}

func (s *workerSuite) TestWatchObjectStoreBackendError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetActiveBackendS3(c)
	s.objectStoreService.EXPECT().WatchObjectStoreBackend(gomock.Any()).
		Return(nil, errors.Errorf("watch error"))

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	err := workertest.CheckKill(c, worker)
	c.Assert(err, tc.ErrorMatches, `watch error`)
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	atomic.StoreInt64(&s.sessionRefCount, 0)
	return s.baseSuite.setupMocks(c)
}

func (s *workerSuite) newWorker(c *tc.C) *s3Worker {
	worker, err := newWorker(s.getConfig(), s.states)
	c.Assert(err, tc.ErrorIsNil)
	return worker
}

func (s *workerSuite) getConfig() workerConfig {
	return workerConfig{
		ObjectStoreService: s.objectStoreService,
		HTTPClient:         s.httpClient,
		NewClient: func(string, s3client.HTTPClient, s3client.Credentials, logger.Logger) (objectstore.Session, error) {
			atomic.AddInt64(&s.sessionRefCount, 1)
			return s.session, nil
		},
		Logger: s.logger,
		Clock:  clock.WallClock,
	}
}

func (s *workerSuite) expectGetActiveBackendS3WithDone(done chan struct{}) {
	s.objectStoreService.EXPECT().GetActiveObjectStoreBackend(gomock.Any()).
		DoAndReturn(func(context.Context) (objectstoreservice.BackendInfo, error) {
			defer close(done)
			return objectstoreservice.BackendInfo{
				Type:      "s3",
				Endpoint:  new("https://s3.example.com"),
				AccessKey: new("access-key"),
				SecretKey: new("secret-key"),
			}, nil
		})
}

func (s *workerSuite) expectGetActiveBackendFileWithDone(done chan struct{}) {
	s.objectStoreService.EXPECT().GetActiveObjectStoreBackend(gomock.Any()).
		DoAndReturn(func(context.Context) (objectstoreservice.BackendInfo, error) {
			defer close(done)
			return objectstoreservice.BackendInfo{
				Type: "file",
			}, nil
		})
}

func (s *workerSuite) expectWatchObjectStoreBackendWithChanges(changes <-chan []string) {
	s.objectStoreService.EXPECT().WatchObjectStoreBackend(gomock.Any()).
		DoAndReturn(s.baseSuite.watchObjectStoreBackendWithChanges(changes))
}

func (s *workerSuite) ensureClientUpdated(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateClientUpdated)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}
