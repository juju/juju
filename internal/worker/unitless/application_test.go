// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainunitless "github.com/juju/juju/domain/unitless"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/unitless/scriptlet"
)

type applicationRunnerSuite struct {
	coretesting.BaseSuite

	service     *MockScriptletService
	application domainunitless.ScriptletApplication
	logger      *recordingLogger
	config      applicationRunnerConfig
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
	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).DoAndReturn(
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
	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).DoAndReturn(
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

func (s *applicationRunnerSuite) TestStartupLifeWatcherApplicationNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.service.EXPECT().WatchScriptletApplicationDying(gomock.Any(), s.application.UUID).Return(
		nil, errors.Errorf("watching removed application: %w", applicationerrors.ApplicationNotFound),
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	c.Assert(runner.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestStartupLifeWatcherError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	boom := errors.New("boom")
	s.service.EXPECT().WatchScriptletApplicationDying(gomock.Any(), s.application.UUID).Return(nil, boom)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "watching scriptlet application life .*: boom")
}

func (s *applicationRunnerSuite) TestStartupLifeWatcherClosed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, false)
	close(lifeWatch.changes)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	err := runner.Wait()
	c.Assert(err, tc.ErrorMatches, "scriptlet application dying watcher closed")
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
	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(
		domainunitless.ScriptletApplication{}, boom,
	)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "getting scriptlet for application .*: boom")
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestStartupEventWatcherApplicationNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, true)
	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(s.application, nil)
	s.service.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).Return(
		nil, errors.Errorf("watching removed application: %w", applicationerrors.ApplicationNotFound),
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	c.Assert(runner.Wait(), tc.ErrorIsNil)
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) TestStartupEventWatcherError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatch := s.expectLifeWatcher(c, true)
	boom := errors.New("boom")
	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(s.application, nil)
	s.service.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).Return(nil, boom)
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
	c.Check(config.Logger, tc.NotNil)

	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestLifeNotificationApplicationNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(
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
	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).DoAndReturn(
		func(context.Context, coreapplication.UUID) (domainunitless.ScriptletApplication, error) {
			queried <- struct{}{}
			return s.application, nil
		},
	)
	event := domainunitless.Event{Name: "config_changed"}
	s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, event.Name).Return(event, nil)
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
	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(
		domainunitless.ScriptletApplication{}, boom,
	)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.lifeWatch.changes <- struct{}{}
	err := runner.Wait()
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, "getting scriptlet application life .*: boom")
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestLifeWatcherClosed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	close(fixture.lifeWatch.changes)
	c.Assert(runner.Wait(), tc.ErrorMatches, "scriptlet application dying watcher closed")
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
	fixture.executor.intents = []scriptlet.Intent{
		{Type: scriptlet.IntentSetStatus, Args: map[string]any{"status": "active"}},
		{Type: scriptlet.IntentSetStatus, Args: map[string]any{"status": "waiting"}},
	}
	first := domainunitless.Event{
		Name:  "config_changed",
		Attrs: map[string]any{"message": "updated"},
	}
	second := domainunitless.Event{Name: "update_status"}

	gomock.InOrder(
		s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, first.Name).Return(first, nil),
		s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, second.Name).Return(second, nil),
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{first.Name.String(), second.Name.String()}
	c.Check(waitFor(c, fixture.executor.handled), tc.DeepEquals, first)
	c.Check(waitFor(c, fixture.executor.handled), tc.DeepEquals, second)

	s.logger.waitFor(c, `event "update_status" intent: set-status`)

	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
	c.Check(strings.Count(s.logMessages(), "intent: set-status"), tc.Equals, 4)
}

func (s *applicationRunnerSuite) TestIgnoresEmptyEventsAndBatches(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	event := domainunitless.Event{Name: "config_changed"}
	s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, event.Name).Return(event, nil)
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
	s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).Return(
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
	s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).Return(
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
	s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).Return(
		domainunitless.Event{}, context.Canceled,
	)
	runner := s.startRunner(c)
	defer workertest.DirtyKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{"config_changed"}
	c.Assert(runner.Wait(), tc.ErrorIs, context.Canceled)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) TestExecutorErrorContinuesAndDiscardsIntents(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	fixture := s.expectRunningApplication(c)
	fixture.executor.err = errors.New("boom")
	fixture.executor.intents = []scriptlet.Intent{{Type: scriptlet.IntentSetStatus}}

	first := domainunitless.Event{Name: "config_changed"}
	second := domainunitless.Event{Name: "update_status"}
	gomock.InOrder(
		s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, first.Name).Return(first, nil),
		s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, second.Name).Return(second, nil),
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{first.Name.String(), second.Name.String()}
	c.Check(waitFor(c, fixture.executor.handled), tc.DeepEquals, first)
	c.Check(waitFor(c, fixture.executor.handled), tc.DeepEquals, second)

	s.logger.waitFor(c, `executing scriptlet event "update_status"`)
	workertest.CleanKill(c, runner)
	fixture.assertWatchersStopped(c)
	c.Check(s.logMessages(), tc.Not(tc.Contains), "intent:")
}

func (s *applicationRunnerSuite) TestShutdownDuringLifeWatcherConstruction(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lifeWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	defer workertest.CleanKill(c, lifeWatcher)
	contexts := make(chan context.Context, 1)
	s.service.EXPECT().WatchScriptletApplicationDying(gomock.Any(), s.application.UUID).DoAndReturn(
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
	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).DoAndReturn(
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
	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(s.application, nil)

	eventWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	defer workertest.CleanKill(c, eventWatcher)

	contexts := make(chan context.Context, 1)
	s.service.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).DoAndReturn(
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
	s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).DoAndReturn(
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
		return applicationExecutorFunc(func(ctx context.Context, _ domainunitless.Event) ([]scriptlet.Intent, error) {
			contexts <- ctx
			<-ctx.Done()
			return nil, ctx.Err()
		}), nil
	}
	s.service.EXPECT().GetScriptletEvent(gomock.Any(), s.application.UUID, domainunitless.EventConfigChanged).Return(
		domainunitless.Event{Name: "config_changed"}, nil,
	)
	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)
	waitFor(c, fixture.executorConfigs)

	fixture.eventChanges <- []string{"config_changed", "update_status"}
	ctx := waitFor(c, contexts)
	workertest.CleanKill(c, runner)
	c.Check(ctx.Err(), tc.ErrorIs, context.Canceled)
	fixture.assertWatchersStopped(c)
}

func (s *applicationRunnerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.service = NewMockScriptletService(ctrl)
	c.Cleanup(func() {
		s.service = nil
	})

	s.application = domainunitless.ScriptletApplication{
		UUID: tc.Must(c, coreapplication.NewUUID),
		Name: "test-app",
		Life: life.Alive,
		Sources: []domainunitless.ScriptSource{{
			LoadPath: "hooks.star",
			Source:   "def init(): pass",
		}},
	}
	s.logger = newRecordingLogger()
	s.config = applicationRunnerConfig{
		ApplicationUUID:  s.application.UUID,
		ScriptletService: s.service,
		NewExecutor: func(context.Context, ExecutorConfig) (Executor, error) {
			return nil, errors.New("unexpected executor creation")
		},
		MaxAllocs: 12345,
		MaxSteps:  67890,
		Logger:    s.logger,
	}
	return ctrl
}

func (s *applicationRunnerSuite) assertStartupStops(c *tc.C, application domainunitless.ScriptletApplication, err error) {
	lifeWatch := s.expectLifeWatcher(c, true)

	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(application, err)

	runner := s.startRunner(c)
	defer workertest.CleanKill(c, runner)

	c.Assert(runner.Wait(), tc.ErrorIsNil)
	c.Assert(lifeWatch.watcher.Wait(), tc.ErrorIsNil)
}

func (s *applicationRunnerSuite) assertLifeTransitionStops(c *tc.C, l life.Value) {
	fixture := s.expectRunningApplication(c)

	application := s.application
	application.Life = l

	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(application, nil)
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

	s.service.EXPECT().GetScriptletApplication(gomock.Any(), s.application.UUID).Return(s.application, nil)

	eventChanges := make(chan []string, 1)
	eventWatcher := watchertest.NewMockStringsWatcher(eventChanges)

	s.service.EXPECT().WatchApplicationEvents(gomock.Any(), s.application.UUID).Return(eventWatcher, nil)

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
	s.service.EXPECT().WatchScriptletApplicationDying(gomock.Any(), s.application.UUID).Return(lifeWatcher, nil)
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

type applicationExecutorFunc func(context.Context, domainunitless.Event) ([]scriptlet.Intent, error)

func (f applicationExecutorFunc) Handle(ctx context.Context, event domainunitless.Event) ([]scriptlet.Intent, error) {
	return f(ctx, event)
}
