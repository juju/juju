// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	"context"
	"sync/atomic"
	time "time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"

	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	sessionRefCount int64
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) TestCleanKill(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := testing.FakeControllerConfig()
	config[controller.ObjectStoreType] = string(objectstore.S3Backend)

	s.expectClock()
	s.expectControllerConfig(c, config)
	s.expectControllerConfigWatch(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := testing.FakeControllerConfig()
	config[controller.ObjectStoreType] = string(objectstore.S3Backend)

	s.expectClock()
	s.expectControllerConfig(c, config)
	s.expectControllerConfigWatch(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	var session objectstore.Session
	err := worker.Session(context.Background(), func(context.Context, objectstore.Session) error {
		session = s.session
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(session, tc.Equals, s.session)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionIsRetried(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := testing.FakeControllerConfig()
	config[controller.ObjectStoreType] = string(objectstore.S3Backend)

	s.expectClock()
	s.expectTimeAfter()
	s.expectControllerConfig(c, config)
	s.expectControllerConfigWatch(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	var attempt int
	var session objectstore.Session
	err := worker.Session(context.Background(), func(context.Context, objectstore.Session) error {
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

	config := testing.FakeControllerConfig()
	config[controller.ObjectStoreType] = string(objectstore.S3Backend)

	s.expectClock()
	s.expectTimeAfter()
	s.expectControllerConfig(c, config)
	s.expectControllerConfigWatch(c)

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	var attempt int
	err := worker.Session(context.Background(), func(context.Context, objectstore.Session) error {
		attempt++
		return errors.Errorf("boom")
	})
	c.Assert(err, tc.ErrorMatches, `boom`)
	c.Check(attempt, tc.Equals, 1)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSessionIsChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := testing.FakeControllerConfig()
	config[controller.ObjectStoreType] = string(objectstore.S3Backend)

	changes := make(chan []string)

	s.expectClock()
	s.expectTimeAfter()
	s.expectControllerConfig(c, config)
	s.expectControllerConfigWatchWithChanges(c, changes)

	// Trigger change will send a change to the watcher and wait for the
	// worker to process it.
	triggerChange := func() {
		triggerDone := make(chan struct{})

		// Now wait for the change to be processed and a new controller config
		// to be fetched.
		s.expectControllerConfigWithDone(c, config, triggerDone)

		// Send a change to the watcher.
		go func() {
			select {
			case changes <- []string{controller.ObjectStoreS3Endpoint}:
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
	err := worker.Session(context.Background(), func(ctx context.Context, session objectstore.Session) error {
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

func (s *workerSuite) TestSessionIsNotChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := testing.FakeControllerConfig()
	config[controller.ObjectStoreType] = string(objectstore.S3Backend)

	changes := make(chan []string)

	s.expectClock()
	s.expectTimeAfter()
	s.expectControllerConfig(c, config)
	s.expectControllerConfigWatchWithChanges(c, changes)

	// Trigger change will send a change to the watcher and wait for the
	// worker to process it.
	triggerChange := func() {
		triggerDone := make(chan struct{})

		// Send a change to the watcher.
		// Note: the change is not one we care about, so we don't expect the
		// controller config to be fetched.
		go func() {
			defer close(triggerDone)

			// Notice we're not sending a change we care about.

			select {
			case changes <- []string{controller.OpenTelemetryEnabled}:
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

	// Done is to ensure we actively retry all the scenario before allowing
	// the test to finish.
	done := make(chan struct{})

	var attempt int
	err := worker.Session(context.Background(), func(ctx context.Context, session objectstore.Session) error {
		attempt++
		if attempt == 1 {
			triggerChange()
			return errors.Forbiddenf("try again")
		}

		// We're done, the test can complete.
		defer close(done)

		return nil
	})

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for controller config watcher to be added")
	}

	c.Assert(err, tc.ErrorIsNil)
	c.Check(attempt, tc.Equals, 2)

	// The client wasn't refreshed, so we should still have the original client.
	c.Check(atomic.LoadInt64(&s.sessionRefCount), tc.Equals, int64(1))

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
		ControllerConfigService: s.controllerConfigService,
		HTTPClient:              s.httpClient,
		NewClient: func(string, s3client.HTTPClient, s3client.Credentials, logger.Logger) (objectstore.Session, error) {
			atomic.AddInt64(&s.sessionRefCount, 1)
			return s.session, nil
		},
		Logger: s.logger,
		Clock:  s.clock,
	}
}

func (s *workerSuite) expectControllerConfigWithDone(c *tc.C, config controller.Config, done chan struct{}) {
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (controller.Config, error) {
		defer close(done)
		return config, nil
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
