// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretexpire_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/secretexpire"
	rotatemocks "github.com/juju/juju/internal/worker/secretrotate/mocks"
	"github.com/juju/juju/internal/worker/secrettriggerschedule"
)

func (s *workerSuite) TestRestartingWorkerRestartsOnReset(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	resetWatcher := rotatemocks.NewMockSecretTriggerWatcher(ctrl)
	resetChanges := make(chan []corewatcher.SecretTriggerChange)
	s.facade.EXPECT().WatchSecretRevisionsExpiryChanges(s.config.SecretOwners).Return(resetWatcher, nil)
	resetWatcher.EXPECT().Changes().AnyTimes().Return(resetChanges)
	resetWatcher.EXPECT().Kill().AnyTimes()
	resetWatcher.EXPECT().Wait().Return(nil).MinTimes(1)

	w, err := secretexpire.NewWithReset(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: now.Add(time.Hour),
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(time.Hour)
	s.expectExpired(c, uri.ID+"/666")

	// Unconfirmed expiries retry every ExpireRetryDelay.
	for range secrettriggerschedule.MaxTriggerRetries {
		s.waitAlarmArmed(c)
		s.clock.Advance(secrets.ExpireRetryDelay)
		s.expectExpired(c, uri.ID+"/666")
	}

	// Once MaxTriggerRetries is reached, the worker exits with
	// errResetRequired and the runner replaces it after the restart
	// delay. The replacement reports the revision is still due on the
	// controller, so expiry fires again.
	s.waitAlarmArmed(c)
	s.clock.Advance(secrets.ExpireRetryDelay)
	// The worker exits with errResetRequired before arming its own
	// alarm, so this wait is for the runner's restart timer (NewWithReset
	// passes the test clock to worker.NewRunner) to appear on the
	// shared Alarms channel.
	s.waitAlarmArmed(c)
	s.clock.Advance(worker.DefaultRestartDelay)
	resetChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: now.Add(time.Hour),
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(0)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestRestartingWorkerFatalError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.NewWithReset(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             secrets.NewURI(),
		Revision:        666,
		NextTriggerTime: s.clock.Now().Add(time.Hour),
	}}
	s.waitAlarmArmed(c)

	close(s.expiryConfigChanges)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "secret revision expiry change channel closed")
}
