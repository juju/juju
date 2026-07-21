// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	"context"
	"sync/atomic"
	stdtesting "testing"
	time "time"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	sessionRefCount int64
}

func TestWorkerSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &workerSuite{})
	})
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

func (s *workerSuite) TestSessionForbiddenIsNotRetried(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackend(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	err := worker.Session(c.Context(), func(context.Context, objectstore.Session) error {
		return errors.Forbiddenf("not today")
	})
	c.Assert(err, tc.ErrorIs, errors.Forbidden)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionErrorPropagated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackend(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	err := worker.Session(c.Context(), func(context.Context, objectstore.Session) error {
		return errors.Errorf("boom")
	})
	c.Assert(err, tc.ErrorMatches, `boom`)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionIsChangedByWatcher(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string)

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackendWithChanges(changes)

	triggerDone := make(chan struct{})
	s.expectGetActiveBackendS3WithDone(triggerDone)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	// Wait for the initial startup to complete.
	s.ensureStartup(c)

	// Verify the initial session works.
	err := worker.Session(c.Context(), func(_ context.Context, sess objectstore.Session) error {
		c.Check(sess, tc.Equals, s.session)
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Send a change to the watcher.
	select {
	case changes <- []string{"backend-changed"}:
	case <-c.Context().Done():
		c.Fatalf("timed out sending change")
	}

	// Wait for the worker to process the change.
	select {
	case <-triggerDone:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for backend refresh")
	}

	s.ensureClientUpdated(c)

	// Session should still work after the change.
	err = worker.Session(c.Context(), func(_ context.Context, sess objectstore.Session) error {
		c.Check(sess, tc.Equals, s.session)
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Two clients created: initial + watcher refresh.
	c.Check(atomic.LoadInt64(&s.sessionRefCount), tc.Equals, int64(2))

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionBecomesNilOnFileBackendChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string)

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackendWithChanges(changes)

	triggerDone := make(chan struct{})
	// After the watcher fires, the backend returns file (no S3 creds).
	s.expectGetActiveBackendFileWithDone(triggerDone)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	// Initially session exists.
	err := worker.Session(c.Context(), func(_ context.Context, sess objectstore.Session) error {
		c.Check(sess, tc.NotNil)
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Send a change to the watcher.
	select {
	case changes <- []string{"backend-changed"}:
	case <-c.Context().Done():
		c.Fatalf("timed out sending change")
	}

	select {
	case <-triggerDone:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for backend refresh")
	}

	s.ensureClientUpdated(c)

	// Now session should be nil (file backend), returning NotSupported.
	err = worker.Session(c.Context(), func(context.Context, objectstore.Session) error {
		c.Fatalf("unexpected call to Session")
		return nil
	})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionStableWhileFnInFlight(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string)

	s.expectGetActiveBackendS3(c)
	s.expectWatchObjectStoreBackendWithChanges(changes)

	triggerDone := make(chan struct{})
	// After the watcher fires, the backend returns file (session becomes nil).
	s.expectGetActiveBackendFileWithDone(triggerDone)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	// Capture the session before spawning the goroutine to avoid
	// racing with the cleanup function that nils out s.session.
	originalSession := s.session

	// Call Session, and while fn is running, trigger a backend change
	// that sets the session to nil. The in-flight fn should still see
	// the original non-nil session.
	fnStarted := make(chan struct{})
	fnDone := make(chan struct{})

	go func() {
		err := worker.Session(c.Context(), func(_ context.Context, sess objectstore.Session) error {
			// Signal that fn has started and captured a session.
			close(fnStarted)
			c.Check(sess, tc.NotNil)

			// Wait for the backend change to complete before returning.
			select {
			case <-fnDone:
			case <-c.Context().Done():
			}

			// The session we captured is still valid, even though
			// the worker's session is now nil.
			c.Check(sess, tc.Equals, originalSession)
			return nil
		})
		c.Check(err, tc.ErrorIsNil)
	}()

	// Wait for fn to be in-flight.
	select {
	case <-fnStarted:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for fn to start")
	}

	// Trigger the backend change while fn is running.
	select {
	case changes <- []string{"backend-changed"}:
	case <-c.Context().Done():
		c.Fatalf("timed out sending change")
	}

	// Wait for the loop to process and set session to nil.
	select {
	case <-triggerDone:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for backend refresh")
	}

	s.ensureClientUpdated(c)

	// Now a new Session call should get NotSupported (nil session).
	err := worker.Session(c.Context(), func(context.Context, objectstore.Session) error {
		c.Fatalf("unexpected call")
		return nil
	})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)

	// Let the in-flight fn complete.
	close(fnDone)

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
		NewClient: func(string, s3client.HTTPClient, s3client.Credentials, string, logger.Logger) (objectstore.Session, error) {
			atomic.AddInt64(&s.sessionRefCount, 1)
			return s.session, nil
		},
		Logger: s.logger,
	}
}

func (s *workerSuite) expectGetActiveBackendS3WithDone(done chan struct{}) {
	endpoint := "https://s3.example.com"
	region := "us-east-1"
	accessKey := "access-key"
	secretKey := "secret-key"
	s.objectStoreService.EXPECT().GetActiveObjectStoreBackend(gomock.Any()).
		DoAndReturn(func(context.Context) (objectstoreservice.BackendInfo, error) {
			defer close(done)
			return objectstoreservice.BackendInfo{
				Type:      "s3",
				Region:    &region,
				Endpoint:  &endpoint,
				AccessKey: &accessKey,
				SecretKey: &secretKey,
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
