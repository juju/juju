// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainunitless "github.com/juju/juju/domain/unitless"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/scriptlet"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type refreshSuite struct {
	coretesting.BaseSuite

	scriptletService   *MockScriptletService
	statusService      *MockApplicationStatusService
	application        domainunitless.ScriptletApplication
	appChanges         chan []string
	appWatcher         *watchertest.MockStringsWatcher
	applicationChanges chan struct{}
	applicationWatcher *watchertest.MockNotifyWatcher
	eventWatchers      []*watchertest.MockStringsWatcher
	executorConfigs    chan ExecutorConfig
	handled            chan refreshedEvent
}

func TestRefreshSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &refreshSuite{})
	})
}

func (s *refreshSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	ctrl := gomock.NewController(c)
	s.scriptletService = NewMockScriptletService(ctrl)
	s.statusService = NewMockApplicationStatusService(ctrl)
	s.application = domainunitless.ScriptletApplication{
		UUID:      tc.Must(c, coreapplication.NewUUID),
		CharmUUID: tc.Must(c, corecharm.NewID),
		Name:      "test-app",
		Life:      life.Alive,
		Sources: []domainunitless.ScriptSource{{
			LoadPath: "hooks.star", Source: "first scriptlet",
		}},
	}
	s.appChanges = make(chan []string)
	s.appWatcher = watchertest.NewMockStringsWatcher(s.appChanges)
	s.scriptletService.EXPECT().WatchScriptletApplications(gomock.Any()).Return(s.appWatcher, nil)
	s.applicationChanges = make(chan struct{}, 1)
	s.applicationChanges <- struct{}{}
	s.applicationWatcher = watchertest.NewMockNotifyWatcher(s.applicationChanges)
	s.scriptletService.EXPECT().WatchScriptletApplication(gomock.Any(), s.application.UUID).Return(s.applicationWatcher, nil)
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(s.application, nil)
	s.eventWatchers = nil
	s.expectEventWatcher()
	s.executorConfigs = make(chan ExecutorConfig, 1)
	s.handled = make(chan refreshedEvent, 1)
}

func (s *refreshSuite) TestNormalCharmToScriptlet(c *tc.C) {
	w := s.startWorker(c)
	defer workertest.DirtyKill(c, w)

	// The initial notification contains no scriptlet applications. Refreshing
	// the normal charm causes its UUID to be emitted by the global watcher.
	s.appChanges <- []string{}
	s.appChanges <- []string{s.application.UUID.String()}
	s.assertRefreshed(c, s.application)

	workertest.CleanKill(c, w)
	s.assertWatchersStopped(c)
}

func (s *refreshSuite) TestScriptletCharmRefresh(c *tc.C) {
	w := s.startWorker(c)
	defer workertest.DirtyKill(c, w)
	s.appChanges <- []string{s.application.UUID.String()}
	s.assertRefreshed(c, s.application)
	application := s.application
	application.CharmUUID = tc.Must(c, corecharm.NewID)
	application.Sources = []domainunitless.ScriptSource{{
		LoadPath: "hooks.star", Source: "second scriptlet",
	}}
	s.expectRefresh(application)

	// Refresh dispatches the initial config event without a config change.
	s.applicationChanges <- struct{}{}
	s.assertRefreshed(c, application)
	c.Assert(s.eventWatchers[0].Wait(), tc.ErrorIsNil)

	workertest.CleanKill(c, w)
	s.assertWatchersStopped(c)
}

func (s *refreshSuite) TestScriptletCharmRefreshUnchangedSources(c *tc.C) {
	w := s.startWorker(c)
	defer workertest.DirtyKill(c, w)
	s.appChanges <- []string{s.application.UUID.String()}
	s.assertRefreshed(c, s.application)
	application := s.application
	application.CharmUUID = tc.Must(c, corecharm.NewID)
	s.expectRefresh(application)

	s.applicationChanges <- struct{}{}
	s.assertRefreshed(c, application)
	c.Assert(s.eventWatchers[0].Wait(), tc.ErrorIsNil)

	workertest.CleanKill(c, w)
	s.assertWatchersStopped(c)
}

func (s *refreshSuite) TestScriptletCharmToNormal(c *tc.C) {
	w := s.startWorker(c)
	defer workertest.DirtyKill(c, w)
	s.appChanges <- []string{s.application.UUID.String()}
	s.assertRefreshed(c, s.application)
	runner, err := w.runner.Worker(s.application.UUID.String(), c.Context().Done())
	c.Assert(err, tc.ErrorIsNil)
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(
		domainunitless.ScriptletApplication{},
		errors.Errorf("charm no longer has scriptlets: %w", applicationerrors.ApplicationNotFound),
	)

	// The global watcher filters out normal charms, so the application watcher
	// must stop the runner after the refresh.
	s.applicationChanges <- struct{}{}
	c.Assert(runner.Wait(), tc.ErrorIsNil)
	c.Check(len(s.executorConfigs), tc.Equals, 0)
	c.Check(len(s.handled), tc.Equals, 0)

	workertest.CleanKill(c, w)
	s.assertWatchersStopped(c)
}

func (s *refreshSuite) startWorker(c *tc.C) *scriptletWorker {
	log := loggertesting.WrapCheckLog(c)
	w, err := NewWorker(Config{
		ScriptletService: s.scriptletService,
		StatusService:    s.statusService,
		NewExecutor: func(ctx context.Context, config ExecutorConfig) (Executor, error) {
			select {
			case s.executorConfigs <- config:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return applicationExecutorFunc(func(ctx context.Context, event domainunitless.Event) (*scriptlet.IntentCollectors, error) {
				select {
				case s.handled <- refreshedEvent{charmUUID: config.Scriptlet.CharmUUID, event: event}:
					return globalEnv().IntentCollectors(), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}), nil
		},
		Clock:       clock.WallClock,
		MaxAllocs:   12345,
		MaxSteps:    67890,
		MaxDuration: 5 * time.Second,
		Logger:      log,
	})
	c.Assert(err, tc.ErrorIsNil)
	return w.(*scriptletWorker)
}

func (s *refreshSuite) assertRefreshed(c *tc.C, application domainunitless.ScriptletApplication) {
	config := waitFor(c, s.executorConfigs)
	c.Check(config.Scriptlet, tc.DeepEquals, application)
	c.Check(config.MaxAllocs, tc.Equals, int64(12345))
	c.Check(config.MaxSteps, tc.Equals, int64(67890))
	c.Check(config.MaxDuration, tc.Equals, 5*time.Second)
	c.Check(waitFor(c, s.handled), tc.DeepEquals, refreshedEvent{
		charmUUID: application.CharmUUID,
		event:     domainunitless.Event{Name: domainunitless.EventConfigChanged},
	})
}

func (s *refreshSuite) expectRefresh(application domainunitless.ScriptletApplication) {
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), application.UUID).Return(application, nil)
	s.expectEventWatcher()
}

func (s *refreshSuite) expectEventWatcher() {
	eventChanges := make(chan []string, 1)
	eventChanges <- []string{domainunitless.EventConfigChanged.String()}
	eventWatcher := watchertest.NewMockStringsWatcher(eventChanges)
	s.eventWatchers = append(s.eventWatchers, eventWatcher)
	s.scriptletService.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).Return(eventWatcher, nil)
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).Return(
		domainunitless.Event{Name: domainunitless.EventConfigChanged}, nil,
	)
}

func (s *refreshSuite) assertWatchersStopped(c *tc.C) {
	c.Assert(s.appWatcher.Wait(), tc.ErrorIsNil)
	c.Assert(s.applicationWatcher.Wait(), tc.ErrorIsNil)
	for _, eventWatcher := range s.eventWatchers {
		c.Assert(eventWatcher.Wait(), tc.ErrorIsNil)
	}
}

type refreshedEvent struct {
	charmUUID corecharm.ID
	event     domainunitless.Event
}
