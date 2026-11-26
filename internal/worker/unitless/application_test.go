// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/life"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainunitless "github.com/juju/juju/domain/unitless"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/scriptlet"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type applicationRunnerSuite struct {
	coretesting.BaseSuite

	scriptletService *MockScriptletService
	statusService    *MockApplicationStatusService
	application      domainunitless.ScriptletApplication
	logger           *recordingLogger
	config           applicationRunnerConfig
}

func TestApplicationRunnerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &applicationRunnerSuite{})
	})
}

func (s *applicationRunnerSuite) TestStartupApplicationNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.assertStartupStops(c, domainunitless.ScriptletApplication{}, applicationerrors.ApplicationNotFound)
}

func (s *applicationRunnerSuite) TestStartupApplicationDying(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.application.Life = life.Dying
	s.assertStartupStops(c, s.application, nil)
}

func (s *applicationRunnerSuite) TestStartupApplicationDead(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.application.Life = life.Dead
	s.assertStartupStops(c, s.application, nil)
}

func (s *applicationRunnerSuite) TestStartupWaitsForInitialLifeNotification(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, false)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	waitFor(c, lifeWatch.changesCalled)

	// No application query or executor creation is expected before readiness.
	workertest.CleanKill(c, runner)
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestStartupApplicationRemovedBeforeInitialNotification(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, false)
	var lookupErr error
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).DoAndReturn(
		func(context.Context, coreapplication.UUID) (domainunitless.ScriptletApplication, error) {
			return domainunitless.ScriptletApplication{}, lookupErr
		},
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	waitFor(c, lifeWatch.changesCalled)

	// The send synchronises the state change with the initial application query.
	lookupErr = errors.Errorf("application removed during watcher startup: %w", applicationerrors.ApplicationNotFound)
	lifeWatch.changes <- struct{}{}
	c.Assert(runner.Wait(), tc.ErrorIsNil)
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestStartupApplicationDyingBeforeInitialNotification(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, false)
	application := s.application
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).DoAndReturn(
		func(context.Context, coreapplication.UUID) (domainunitless.ScriptletApplication, error) {
			return application, nil
		},
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	waitFor(c, lifeWatch.changesCalled)

	application.Life = life.Dying
	lifeWatch.changes <- struct{}{}
	c.Assert(runner.Wait(), tc.ErrorIsNil)
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestStartupApplicationRefreshedBeforeInitialNotification(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, false)
	application := s.application
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).DoAndReturn(
		func(context.Context, coreapplication.UUID) (domainunitless.ScriptletApplication, error) {
			return application, nil
		},
	)
	eventWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	s.scriptletService.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).Return(eventWatcher, nil)
	executorConfigs := make(chan ExecutorConfig, 1)
	s.config.NewExecutor = func(_ context.Context, config ExecutorConfig) (Executor, error) {
		executorConfigs <- config
		return &fakeExecutor{handled: make(chan domainunitless.Event, 1)}, nil
	}
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	waitFor(c, lifeWatch.changesCalled)

	// The initial query must use the charm refreshed during watcher startup.
	application.CharmUUID = tc.Must(c, corecharm.NewID)
	application.Sources = []domainunitless.ScriptSource{{
		LoadPath: "hooks.star", Source: "refreshed scriptlet",
	}}
	lifeWatch.changes <- struct{}{}
	c.Check(waitFor(c, executorConfigs).Scriptlet, tc.DeepEquals, application)

	workertest.CleanKill(c, runner)
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
	c.Assert(eventWatcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestStartupLifeWatcherError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	boom := errors.New("boom")
	s.scriptletService.EXPECT().WatchScriptletApplication(gomock.Any(), s.application.UUID).Return(nil, boom)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "watching scriptlet application .*: boom")
}

func (s *applicationRunnerSuite) TestStartupLifeWatcherClosed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, false)
	close(lifeWatch.changes)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	err := runner.Wait()
	c.Assert(err, tc.ErrorMatches, "scriptlet application watcher closed")
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestStartupLifeWatcherFails(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, false)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, lifeWatch.changesCalled)

	boom := errors.New("boom")
	lifeWatch.watcher.KillErr(boom)
	c.Assert(runner.Wait(), tc.ErrorIs, boom)
}

func (s *applicationRunnerSuite) TestStartupGetApplicationError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, true)
	boom := errors.New("boom")
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(
		domainunitless.ScriptletApplication{}, boom,
	)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "getting scriptlet application .*: boom")
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestStartupEventWatcherError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, true)
	boom := errors.New("boom")
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(s.application, nil)
	s.scriptletService.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).Return(nil, boom)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "watching scriptlet events for application .*: boom")
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestStartupExecutorError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	boom := errors.New("boom")
	s.config.NewExecutor = func(context.Context, ExecutorConfig) (Executor, error) {
		return nil, boom
	}
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "creating scriptlet executor for application .*: boom")
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestStartsExecutorWithApplicationAndLimits(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	config := waitFor(c, fixture.executorConfigs)
	c.Check(config.Scriptlet, tc.DeepEquals, s.application)
	c.Check(config.MaxAllocs, tc.Equals, s.config.MaxAllocs)
	c.Check(config.MaxSteps, tc.Equals, s.config.MaxSteps)
	c.Check(config.MaxDuration, tc.Equals, s.config.MaxDuration)
	c.Check(config.Logger, tc.NotNil)

	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestLifeNotificationApplicationNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(
		domainunitless.ScriptletApplication{}, errors.Errorf("application removed: %w", applicationerrors.ApplicationNotFound),
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.lifeWatch.changes <- struct{}{}
	c.Assert(runner.Wait(), tc.ErrorIsNil)
	fixture.assertWatchersStopped(c)
	c.Check(len(fixture.executor.handled), tc.Equals, 0)
}

func (s *applicationRunnerSuite) TestLifeNotificationApplicationDying(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.assertLifeTransitionStops(c, life.Dying)
}

func (s *applicationRunnerSuite) TestLifeNotificationApplicationDead(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.assertLifeTransitionStops(c, life.Dead)
}

func (s *applicationRunnerSuite) TestLifeNotificationApplicationStillAlive(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	queried := make(chan struct{}, 1)
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).DoAndReturn(
		func(context.Context, coreapplication.UUID) (domainunitless.ScriptletApplication, error) {
			queried <- struct{}{}
			return s.application, nil
		},
	)
	event := domainunitless.Event{Name: "config_changed"}
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, event.Name).Return(event, nil)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.lifeWatch.changes <- struct{}{}
	waitFor(c, queried)

	fixture.eventChanges <- []string{event.Name.String()}
	c.Check(waitFor(c, fixture.executor.handled), tc.DeepEquals, event)
	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestLifeNotificationGetApplicationError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	boom := errors.New("boom")
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(
		domainunitless.ScriptletApplication{}, boom,
	)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.lifeWatch.changes <- struct{}{}
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "getting scriptlet application .*: boom")
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestRefreshEventWatcherError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	s.expectRefresh(c)
	boom := errors.New("boom")
	s.scriptletService.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).Return(nil, boom)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.lifeWatch.changes <- struct{}{}
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "watching scriptlet events for application .*: boom")
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestRefreshExecutorError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	refreshedApplication := s.expectRefresh(c)
	eventWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	s.scriptletService.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).Return(eventWatcher, nil)
	boom := errors.New("boom")
	s.config.NewExecutor = func(_ context.Context, config ExecutorConfig) (Executor, error) {
		if config.Scriptlet.CharmUUID == refreshedApplication.CharmUUID {
			return nil, boom
		}
		fixture.executorConfigs <- config
		return fixture.executor, nil
	}
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.lifeWatch.changes <- struct{}{}
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "creating scriptlet executor for application .*: boom")
	fixture.assertWatchersStopped(c)
	c.Assert(eventWatcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestLifeWatcherClosed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	close(fixture.lifeWatch.changes)
	c.Assert(runner.Wait(), tc.ErrorMatches, "scriptlet application watcher closed")
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestEventWatcherClosed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	close(fixture.eventChanges)
	c.Assert(runner.Wait(), tc.ErrorMatches, "scriptlet event watcher closed")
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestLifeWatcherFails(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	boom := errors.New("boom")
	fixture.lifeWatch.watcher.KillErr(boom)
	c.Assert(runner.Wait(), tc.ErrorIs, boom)
	c.Assert(fixture.eventWatcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestEventWatcherFails(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	boom := errors.New("boom")
	fixture.eventWatcher.KillErr(boom)
	c.Assert(runner.Wait(), tc.ErrorIs, boom)
	c.Assert(fixture.lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestDispatchesEventsInOrderAndLogsIntents(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	statuses := s.expectStatusSets()
	registry := globalEnv()
	fixture.executor.collectors = registry.IntentCollectors()
	statusCollector := scriptlet.IntentCollector[*scriptlet.StatusCollector](fixture.executor.collectors)
	statusCollector.Status = "active"
	statusCollector.Message = "updated"
	first := domainunitless.Event{
		Name:  "config_changed",
		Attrs: map[string]any{"message": "updated"},
	}
	second := domainunitless.Event{Name: "update_status"}

	gomock.InOrder(
		s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, first.Name).Return(first, nil),
		s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, second.Name).Return(second, nil),
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{first.Name.String(), second.Name.String()}
	c.Check(waitFor(c, fixture.executor.handled), tc.DeepEquals, first)
	c.Check(waitFor(c, fixture.executor.handled), tc.DeepEquals, second)

	s.logger.waitFor(c, `event "update_status" intent: set-status`)

	// Each event applies the status collected while handling it, in
	// dispatch order.
	for range 2 {
		info := waitFor(c, statuses)
		c.Check(info.Status, tc.Equals, corestatus.Active)
		c.Check(info.Message, tc.Equals, "updated")
	}

	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
	c.Check(strings.Count(s.logMessages(), "intent: set-status"), tc.Equals, 2)
}

func (s *applicationRunnerSuite) TestIgnoresEmptyEventsAndBatches(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	event := domainunitless.Event{Name: "config_changed"}
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, event.Name).Return(event, nil)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{}
	fixture.eventChanges <- []string{"", event.Name.String(), ""}

	c.Check(waitFor(c, fixture.executor.handled), tc.DeepEquals, event)

	s.logger.waitFor(c, "ignoring empty scriptlet event")

	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestEventApplicationNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).Return(
		domainunitless.Event{}, errors.Errorf("application removed: %w", applicationerrors.ApplicationNotFound),
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{"config_changed", "update_status"}
	c.Assert(runner.Wait(), tc.ErrorIsNil)

	fixture.assertWatchersStopped(c)
	c.Check(len(fixture.executor.handled), tc.Equals, 0)
}

func (s *applicationRunnerSuite) TestEventLookupErrorStopsBatch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	boom := errors.New("boom")
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).Return(
		domainunitless.Event{}, boom,
	)

	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{"config_changed", "update_status"}
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "handling scriptlet event .* for application .*: getting event snapshot: boom")

	fixture.assertWatchersStopped(c)
	c.Check(len(fixture.executor.handled), tc.Equals, 0)
}

func (s *applicationRunnerSuite) TestEventLookupCanceledWithoutShutdown(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).Return(
		domainunitless.Event{}, context.Canceled,
	)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{"config_changed"}
	c.Assert(runner.Wait(), tc.ErrorIs, context.Canceled)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestScriptletFailureSetsBlockedStatus(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Drive the real Starform executor so the scriptlet's own error
	// message, not a stubbed one, must reach the status write.
	s.application.Sources = []domainunitless.ScriptSource{{
		LoadPath: "hooks.star",
		Source: `
def init():
    juju.observe("config_changed", on_config_changed)

def on_config_changed(event):
    fail("canary")
`,
	}}
	fixture := s.expectRunningApplication(c)
	statuses := s.expectStatusSets()
	s.config.NewExecutor = func(ctx context.Context, config ExecutorConfig) (Executor, error) {
		fixture.executorConfigs <- config
		return NewStarformExecutor(ctx, config)
	}

	event := domainunitless.Event{Name: "config_changed"}
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, event.Name).Return(event, nil)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{event.Name.String()}
	info := waitFor(c, statuses)
	c.Check(info.Status, tc.Equals, corestatus.Blocked)
	c.Check(info.Message, tc.Matches, `.*canary.*`)

	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestScriptletSetStatusAppliesIntent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Drive the real Starform executor so the intent is produced by the
	// set-status builtin, not stubbed by a fake executor.
	s.application.Sources = []domainunitless.ScriptSource{{
		LoadPath: "hooks.star",
		Source: `
def init():
    juju.observe("config_changed", on_config_changed)

def on_config_changed(event):
    juju.set_status("active", message="updated")
`,
	}}
	fixture := s.expectRunningApplication(c)
	statuses := s.expectStatusSets()
	s.config.NewExecutor = func(ctx context.Context, config ExecutorConfig) (Executor, error) {
		fixture.executorConfigs <- config
		return NewStarformExecutor(ctx, config)
	}

	// TODO(kcza): The construction of the config_changed event needs to
	// be unified so that the event payload is populated with the
	// application's current config.
	event := domainunitless.Event{Name: "config_changed"}
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, event.Name).Return(event, nil)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{event.Name.String()}
	info := waitFor(c, statuses)
	c.Check(info.Status, tc.Equals, corestatus.Active)
	c.Check(info.Message, tc.Equals, "updated")
	c.Check(info.Since, tc.NotNil)

	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestExecutorErrorContinuesAndSetsBlockedStatus(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	statuses := s.expectStatusSets()
	fixture.executor.err = errors.New("oh no")

	first := domainunitless.Event{Name: "config_changed"}
	second := domainunitless.Event{Name: "update_status"}
	gomock.InOrder(
		s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, first.Name).Return(first, nil),
		s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, second.Name).Return(second, nil),
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{first.Name.String(), second.Name.String()}
	c.Check(waitFor(c, fixture.executor.handled), tc.DeepEquals, first)
	c.Check(waitFor(c, fixture.executor.handled), tc.DeepEquals, second)

	// Each failed event sets the application status to blocked with the
	// error message, and processing continues.
	for range 2 {
		info := waitFor(c, statuses)
		c.Check(info.Status, tc.Equals, corestatus.Blocked)
		c.Check(info.Message, tc.Equals, "oh no")
	}

	s.logger.waitFor(c, `executing scriptlet event "update_status"`)
	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
	c.Check(s.logMessages(), tc.Not(tc.Contains), "intent:")
}

func (s *applicationRunnerSuite) TestUnexpectedCancellationSetsBlockedStatus(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The runner is alive, so a canceled error cannot be shutdown; it
	// must be treated as a scriptlet failure.
	fixture := s.expectRunningApplication(c)
	statuses := s.expectStatusSets()
	fixture.executor.err = context.Canceled

	event := domainunitless.Event{Name: "config_changed"}
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, event.Name).Return(event, nil)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{event.Name.String()}
	info := waitFor(c, statuses)
	c.Check(info.Status, tc.Equals, corestatus.Blocked)
	c.Check(info.Message, tc.Equals, "context canceled")

	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestExecutorErrorStatusSetFailureStopsRunner(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	fixture.executor.err = errors.New("oh no")
	statusErr := errors.New("status oh no")
	s.statusService.EXPECT().SetApplicationStatusByUUID(gomock.Any(), s.application.UUID, gomock.Any()).
		Return(statusErr)

	event := domainunitless.Event{Name: "config_changed"}
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, event.Name).Return(event, nil)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{event.Name.String()}
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, statusErr)
	c.Check(err, tc.ErrorMatches, `handling scriptlet event "config_changed" for application .*: setting blocked status: status oh no`)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestSetStatusIntentFailureStopsRunner(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	registry := globalEnv()
	fixture.executor.collectors = registry.IntentCollectors()
	statusCollector := scriptlet.IntentCollector[*scriptlet.StatusCollector](fixture.executor.collectors)
	statusCollector.Status = "invalid"
	statusErr := errors.New("status oh no")
	s.statusService.EXPECT().SetApplicationStatusByUUID(gomock.Any(), s.application.UUID, gomock.Any()).
		Return(statusErr)

	event := domainunitless.Event{Name: "config_changed"}
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, event.Name).Return(event, nil)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{event.Name.String()}
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, statusErr)
	c.Check(err, tc.ErrorMatches, `handling scriptlet event "config_changed" for application .*: applying set-status intent: status oh no`)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) expectStatusSets() chan corestatus.StatusInfo {
	statuses := make(chan corestatus.StatusInfo, 10)
	s.statusService.EXPECT().SetApplicationStatusByUUID(gomock.Any(), s.application.UUID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ coreapplication.UUID, info corestatus.StatusInfo) error {
			statuses <- info
			return nil
		}).
		AnyTimes()
	return statuses
}

func (s *applicationRunnerSuite) TestShutdownDuringLifeWatcherConstruction(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	defer workertest.CleanKill(c, lifeWatcher)
	contexts := make(chan context.Context, 1)
	s.scriptletService.EXPECT().WatchScriptletApplication(gomock.Any(), s.application.UUID).DoAndReturn(
		func(ctx context.Context, _ coreapplication.UUID) (watcher.NotifyWatcher, error) {
			contexts <- ctx
			<-ctx.Done()
			return lifeWatcher, nil
		},
	)

	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	ctx := waitFor(c, contexts)
	workertest.CleanKill(c, runner)

	c.Check(ctx.Err(), tc.ErrorIs, context.Canceled)
	c.Assert(lifeWatcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestShutdownCancelsApplicationLookup(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, true)
	contexts := make(chan context.Context, 1)
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).DoAndReturn(
		func(ctx context.Context, _ coreapplication.UUID) (domainunitless.ScriptletApplication, error) {
			contexts <- ctx
			<-ctx.Done()
			return domainunitless.ScriptletApplication{}, ctx.Err()
		},
	)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)

	ctx := waitFor(c, contexts)
	workertest.DirtyKill(c, runner)

	c.Assert(runner.Wait(), tc.ErrorIs, context.Canceled)
	c.Check(ctx.Err(), tc.ErrorIs, context.Canceled)
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestShutdownDuringEventWatcherConstruction(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, true)
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(s.application, nil)

	eventWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	defer workertest.CleanKill(c, eventWatcher)

	contexts := make(chan context.Context, 1)
	s.scriptletService.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).DoAndReturn(
		func(ctx context.Context, _ coreapplication.UUID) (watcher.StringsWatcher, error) {
			contexts <- ctx
			<-ctx.Done()
			return eventWatcher, nil
		},
	)

	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	ctx := waitFor(c, contexts)
	workertest.CleanKill(c, runner)

	c.Check(ctx.Err(), tc.ErrorIs, context.Canceled)
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
	c.Assert(eventWatcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestShutdownCancelsExecutorCreation(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	contexts := make(chan context.Context, 1)
	s.config.NewExecutor = func(ctx context.Context, _ ExecutorConfig) (Executor, error) {
		contexts <- ctx
		<-ctx.Done()
		return nil, ctx.Err()
	}
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	ctx := waitFor(c, contexts)
	workertest.DirtyKill(c, runner)
	c.Assert(runner.Wait(), tc.ErrorIs, context.Canceled)
	c.Check(ctx.Err(), tc.ErrorIs, context.Canceled)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestShutdownCancelsEventLookup(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	contexts := make(chan context.Context, 1)
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).DoAndReturn(
		func(ctx context.Context, _ coreapplication.UUID, _ domainunitless.EventType) (domainunitless.Event, error) {
			contexts <- ctx
			<-ctx.Done()
			return domainunitless.Event{}, ctx.Err()
		},
	)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{"config_changed", "update_status"}
	ctx := waitFor(c, contexts)
	workertest.DirtyKill(c, runner)
	c.Assert(runner.Wait(), tc.ErrorIs, context.Canceled)
	c.Check(ctx.Err(), tc.ErrorIs, context.Canceled)
	fixture.assertWatchersStopped(c)
	c.Check(len(fixture.executor.handled), tc.Equals, 0)
}

func (s *applicationRunnerSuite) TestShutdownCancelsEventExecutionAndStopsBatch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	contexts := make(chan context.Context, 1)
	s.config.NewExecutor = func(_ context.Context, config ExecutorConfig) (Executor, error) {
		fixture.executorConfigs <- config
		return applicationExecutorFunc(func(ctx context.Context, _ domainunitless.Event) (*scriptlet.IntentCollectors, error) {
			contexts <- ctx
			<-ctx.Done()
			return nil, ctx.Err()
		}), nil
	}
	s.scriptletService.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).Return(
		domainunitless.Event{Name: "config_changed"}, nil,
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{"config_changed", "update_status"}
	ctx := waitFor(c, contexts)
	workertest.CleanKill(c, runner)
	c.Check(ctx.Err(), tc.ErrorIs, context.Canceled)
	s.logger.waitFor(c, `interrupted by shutdown`)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.scriptletService = NewMockScriptletService(ctrl)
	c.Cleanup(func() {
		s.scriptletService = nil
	})

	s.statusService = NewMockApplicationStatusService(ctrl)
	c.Cleanup(func() {
		s.statusService = nil
	})

	s.application = domainunitless.ScriptletApplication{
		UUID:      tc.Must(c, coreapplication.NewUUID),
		CharmUUID: tc.Must(c, corecharm.NewID),
		Name:      "test-app",
		Life:      life.Alive,
		Sources: []domainunitless.ScriptSource{{
			LoadPath: "hooks.star",
			Source:   "def init(): pass",
		}},
	}
	s.logger = newRecordingLogger()
	s.config = applicationRunnerConfig{
		ApplicationUUID:  s.application.UUID,
		ScriptletService: s.scriptletService,
		StatusService:    s.statusService,
		NewExecutor: func(context.Context, ExecutorConfig) (Executor, error) {
			return nil, errors.New("unexpected executor creation")
		},
		MaxAllocs:   12345,
		MaxSteps:    67890,
		MaxDuration: 5 * time.Second,
		Clock:       clock.WallClock,
		Logger:      s.logger,
	}
	return ctrl
}

func (s *applicationRunnerSuite) assertStartupStops(c *tc.C, application domainunitless.ScriptletApplication, err error) {
	lifeWatch := s.expectLifeWatcher(c, true)

	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(application, err)

	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	c.Assert(runner.Wait(), tc.ErrorIsNil)
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) assertLifeTransitionStops(c *tc.C, l life.Value) {
	fixture := s.expectRunningApplication(c)

	application := s.application
	application.Life = l

	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(application, nil)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.lifeWatch.changes <- struct{}{}
	c.Assert(runner.Wait(), tc.ErrorIsNil)
	fixture.assertWatchersStopped(c)
	c.Check(len(fixture.executor.handled), tc.Equals, 0)
}

func (s *applicationRunnerSuite) expectRunningApplication(c *tc.C) *applicationRunnerFixture {
	lifeWatch := s.expectLifeWatcher(c, true)

	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(s.application, nil)

	eventChanges := make(chan []string, 1)
	eventWatcher := watchertest.NewMockStringsWatcher(eventChanges)

	s.scriptletService.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).Return(eventWatcher, nil)

	fixture := &applicationRunnerFixture{
		lifeWatch:       lifeWatch,
		eventChanges:    eventChanges,
		eventWatcher:    eventWatcher,
		executor:        &fakeExecutor{handled: make(chan domainunitless.Event, 10)},
		executorConfigs: make(chan ExecutorConfig, 1),
	}

	s.config.NewExecutor = func(_ context.Context, config ExecutorConfig) (Executor, error) {
		fixture.executorConfigs <- config
		return fixture.executor, nil
	}

	return fixture
}

func (s *applicationRunnerSuite) expectRefresh(c *tc.C) domainunitless.ScriptletApplication {
	application := s.application
	application.CharmUUID = tc.Must(c, corecharm.NewID)
	s.scriptletService.EXPECT().GetScriptletApplication(gomock.Any(), application.UUID).Return(application, nil)
	return application
}

func (s *applicationRunnerSuite) expectLifeWatcher(c *tc.C, initial bool) *applicationLifeWatch {
	changes := make(chan struct{})
	if initial {
		changes = make(chan struct{}, 1)
		changes <- struct{}{}
	}
	lifeWatcher := &applicationNotifyWatcher{
		MockNotifyWatcher: watchertest.NewMockNotifyWatcher(changes),
		changesCalled:     make(chan struct{}),
	}
	s.scriptletService.EXPECT().WatchScriptletApplication(gomock.Any(), s.application.UUID).Return(lifeWatcher, nil)
	return &applicationLifeWatch{
		changes:       changes,
		watcher:       lifeWatcher,
		changesCalled: lifeWatcher.changesCalled,
	}
}

func (s *applicationRunnerSuite) startRunner(c *tc.C) worker.Worker {
	runner, err := newApplicationRunner(s.config)
	c.Assert(err, tc.ErrorIsNil)
	return runner
}

func (s *applicationRunnerSuite) logMessages() string {
	s.logger.mu.Lock()
	defer s.logger.mu.Unlock()
	return strings.Join(s.logger.messages, "\n")
}

type applicationRunnerFixture struct {
	lifeWatch       *applicationLifeWatch
	eventChanges    chan []string
	eventWatcher    *watchertest.MockStringsWatcher
	executor        *fakeExecutor
	executorConfigs chan ExecutorConfig
}

func (f *applicationRunnerFixture) assertWatchersStopped(c *tc.C) {
	c.Assert(f.lifeWatch.watcher.Wait(), tc.ErrorIsNil)
	c.Assert(f.eventWatcher.Wait(), tc.ErrorIsNil)
}

type applicationLifeWatch struct {
	changes       chan struct{}
	watcher       *applicationNotifyWatcher
	changesCalled <-chan struct{}
}

type applicationNotifyWatcher struct {
	*watchertest.MockNotifyWatcher
	changesCalled chan struct{}
	once          sync.Once
}

func (w *applicationNotifyWatcher) Changes() <-chan struct{} {
	w.once.Do(func() { close(w.changesCalled) })
	return w.MockNotifyWatcher.Changes()
}

type applicationExecutorFunc func(context.Context, domainunitless.Event) (*scriptlet.IntentCollectors, error)

func (f applicationExecutorFunc) Handle(ctx context.Context, event domainunitless.Event) (*scriptlet.IntentCollectors, error) {
	return f(ctx, event)
}
