// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretexpire_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/secretexpire"
	"github.com/juju/juju/internal/worker/secretexpire/mocks"
	rotatemocks "github.com/juju/juju/internal/worker/secretrotate/mocks"
	"github.com/juju/juju/internal/worker/secrettriggerschedule"
	"github.com/juju/juju/internal/worker/secrettriggerschedule/scheduletest"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	testing.BaseSuite

	clock  *testclock.Clock
	config secretexpire.Config

	facade              *mocks.MockSecretManagerFacade
	triggerWatcher      *rotatemocks.MockSecretTriggerWatcher
	expiryConfigChanges chan []corewatcher.SecretTriggerChange
	expiredSecrets      chan []string
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *workerSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

func (s *workerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = testclock.NewClock(time.Now())
	s.facade = mocks.NewMockSecretManagerFacade(ctrl)
	s.triggerWatcher = rotatemocks.NewMockSecretTriggerWatcher(ctrl)
	s.expiryConfigChanges = make(chan []corewatcher.SecretTriggerChange)
	s.expiredSecrets = make(chan []string, 5)
	s.config = secretexpire.Config{
		Clock:               s.clock,
		SecretManagerFacade: s.facade,
		Logger:              loggo.GetLogger("test"),
		SecretOwners:        []names.Tag{names.NewApplicationTag("mariadb")},
		ExpireRevisions:     s.expiredSecrets,
	}
	return ctrl
}

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	_ = s.setup(c)

	s.testValidateConfig(c, func(config *secretexpire.Config) {
		config.SecretManagerFacade = nil
	}, `nil Facade not valid`)

	s.testValidateConfig(c, func(config *secretexpire.Config) {
		config.SecretOwners = nil
	}, `empty SecretOwners not valid`)

	s.testValidateConfig(c, func(config *secretexpire.Config) {
		config.ExpireRevisions = nil
	}, `nil ExpireRevisionsChannel not valid`)

	s.testValidateConfig(c, func(config *secretexpire.Config) {
		config.Logger = nil
	}, `nil Logger not valid`)

	s.testValidateConfig(c, func(config *secretexpire.Config) {
		config.Clock = nil
	}, `nil Clock not valid`)
}

func (s *workerSuite) testValidateConfig(c *gc.C, f func(*secretexpire.Config), expect string) {
	config := s.config
	f(&config)
	c.Check(config.Validate(), gc.ErrorMatches, expect)
}

func (s *workerSuite) expectWorker() {
	s.facade.EXPECT().WatchSecretRevisionsExpiryChanges(s.config.SecretOwners).Return(s.triggerWatcher, nil)
	s.triggerWatcher.EXPECT().Changes().AnyTimes().Return(s.expiryConfigChanges)
	s.triggerWatcher.EXPECT().Kill().AnyTimes()
	s.triggerWatcher.EXPECT().Wait().Return(nil).MinTimes(1)
}

func (s *workerSuite) TestStartStop(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) expectNoExpiry(c *gc.C) {
	select {
	case uris := <-s.expiredSecrets:
		c.Fatalf("got unexpected secret expiry %q", uris)
	case <-time.After(testing.ShortWait):
	}
}

func (s *workerSuite) expectExpired(c *gc.C, expected ...string) {
	select {
	case uris, ok := <-s.expiredSecrets:
		c.Assert(ok, jc.IsTrue)
		c.Assert(uris, jc.SameContents, expected)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for secrets to be expired")
	}
}

func (s *workerSuite) waitAlarmArmed(c *gc.C) {
	scheduletest.WaitTimerArmed(c, s.clock)
}

func (s *workerSuite) TestExpires(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next,
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(time.Hour)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestRetrigger(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next,
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(time.Hour)
	s.expectExpired(c, uri.ID+"/666")
	s.waitAlarmArmed(c)

	// Secret not removed, will retrigger in 5 minutes.
	s.clock.Advance(2 * time.Minute)
	s.expectNoExpiry(c)

	s.clock.Advance(3 * time.Minute)
	s.expectExpired(c, uri.ID+"/666")
	s.waitAlarmArmed(c)

	// Remove secret, will not retrigger.
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:      uri,
		Revision: 666,
	}}
	err = s.clock.WaitAdvance(5*time.Minute, testing.LongWait, 0)
	c.Assert(err, jc.ErrorIsNil)
	s.expectNoExpiry(c)
}

func (s *workerSuite) TestSecretUpdateBeforeExpires(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(time.Hour)
	s.expectNoExpiry(c)

	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: now.Add(time.Hour),
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(0)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestSecretUpdateBeforeExpiresNotTriggered(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
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
	s.clock.Advance(30 * time.Minute)
	s.expectNoExpiry(c)

	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(30 * time.Minute)
	s.expectNoExpiry(c)

	// Final sanity check.
	s.clock.Advance(time.Hour)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestNewSecretTriggersBefore(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
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
	s.clock.Advance(15 * time.Minute)
	s.expectNoExpiry(c)

	// New secret with earlier expiry time triggers first.
	uri2 := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		Revision:        667,
		NextTriggerTime: now.Add(30 * time.Minute),
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(15 * time.Minute)
	s.expectExpired(c, uri2.ID+"/667")
	s.waitAlarmArmed(c)

	s.clock.Advance(30 * time.Minute)
	s.expectExpired(c, uri.ID+"/666", uri2.ID+"/667")
}

func (s *workerSuite) TestManySecretsTrigger(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next,
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(time.Second) // ensure some fake time has elapsed

	uri2 := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		Revision:        667,
		NextTriggerTime: next,
	}}
	s.expiryConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)

	s.clock.Advance(90 * time.Minute)
	s.expectExpired(c, uri.ID+"/666", uri2.ID+"/667")
}

func (s *workerSuite) TestDeleteSecretExpiry(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
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
	s.clock.Advance(30 * time.Minute)
	s.expectNoExpiry(c)

	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:      uri,
		Revision: 666,
	}}
	err = s.clock.WaitAdvance(30*time.Hour, testing.LongWait, 0)
	c.Assert(err, jc.ErrorIsNil)
	s.expectNoExpiry(c)
}

func (s *workerSuite) TestManySecretsDeleteOne(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next,
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(time.Second) // ensure some fake time has elapsed

	uri2 := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		Revision:        667,
		NextTriggerTime: next,
	}}
	s.expiryConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	s.expectNoExpiry(c)

	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:      uri2,
		Revision: 667,
	}}
	s.expiryConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	// Secret 2 would have expired here.
	s.expectNoExpiry(c)

	s.clock.Advance(30 * time.Minute)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestExpiryGranularity(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: now.Add(25 * time.Second),
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(time.Second) // ensure some fake time has elapsed

	uri2 := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		Revision:        667,
		NextTriggerTime: now.Add(39 * time.Second),
	}}
	s.expiryConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)
	// Advance past both sub-minute trigger times so both expire together.
	s.clock.Advance(46 * time.Second)
	s.expectExpired(c, uri.ID+"/666", uri2.ID+"/667")
}

func (s *workerSuite) TestExpireSameTriggerReAdd(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next,
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(time.Hour)
	s.expectExpired(c, uri.ID+"/666")
	s.waitAlarmArmed(c) // wait for retry alarm to be armed

	// The retry is scheduled for next+ExpireRetryDelay. Remove the only
	// revision, then re-add a trigger for exactly that retry time.
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:      uri,
		Revision: 666,
	}}
	// Unbuffered channel send blocks until the deletion is processed.
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next.Add(secrets.ExpireRetryDelay),
	}}
	s.waitAlarmArmed(c)

	s.clock.Advance(secrets.ExpireRetryDelay)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestExpireAtMinuteBoundary(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.clock = testclock.NewClock(time.Now().Truncate(time.Minute))
	s.config.Clock = s.clock
	s.expectWorker()

	w, err := secretexpire.New(s.config)
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
}

func (s *workerSuite) TestExpireTimerFiresEarly(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	start := time.Now()
	clk := scheduletest.NewEarlyTimerClock(start)
	s.config.Clock = clk

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	trigger := start.Add(time.Hour)
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: trigger,
	}}
	clk.WaitArmed(c)

	// Fire two minutes before the trigger is due.
	clk.Advance(58 * time.Minute)
	c.Assert(clk.Fire(clk.Now()), jc.IsTrue)
	clk.WaitArmed(c)
	s.expectNoExpiry(c)

	clk.Advance(2 * time.Minute)
	c.Assert(clk.Fire(clk.Now()), jc.IsTrue)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestExpireRetryBackoffAndResync(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	resyncedWatcher := rotatemocks.NewMockSecretTriggerWatcher(ctrl)
	resyncedChanges := make(chan []corewatcher.SecretTriggerChange)
	resyncCalled := make(chan struct{})
	s.facade.EXPECT().WatchSecretRevisionsExpiryChanges(s.config.SecretOwners).DoAndReturn(
		func(...names.Tag) (corewatcher.SecretTriggerWatcher, error) {
			close(resyncCalled)
			return resyncedWatcher, nil
		},
	)
	resyncedWatcher.EXPECT().Changes().AnyTimes().Return(resyncedChanges)
	resyncedWatcher.EXPECT().Kill().AnyTimes()
	resyncedWatcher.EXPECT().Wait().Return(nil).MinTimes(1)

	w, err := secretexpire.New(s.config)
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

	for attempt := range secrettriggerschedule.MaxTriggerRetries {
		s.waitAlarmArmed(c)
		delay := secrettriggerschedule.TriggerRetryDelay(secrets.ExpireRetryDelay, attempt)
		if delay > secrets.ExpireRetryDelay {
			s.clock.Advance(secrets.ExpireRetryDelay)
			s.expectNoExpiry(c)
			s.clock.Advance(delay - secrets.ExpireRetryDelay)
		} else {
			s.clock.Advance(delay)
		}
		s.expectExpired(c, uri.ID+"/666")
	}

	// Once MaxTriggerRetries is reached, the next deadline resyncs the
	// watcher instead of blindly re-firing.
	s.waitAlarmArmed(c)
	exhaustedDelay := secrettriggerschedule.TriggerRetryDelay(
		secrets.ExpireRetryDelay, secrettriggerschedule.MaxTriggerRetries,
	)
	s.clock.Advance(exhaustedDelay)
	select {
	case <-resyncCalled:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for watcher resync after exhausting retries")
	}
	s.expectNoExpiry(c)

	// If the resynced watcher reports the revision is still due on the
	// controller, expiry fires again.
	resyncedChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: now.Add(time.Hour),
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(0)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestExpireResyncError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()
	s.facade.EXPECT().WatchSecretRevisionsExpiryChanges(s.config.SecretOwners).Return(
		nil, errors.New("resync boom"),
	)

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

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

	for attempt := range secrettriggerschedule.MaxTriggerRetries {
		s.waitAlarmArmed(c)
		s.clock.Advance(secrettriggerschedule.TriggerRetryDelay(secrets.ExpireRetryDelay, attempt))
		s.expectExpired(c, uri.ID+"/666")
	}

	s.waitAlarmArmed(c)
	s.clock.Advance(secrettriggerschedule.TriggerRetryDelay(
		secrets.ExpireRetryDelay, secrettriggerschedule.MaxTriggerRetries,
	))
	workertest.CheckKilled(c, w)
	c.Check(w.Wait(), gc.ErrorMatches, "resync boom")
}

func (s *workerSuite) TestChangesChannelClosed(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             secrets.NewURI(),
		Revision:        666,
		NextTriggerTime: s.clock.Now().Add(time.Hour),
	}}
	s.waitAlarmArmed(c)

	close(s.expiryConfigChanges)
	workertest.CheckKilled(c, w)
	c.Check(w.Wait(), gc.ErrorMatches, "secret revision expiry change channel closed")
}

func (s *workerSuite) TestStaleTriggerIgnoredDuringRetry(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next,
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(time.Hour)
	s.expectExpired(c, uri.ID+"/666")
	s.waitAlarmArmed(c)

	// A duplicate watcher event carrying the identical trigger time must not
	// reset the pending retry backoff, re-arm the alarm, or cause a premature
	// fire.
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next,
	}}
	s.expiryConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)

	s.clock.Advance(time.Minute)
	s.expectNoExpiry(c)

	// The original pending retry still fires on schedule.
	s.clock.Advance(secrets.ExpireRetryDelay - time.Minute)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestShortenedTriggerAcceptedDuringRetry(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next,
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(time.Hour)
	s.expectExpired(c, uri.ID+"/666")
	s.waitAlarmArmed(c)

	// Shortening the expiry time during a retry window must take effect
	// immediately.
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next.Add(-10 * time.Minute),
	}}
	s.waitAlarmArmed(c)
	s.clock.Advance(0)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestBatchDuplicateRevisionLastWins(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	uri := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{
		{
			URI:             uri,
			Revision:        666,
			NextTriggerTime: now.Add(2 * time.Hour),
		},
		{
			URI:             uri,
			Revision:        666,
			NextTriggerTime: now.Add(time.Hour),
		},
	}
	s.waitAlarmArmed(c)

	s.clock.Advance(time.Hour)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestBatchDeleteAndReenableLastWins(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	uriDeleted := secrets.NewURI()
	uriReenabled := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{
		{
			URI:             uriDeleted,
			Revision:        666,
			NextTriggerTime: now.Add(time.Hour),
		},
		{
			URI:      uriDeleted,
			Revision: 666,
		},
		{
			URI:      uriReenabled,
			Revision: 667,
		},
		{
			URI:             uriReenabled,
			Revision:        667,
			NextTriggerTime: now.Add(time.Hour),
		},
	}
	s.waitAlarmArmed(c)

	s.clock.Advance(time.Hour)
	s.expectExpired(c, uriReenabled.ID+"/667")
	s.waitAlarmArmed(c)
	s.clock.Advance(secrets.ExpireRetryDelay)
	s.expectExpired(c, uriReenabled.ID+"/667")
	s.expectNoExpiry(c)
}
