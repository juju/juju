// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	time "time"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	states chan string
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) TestNewWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := auditlog.Config{}

	s.expectControllerConfigWatcher(c)

	worker, err := s.newWorker(cfg, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestNewWorkerUpdatedCurrentConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := auditlog.Config{}

	ch := s.expectControllerConfigWatcher(c)

	controllerConfig := testing.FakeControllerConfig()
	controllerConfig[controller.AuditingEnabled] = true
	controllerConfig[controller.AuditLogCaptureArgs] = true
	controllerConfig[controller.AuditLogMaxSize] = "10MB"
	controllerConfig[controller.AuditLogMaxBackups] = 5
	controllerConfig[controller.AuditLogExcludeMethods] = "foo,bar"
	s.expectControllerConfigWithConfig(controllerConfig)

	worker, err := s.newWorker(cfg, func(c auditlog.Config) auditlog.AuditLog {
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	s.ensureStartup(c)

	select {
	case ch <- []string{}:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out seeding initial event")
	}

	s.ensureChanged(c)

	current := worker.CurrentConfig()
	c.Assert(current, tc.DeepEquals, auditlog.Config{
		Enabled:        true,
		CaptureAPIArgs: true,
		MaxSizeMB:      10,
		MaxBackups:     5,
		ExcludeMethods: set.NewStrings("foo", "bar"),
	})

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) newWorker(initial auditlog.Config, logFactory AuditLogFactory) (*updater, error) {
	return newWorker(s.controllerConfigService, initial, logFactory, s.states)
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	return s.baseSuite.setupMocks(c)
}

func (s *workerSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *workerSuite) ensureChanged(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateChanged)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *workerSuite) expectControllerConfigWatcher(c *tc.C) chan []string {
	ch := make(chan []string)
	// Seed the initial event.
	go func() {
		select {
		case ch <- []string{}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out seeding initial event")
		}
	}()

	watcher := watchertest.NewMockStringsWatcher(ch)

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(watcher, nil)

	return ch
}
