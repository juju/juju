// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/secretbackendrotate"
	"github.com/juju/juju/internal/worker/secretbackendrotate/mocks"
	"github.com/juju/juju/internal/worker/secrettriggerschedule/scheduletest"
)

func (s *workerSuite) TestRestartingWorkerRestartsOnReset(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	// The restarted worker gets a fresh watcher from the facade.
	resetWatcher := mocks.NewMockSecretBackendRotateWatcher(ctrl)
	resetChanges := make(chan []corewatcher.SecretBackendRotateChange)
	s.facade.EXPECT().WatchTokenRotationChanges().Return(resetWatcher, nil)
	resetWatcher.EXPECT().Changes().AnyTimes().Return(resetChanges)
	resetWatcher.EXPECT().Kill().AnyTimes()
	resetWatcher.EXPECT().Wait().Return(nil).MinTimes(1)

	w, err := secretbackendrotate.NewWithReset(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")

	// No watcher update: after RotateRetryDelay the worker exits with
	// errResetRequired and the runner replaces it after the restart
	// delay. The replacement reports the backend is still due on the
	// controller, so rotation fires again.
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(secrets.RotateRetryDelay)
	// The worker exits with errResetRequired before arming its own
	// timer, so this wait is for the runner's restart timer (NewWithReset
	// passes the test clock to worker.NewRunner) to appear on the
	// shared timers channel.
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(worker.DefaultRestartDelay)
	resetChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(0)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestRestartingWorkerFatalError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWithReset(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: s.clock.Now().Add(time.Hour),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)

	close(s.rotateConfigChanges)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "secret rotation change channel closed")
}
