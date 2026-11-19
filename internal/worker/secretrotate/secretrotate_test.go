// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretrotate_test

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/secretrotate"
	"github.com/juju/juju/internal/worker/secretrotate/mocks"
	"github.com/juju/juju/internal/worker/secrettriggerschedule"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	testing.BaseSuite

	clock  *testclock.Clock
	config secretrotate.Config

	facade              *mocks.MockSecretManagerFacade
	rotateWatcher       *mocks.MockSecretTriggerWatcher
	rotateConfigChanges chan []corewatcher.SecretTriggerChange
	rotatedSecrets      chan []string
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
	s.rotateWatcher = mocks.NewMockSecretTriggerWatcher(ctrl)
	s.rotateConfigChanges = make(chan []corewatcher.SecretTriggerChange)
	s.rotatedSecrets = make(chan []string, 5)
	s.config = secretrotate.Config{
		Clock:               s.clock,
		SecretManagerFacade: s.facade,
		Logger:              loggo.GetLogger("test"),
		SecretOwners:        []names.Tag{names.NewApplicationTag("mariadb")},
		RotateSecrets:       s.rotatedSecrets,
	}
	return ctrl
}

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	_ = s.setup(c)

	s.testValidateConfig(c, func(config *secretrotate.Config) {
		config.SecretManagerFacade = nil
	}, `nil Facade not valid`)

	s.testValidateConfig(c, func(config *secretrotate.Config) {
		config.SecretOwners = nil
	}, `empty SecretOwners not valid`)

	s.testValidateConfig(c, func(config *secretrotate.Config) {
		config.RotateSecrets = nil
	}, `nil RotateSecretsChannel not valid`)

	s.testValidateConfig(c, func(config *secretrotate.Config) {
		config.Logger = nil
	}, `nil Logger not valid`)

	s.testValidateConfig(c, func(config *secretrotate.Config) {
		config.Clock = nil
	}, `nil Clock not valid`)
}

func (s *workerSuite) testValidateConfig(c *gc.C, f func(*secretrotate.Config), expect string) {
	config := s.config
	f(&config)
	c.Check(config.Validate(), gc.ErrorMatches, expect)
}

func (s *workerSuite) expectWorker() {
	s.facade.EXPECT().WatchSecretsRotationChanges(s.config.SecretOwners).Return(s.rotateWatcher, nil)
	s.rotateWatcher.EXPECT().Changes().AnyTimes().Return(s.rotateConfigChanges)
	s.rotateWatcher.EXPECT().Kill().MaxTimes(1)
	s.rotateWatcher.EXPECT().Wait().Return(nil).MinTimes(1)
}

func (s *workerSuite) TestStartStop(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) expectNoRotates(c *gc.C) {
	select {
	case uris := <-s.rotatedSecrets:
		c.Fatalf("got unexpected secret rotation %q", uris)
	case <-time.After(testing.ShortWait):
	}
}

func (s *workerSuite) expectRotated(c *gc.C, expected ...string) {
	select {
	case uris, ok := <-s.rotatedSecrets:
		c.Assert(ok, jc.IsTrue)
		c.Assert(uris, jc.SameContents, expected)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for secrets to be rotated")
	}
}

func (s *workerSuite) TestFirstSecret(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestSecretUpdateBeforeRotate(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(2 * time.Hour)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestSecretUpdateBeforeRotateNotTriggered(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	// Final sanity check.
	s.clock.Advance(time.Hour)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestNewSecretTriggersBefore(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	clk := testclock.NewClock(time.Now())
	s.clock = clk
	s.config.Clock = clk
	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := clk.Now()
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	// New secret with earlier rotate time triggers first.
	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: now.Add(30 * time.Minute),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(15 * time.Minute)
	s.expectRotated(c, uri2.String())
	waitTimerArmed(c, clk) // retry timer armed on fire

	// The controller reports the new rotation time after the hook completes.
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(30 * time.Minute)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestManySecretsTrigger(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, s.clock)

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: next,
	}}
	// Empty sync batch blocks until the previous handler completes; soonest is
	// unchanged so no timer re-arm occurs.
	s.rotateConfigChanges <- nil
	assertNoTimerArmed(c, s.clock)

	s.clock.Advance(90 * time.Minute)
	s.expectRotated(c, uri.String(), uri2.String())
}

func (s *workerSuite) TestDeleteSecretRotation(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI: uri,
	}}
	err = s.clock.WaitAdvance(30*time.Hour, testing.LongWait, 0)
	c.Assert(err, jc.ErrorIsNil)
	s.expectNoRotates(c)
}

func (s *workerSuite) TestManySecretsDeleteOne(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, s.clock)

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: next,
	}}
	s.rotateConfigChanges <- nil
	assertNoTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI: uri2,
	}}
	s.rotateConfigChanges <- nil
	assertNoTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	// Secret 2 would have rotated here.
	s.expectNoRotates(c)

	s.clock.Advance(30 * time.Minute)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestRotateGranularity(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(25 * time.Second),
	}}
	waitTimerArmed(c, s.clock)

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: now.Add(39 * time.Second),
	}}
	s.rotateConfigChanges <- nil
	assertNoTimerArmed(c, s.clock)
	// Advance past both sub-minute trigger times so both rotate together.
	s.clock.Advance(46 * time.Second)
	s.expectRotated(c, uri.String(), uri2.String())
}

func (s *workerSuite) TestRotateSameTriggerReAdd(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	// Use a fake clock so the re-added trigger time is exactly the
	// worker's last armed time. The dilated wall clock drifts past it.
	clk := testclock.NewClock(time.Now())
	s.clock = clk
	s.config.Clock = clk
	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := clk.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, clk)
	clk.Advance(time.Hour)
	s.expectRotated(c, uri.String())
	waitTimerArmed(c, clk) // retry timer armed on fire

	// The retry is scheduled for next+RotateRetryDelay. Remove the rotation,
	// then re-add a trigger for that exact time.
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI: uri,
	}}
	// Unbuffered channel send blocks until the deletion is processed.
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next.Add(secrets.RotateRetryDelay),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(secrets.RotateRetryDelay)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestRotateRetriesUntilUpdate(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	clk := testclock.NewClock(time.Now())
	s.clock = clk
	s.config.Clock = clk
	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := clk.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, clk)
	clk.Advance(time.Hour)
	s.expectRotated(c, uri.String())

	// No watcher update: the worker must retry after RotateRetryDelay.
	waitTimerArmed(c, clk)
	clk.Advance(secrets.RotateRetryDelay)
	s.expectRotated(c, uri.String())
	waitTimerArmed(c, clk)

	// A later watcher update replaces the retry with the real time.
	newNext := now.Add(3 * time.Hour)
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: newNext,
	}}
	waitTimerArmed(c, clk)
	clk.Advance(2 * time.Hour)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestRotateAtMinuteBoundary(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	clk := testclock.NewClock(time.Now().Truncate(time.Minute))
	s.clock = clk
	s.config.Clock = clk
	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := clk.Now()
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(time.Hour)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestRotateTimerFiresEarly(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	start := time.Now()
	clk := newEarlyTimerClock(start)
	s.config.Clock = clk

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	trigger := start.Add(time.Hour)
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: trigger,
	}}
	clk.waitArmed(c)

	// Fire two minutes before the trigger is due.
	clk.Advance(58 * time.Minute)
	c.Assert(clk.fire(clk.Now()), jc.IsTrue)
	clk.waitArmed(c)
	s.expectNoRotates(c)

	clk.Advance(2 * time.Minute)
	c.Assert(clk.fire(clk.Now()), jc.IsTrue)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestRotateRetryBackoffAndCap(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	clk := testclock.NewClock(time.Now())
	s.clock = clk
	s.config.Clock = clk
	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := clk.Now()
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(time.Hour)
	s.expectRotated(c, uri.String())

	for attempt := range secrettriggerschedule.MaxTriggerRetries {
		waitTimerArmed(c, clk)
		delay := secrettriggerschedule.TriggerRetryDelay(secrets.RotateRetryDelay, attempt)
		if delay > secrets.RotateRetryDelay {
			clk.Advance(secrets.RotateRetryDelay)
			s.expectNoRotates(c)
			clk.Advance(delay - secrets.RotateRetryDelay)
		} else {
			clk.Advance(delay)
		}
		s.expectRotated(c, uri.String())
	}

	// After MaxTriggerRetries, retries continue at MaxTriggerRetryDelay cadence.
	for range 2 {
		waitTimerArmed(c, clk)
		clk.Advance(secrets.RotateRetryDelay)
		s.expectNoRotates(c)
		clk.Advance(secrettriggerschedule.MaxTriggerRetryDelay - secrets.RotateRetryDelay)
		s.expectRotated(c, uri.String())
	}
}

func (s *workerSuite) TestStaleTriggerIgnoredDuringRetry(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, uri.String())
	// Retry timer armed for RotateRetryDelay.
	waitTimerArmed(c, s.clock)

	// A duplicate watcher event carrying the identical trigger time must not
	// reset the pending retry backoff, re-arm the timer, or cause a premature
	// fire.
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	s.rotateConfigChanges <- nil
	assertNoTimerArmed(c, s.clock)

	s.clock.Advance(time.Minute)
	s.expectNoRotates(c)

	// The original pending retry still fires on schedule.
	s.clock.Advance(secrets.RotateRetryDelay - time.Minute)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestShortenedTriggerAcceptedDuringRetry(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, uri.String())
	// Retry timer armed for RotateRetryDelay.
	waitTimerArmed(c, s.clock)

	// Shortening the schedule to an earlier trigger time during a retry window
	// must take effect immediately rather than waiting for the retry backoff.
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next.Add(-10 * time.Minute),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(0)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestBatchDuplicateURILastWins(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	// Enable followed by delete: delete wins.
	uriDisabled := secrets.NewURI()
	// Delete followed by re-enable: re-enable wins.
	uriReenabled := secrets.NewURI()
	// Shortened schedule in same batch: last (1h) wins over 2h.
	uriShortened := secrets.NewURI()

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{
		{
			URI:             uriDisabled,
			NextTriggerTime: now.Add(time.Hour),
		},
		{
			URI: uriDisabled,
		},
		{
			URI: uriReenabled,
		},
		{
			URI:             uriReenabled,
			NextTriggerTime: now.Add(time.Hour),
		},
		{
			URI:             uriShortened,
			NextTriggerTime: now.Add(2 * time.Hour),
		},
		{
			URI:             uriShortened,
			NextTriggerTime: now.Add(time.Hour),
		},
	}
	waitTimerArmed(c, s.clock)

	s.clock.Advance(time.Hour)
	select {
	case got := <-s.rotatedSecrets:
		c.Check(got, jc.SameContents, []string{
			uriReenabled.String(),
			uriShortened.String(),
		})
	case <-time.After(testing.LongWait):
		c.Fatalf("expected re-enabled and shortened secrets to rotate")
	}

	// Only the two active secrets are tracked (so only they retry).
	waitTimerArmed(c, s.clock)
	s.clock.Advance(secrets.RotateRetryDelay)
	select {
	case got := <-s.rotatedSecrets:
		c.Check(got, jc.SameContents, []string{
			uriReenabled.String(),
			uriShortened.String(),
		})
	case <-time.After(testing.LongWait):
		c.Fatalf("expected only active secrets to retry")
	}
	s.expectNoRotates(c)
}

func waitTimerArmed(c *gc.C, clk *testclock.Clock) {
	select {
	case <-clk.Alarms():
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for timer to be armed")
	}
}

func assertNoTimerArmed(c *gc.C, clk *testclock.Clock) {
	select {
	case <-clk.Alarms():
		c.Fatal("unexpected timer or alarm armed")
	default:
	}
}

type earlyTimerClock struct {
	*testclock.Clock
	armed chan struct{}
	mu    sync.Mutex
	timer *earlyTimer
}

func newEarlyTimerClock(now time.Time) *earlyTimerClock {
	return &earlyTimerClock{
		Clock: testclock.NewClock(now),
		armed: make(chan struct{}, 10),
	}
}

func (c *earlyTimerClock) NewTimer(_ time.Duration) clock.Timer {
	tr := &earlyTimer{
		clock: c,
		ch:    make(chan time.Time, 1),
		armed: true,
	}
	c.mu.Lock()
	c.timer = tr
	c.mu.Unlock()
	c.notifyArmed()
	return tr
}

func (c *earlyTimerClock) fire(now time.Time) bool {
	c.mu.Lock()
	tr := c.timer
	c.mu.Unlock()
	return tr != nil && tr.fire(now)
}

func (c *earlyTimerClock) waitArmed(cc *gc.C) {
	select {
	case <-c.armed:
	case <-time.After(testing.LongWait):
		cc.Fatal("timed out waiting for timer to be armed")
	}
}

func (c *earlyTimerClock) notifyArmed() {
	select {
	case c.armed <- struct{}{}:
	default:
	}
}

type earlyTimer struct {
	mu    sync.Mutex
	clock *earlyTimerClock
	ch    chan time.Time
	armed bool
}

func (t *earlyTimer) Chan() <-chan time.Time { return t.ch }

func (t *earlyTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	was := t.armed
	t.armed = false
	return was
}

func (t *earlyTimer) Reset(time.Duration) bool {
	t.mu.Lock()
	was := t.armed
	t.armed = true
	t.mu.Unlock()
	t.clock.notifyArmed()
	return was
}

func (t *earlyTimer) fire(now time.Time) bool {
	t.mu.Lock()
	if !t.armed {
		t.mu.Unlock()
		return false
	}
	t.armed = false
	t.mu.Unlock()
	t.ch <- now
	return true
}
