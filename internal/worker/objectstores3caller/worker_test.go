// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	"context"
	"sync/atomic"
	stdtesting "testing"
	time "time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
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

	info := objectstoreservice.BackendInfo{
		UUID:            "backend-uuid",
		ObjectStoreType: objectstore.S3Backend,
	}

	s.expectClock()
	s.expectActiveObjectStoreBackend(c, info)
	s.expectObjectStoreBackendWatch(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	info := objectstoreservice.BackendInfo{
		UUID:            "backend-uuid",
		ObjectStoreType: objectstore.S3Backend,
	}

	s.expectClock()
	s.expectActiveObjectStoreBackend(c, info)
	s.expectObjectStoreBackendWatch(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	var session objectstore.Session
	err := worker.Session(c.Context(), func(context.Context, objectstore.Session) error {
		session = s.session
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(session, tc.Equals, s.session)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionIsRetried(c *tc.C) {
	defer s.setupMocks(c).Finish()

	info := objectstoreservice.BackendInfo{
		UUID:            "backend-uuid",
		ObjectStoreType: objectstore.S3Backend,
	}

	s.expectClock()
	s.expectTimeAfter()
	s.expectActiveObjectStoreBackend(c, info)
	s.expectObjectStoreBackendWatch(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	var attempt int
	var session objectstore.Session
	err := worker.Session(c.Context(), func(context.Context, objectstore.Session) error {
		session = s.session

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

	info := objectstoreservice.BackendInfo{
		UUID:            "backend-uuid",
		ObjectStoreType: objectstore.S3Backend,
	}

	s.expectClock()
	s.expectTimeAfter()
	s.expectActiveObjectStoreBackend(c, info)
	s.expectObjectStoreBackendWatch(c)

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

	info := objectstoreservice.BackendInfo{
		UUID:            "backend-uuid",
		ObjectStoreType: objectstore.S3Backend,
	}

	changes := make(chan []string)

	s.expectClock()
	s.expectTimeAfter()
	s.expectActiveObjectStoreBackend(c, info)
	s.expectObjectStoreBackendWatchWithChanges(c, changes)

	// Trigger change will send a change to the watcher and wait for the
	// worker to process it.
	triggerChange := func() {
		triggerDone := make(chan struct{})

		// Now wait for the change to be processed and a new controller config
		// to be fetched.
		s.expectControllerConfigWithDone(c, info, triggerDone)

		// Send a change to the watcher.
		go func() {
			select {
			case changes <- []string{"backend-uuid"}:
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out sending change")
			}
		}()

		select {
		case <-triggerDone:
		case <-time.After(testing.LongWait):
		}
	}

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	// Wait for the initial startup to complete.
	s.sendInitialChange(c, changes)
	s.ensureStartup(c)

	var attempt int
	err := worker.Session(c.Context(), func(ctx context.Context, session objectstore.Session) error {
		attempt++
		if attempt == 1 {
			triggerChange()
			return errors.Forbiddenf("try again")
		}
		return nil
	})

	s.ensureClientUpdated(c)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(attempt, tc.Equals, 2)

	// Ensure we called new client twice.
	c.Check(atomic.LoadInt64(&s.sessionRefCount), tc.Equals, int64(2))

	workertest.CleanKill(c, worker)
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
		Clock:  s.clock,
	}
}

func (s *workerSuite) expectControllerConfigWithDone(c *tc.C, info objectstoreservice.BackendInfo, done chan struct{}) {
	s.objectStoreService.EXPECT().GetActiveObjectStoreBackend(gomock.Any()).DoAndReturn(func(context.Context) (objectstoreservice.BackendInfo, error) {
		defer close(done)
		return info, nil
	})
}

func (s *workerSuite) ensureClientUpdated(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateClientUpdated)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}
