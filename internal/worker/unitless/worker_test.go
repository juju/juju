// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	domainunitless "github.com/juju/juju/domain/unitless"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	testhelpers.IsolationSuite
}

func TestWorkerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestConfigValidate(c *tc.C) {
	err := Config{}.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, "nil ScriptletService not valid")

	ctrl := gomock.NewController(c)
	service := NewMockScriptletService(ctrl)
	err = Config{ScriptletService: service}.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, "nil NewExecutor not valid")

	newExecutor := func(context.Context, ExecutorConfig) (Executor, error) {
		return nil, nil
	}
	err = Config{
		ScriptletService: service,
		NewExecutor:      newExecutor,
	}.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, "nil Clock not valid")

	err = Config{
		ScriptletService: service,
		NewExecutor:      newExecutor,
		Clock:            clock.WallClock,
	}.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, "nil Logger not valid")

	err = Config{
		ScriptletService: service,
		NewExecutor:      newExecutor,
		Clock:            clock.WallClock,
		Logger:           newRecordingLogger(),
		MaxAllocs:        -1,
	}.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, "negative MaxAllocs not valid")

	err = Config{
		ScriptletService: service,
		NewExecutor:      newExecutor,
		Clock:            clock.WallClock,
		Logger:           newRecordingLogger(),
		MaxSteps:         -1,
	}.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, "negative MaxSteps not valid")

	err = Config{
		ScriptletService: service,
		NewExecutor:      newExecutor,
		Clock:            clock.WallClock,
		Logger:           newRecordingLogger(),
	}.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *workerSuite) TestWorkerDispatchesEventAndLogsIntents(c *tc.C) {
	ctrl := gomock.NewController(c)
	service := NewMockScriptletService(ctrl)
	applicationUUID := coreapplication.UUID("app-uuid-1")
	scriptlet := domainunitless.Scriptlet{
		Sources: []domainunitless.ScriptSource{{
			LoadPath: "hooks.star",
			Source:   "def init(): pass",
		}},
	}
	event := domainunitless.Event{
		Name: "config_changed",
		Attrs: map[string]any{
			"message": "updated",
		},
	}
	appChanges := make(chan []string, 1)
	eventChanges := make(chan []string, 1)
	eventWatchers := make(chan coreapplication.UUID, 1)

	service.EXPECT().WatchScriptletApplications(gomock.Any()).Return(
		watchertest.NewMockStringsWatcher(appChanges), nil,
	)
	service.EXPECT().GetApplicationScriptlet(gomock.Any(), applicationUUID).Return(
		scriptlet, nil,
	)
	service.EXPECT().WatchApplicationEvents(gomock.Any(), applicationUUID).DoAndReturn(
		func(context.Context, coreapplication.UUID) (watcher.StringsWatcher, error) {
			eventWatchers <- applicationUUID
			return watchertest.NewMockStringsWatcher(eventChanges), nil
		},
	)
	service.EXPECT().GetScriptletEvent(gomock.Any(), applicationUUID, "config_changed").Return(
		event, nil,
	)

	executor := &fakeExecutor{
		handled: make(chan domainunitless.Event, 1),
		intents: []Intent{{
			Type: IntentSetStatus,
			Args: map[string]any{
				"status":  "active",
				"message": "updated",
			},
		}},
	}
	executorConfigs := make(chan ExecutorConfig, 1)
	log := newRecordingLogger()
	maxAllocs := int64(12345)
	maxSteps := int64(67890)

	w, err := NewWorker(Config{
		ScriptletService: service,
		NewExecutor: func(ctx context.Context, config ExecutorConfig) (Executor, error) {
			executorConfigs <- config
			return executor, nil
		},
		Clock:     clock.WallClock,
		MaxAllocs: maxAllocs,
		MaxSteps:  maxSteps,
		Logger:    log,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	appChanges <- []string{applicationUUID.String()}
	executorConfig := waitFor(c, executorConfigs)
	c.Check(executorConfig.Scriptlet, tc.DeepEquals, scriptlet)
	c.Check(executorConfig.MaxAllocs, tc.Equals, maxAllocs)
	c.Check(executorConfig.MaxSteps, tc.Equals, maxSteps)
	c.Check(executorConfig.Logger, tc.NotNil)
	waitFor(c, eventWatchers)

	eventChanges <- []string{"config_changed"}
	handledEvent := waitFor(c, executor.handled)
	c.Check(handledEvent, tc.DeepEquals, event)
	log.waitFor(c, `scriptlet application "app-uuid-1" event "config_changed" intent: set-status`)

	workertest.CleanKill(c, w)
}

type fakeExecutor struct {
	handled chan domainunitless.Event
	intents []Intent
	err     error
}

func (e *fakeExecutor) Handle(_ context.Context, event domainunitless.Event) ([]Intent, error) {
	e.handled <- event
	return e.intents, e.err
}

type recordingLogger struct {
	mu       sync.Mutex
	messages []string
	waiting  chan string
}

func newRecordingLogger() *recordingLogger {
	return &recordingLogger{
		waiting: make(chan string, 100),
	}
}

func (l *recordingLogger) Criticalf(ctx context.Context, msg string, args ...any) {
	l.record(msg, args...)
}

func (l *recordingLogger) Errorf(ctx context.Context, msg string, args ...any) {
	l.record(msg, args...)
}

func (l *recordingLogger) Warningf(ctx context.Context, msg string, args ...any) {
	l.record(msg, args...)
}

func (l *recordingLogger) Infof(ctx context.Context, msg string, args ...any) {
	l.record(msg, args...)
}

func (l *recordingLogger) Debugf(ctx context.Context, msg string, args ...any) {
	l.record(msg, args...)
}

func (l *recordingLogger) Tracef(ctx context.Context, msg string, args ...any) {
	l.record(msg, args...)
}

func (l *recordingLogger) Logf(ctx context.Context, level logger.Level, labels logger.Labels, format string, args ...any) {
	l.record(format, args...)
}

func (l *recordingLogger) IsLevelEnabled(logger.Level) bool {
	return true
}

func (l *recordingLogger) Child(string, ...string) logger.Logger {
	return l
}

func (l *recordingLogger) GetChildByName(string) logger.Logger {
	return l
}

func (l *recordingLogger) Helper() {}

func (l *recordingLogger) record(msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	l.mu.Lock()
	l.messages = append(l.messages, formatted)
	l.mu.Unlock()
	l.waiting <- formatted
}

func (l *recordingLogger) waitFor(c *tc.C, contains string) {
	for {
		msg := waitFor(c, l.waiting)
		if strings.Contains(msg, contains) {
			return
		}
	}
}

func waitFor[T any](c *tc.C, ch <-chan T) T {
	select {
	case value := <-ch:
		return value
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for value")
		var zero T
		return zero
	}
}
