// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate_test

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/secretbackendrotate"
	"github.com/juju/juju/internal/worker/secretbackendrotate/mocks"
	"github.com/juju/juju/internal/worker/secrettriggerschedule"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	testing.BaseSuite

	clock  *testclock.Clock
	config secretbackendrotate.Config

	facade              *mocks.MockSecretBackendManagerFacade
	rotateWatcher       *mocks.MockSecretBackendRotateWatcher
	rotateConfigChanges chan []corewatcher.SecretBackendRotateChange
	rotatedTokens       chan []string
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
	s.facade = mocks.NewMockSecretBackendManagerFacade(ctrl)
	s.rotateWatcher = mocks.NewMockSecretBackendRotateWatcher(ctrl)
	s.rotateConfigChanges = make(chan []corewatcher.SecretBackendRotateChange)
	s.rotatedTokens = make(chan []string, 5)
	s.config = secretbackendrotate.Config{
		Clock:                      s.clock,
		SecretBackendManagerFacade: s.facade,
		Logger:                     loggo.GetLogger("test"),
	}
	return ctrl
}

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	_ = s.setup(c)

	s.testValidateConfig(c, func(config *secretbackendrotate.Config) {
		config.SecretBackendManagerFacade = nil
	}, `nil Facade not valid`)

	s.testValidateConfig(c, func(config *secretbackendrotate.Config) {
		config.Logger = nil
	}, `nil Logger not valid`)

	s.testValidateConfig(c, func(config *secretbackendrotate.Config) {
		config.Clock = nil
	}, `nil Clock not valid`)
}

func (s *workerSuite) testValidateConfig(c *gc.C, f func(*secretbackendrotate.Config), expect string) {
	config := s.config
	f(&config)
	c.Check(config.Validate(), gc.ErrorMatches, expect)
}

func (s *workerSuite) expectWorker() {
	s.facade.EXPECT().WatchTokenRotationChanges().Return(s.rotateWatcher, nil)
	s.rotateWatcher.EXPECT().Changes().AnyTimes().Return(s.rotateConfigChanges)
	s.rotateWatcher.EXPECT().Kill().MaxTimes(1)
	s.rotateWatcher.EXPECT().Wait().Return(nil).MinTimes(1)

	s.facade.EXPECT().RotateBackendTokens(gomock.Any()).DoAndReturn(
		func(ids ...string) error {
			s.rotatedTokens <- ids
			return nil
		},
	).AnyTimes()
}

func (s *workerSuite) TestStartStop(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) expectRotated(c *gc.C, expected ...string) {
	select {
	case ids, ok := <-s.rotatedTokens:
		c.Assert(ok, jc.IsTrue)
		c.Assert(ids, jc.SameContents, expected)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for token to be rotated")
	}
}

func (s *workerSuite) expectNoRotates(c *gc.C) {
	select {
	case ids := <-s.rotatedTokens:
		c.Fatalf("got unexpected secret rotation %q", ids)
	case <-time.After(testing.ShortWait):
	}
}

func (s *workerSuite) TestFirstToken(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)

	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestBackendUpdateBeforeRotate(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(2 * time.Hour)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestUpdateBeforeRotateNotTriggered(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	// Final sanity check.
	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestNewBackendTriggersBefore(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	clk := testclock.NewClock(time.Now())
	s.clock = clk
	s.config.Clock = clk
	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := clk.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	// New secret with earlier rotate time triggers first.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: now.Add(30 * time.Minute),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(15 * time.Minute)
	s.expectRotated(c, "some-backend-id2")
	waitTimerArmed(c, clk) // retry timer armed on fire

	// The controller reports the new rotation time after the rotation.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(30 * time.Minute)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestManyBackendsTrigger(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, s.clock)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: next,
	}}
	s.rotateConfigChanges <- nil
	assertNoTimerArmed(c, s.clock)

	s.clock.Advance(90 * time.Minute)
	s.expectRotated(c, "some-backend-id", "some-backend-id2")
}

func (s *workerSuite) TestDeleteBackendRotation(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:   "some-backend-id",
		Name: "some-backend",
	}}
	err = s.clock.WaitAdvance(30*time.Hour, testing.LongWait, 0)
	c.Assert(err, jc.ErrorIsNil)
	s.expectNoRotates(c)
}

func (s *workerSuite) TestManyBackendsDeleteOne(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, s.clock)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: next,
	}}
	s.rotateConfigChanges <- nil
	assertNoTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:   "some-backend-id2",
		Name: "some-backend2",
	}}
	s.rotateConfigChanges <- nil
	assertNoTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	// Secret 2 would have rotated here.
	s.expectNoRotates(c)

	s.clock.Advance(30 * time.Minute)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestRotateGranularity(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(25 * time.Second),
	}}
	waitTimerArmed(c, s.clock)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: now.Add(39 * time.Second),
	}}
	s.rotateConfigChanges <- nil
	assertNoTimerArmed(c, s.clock)
	// Advance past both sub-minute trigger times so both rotate together.
	s.clock.Advance(46 * time.Second)
	s.expectRotated(c, "some-backend-id", "some-backend-id2")
}

func (s *workerSuite) TestSameTokenTriggerReAdd(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	// Use a fake clock so the re-added time is exactly the last armed
	// time; the dilated wall clock drifts past it.
	clk := testclock.NewClock(time.Now())
	s.clock = clk
	s.config.Clock = clk
	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := clk.Now()
	next := now.Add(time.Hour)
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, clk)
	clk.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
	waitTimerArmed(c, clk) // retry timer armed on fire

	// The retry is scheduled for next+RotateRetryDelay. Remove the rotation,
	// then re-add a trigger for that exact time.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:   "some-backend-id",
		Name: "some-backend",
	}}
	// Unbuffered channel send blocks until the deletion is processed.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next.Add(secrets.RotateRetryDelay),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(secrets.RotateRetryDelay)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestTokenRotationRetriesUntilUpdate(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	clk := testclock.NewClock(time.Now())
	s.clock = clk
	s.config.Clock = clk
	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := clk.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")

	// No watcher update: the worker retries after RotateRetryDelay.
	waitTimerArmed(c, clk)
	clk.Advance(secrets.RotateRetryDelay)
	s.expectRotated(c, "some-backend-id")
	waitTimerArmed(c, clk)

	// A later watcher update replaces the retry with the real time.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(3 * time.Hour),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(2 * time.Hour)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestTokenRotationAtMinuteBoundary(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	clk := testclock.NewClock(time.Now().Truncate(time.Minute))
	s.clock = clk
	s.config.Clock = clk
	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := clk.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestTokenRotationTimerFiresEarly(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	start := time.Now()
	clk := newEarlyTimerClock(start)
	s.config.Clock = clk

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	trigger := start.Add(time.Hour)
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
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
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestTokenRotationRetryBackoffAndCap(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	clk := testclock.NewClock(time.Now())
	s.clock = clk
	s.config.Clock = clk
	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := clk.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	waitTimerArmed(c, clk)
	clk.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")

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
		s.expectRotated(c, "some-backend-id")
	}

	// After MaxTriggerRetries, retries continue at MaxTriggerRetryDelay cadence.
	for range 2 {
		waitTimerArmed(c, clk)
		clk.Advance(secrets.RotateRetryDelay)
		s.expectNoRotates(c)
		clk.Advance(secrettriggerschedule.MaxTriggerRetryDelay - secrets.RotateRetryDelay)
		s.expectRotated(c, "some-backend-id")
	}
}

func (s *workerSuite) TestBatchDuplicateIDLastWins(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()

	// Last entry in the chronological batch wins (shortened 1h overrides 2h).
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{
		{
			ID:              "some-backend-id",
			Name:            "some-backend",
			NextTriggerTime: now.Add(2 * time.Hour),
		},
		{
			ID:              "some-backend-id",
			Name:            "some-backend",
			NextTriggerTime: now.Add(time.Hour),
		},
	}
	waitTimerArmed(c, s.clock)

	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestStaleTriggerIgnoredDuringRetry(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
	// Retry timer armed for RotateRetryDelay.
	waitTimerArmed(c, s.clock)

	// A duplicate watcher event with the identical trigger time must not reset
	// the pending retry backoff, re-arm the timer, or cause a premature fire.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next,
	}}
	s.rotateConfigChanges <- nil
	assertNoTimerArmed(c, s.clock)

	s.clock.Advance(time.Minute)
	s.expectNoRotates(c)

	// The original pending retry still fires on schedule.
	s.clock.Advance(secrets.RotateRetryDelay - time.Minute)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestShortenedTriggerAcceptedDuringRetry(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	next := now.Add(time.Hour)
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next,
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
	waitTimerArmed(c, s.clock)

	// An earlier trigger time during retry (operator shortening the schedule)
	// must be accepted immediately.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next.Add(-10 * time.Minute),
	}}
	waitTimerArmed(c, s.clock)
	s.clock.Advance(0)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestBatchDeleteAndReenableLastWins(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{
		{
			ID:              "some-backend-id",
			Name:            "some-backend",
			NextTriggerTime: now.Add(time.Hour),
		},
		{
			ID:   "some-backend-id",
			Name: "some-backend",
		},
		{
			ID:   "some-backend-id2",
			Name: "some-backend2",
		},
		{
			ID:              "some-backend-id2",
			Name:            "some-backend2",
			NextTriggerTime: now.Add(time.Hour),
		},
	}
	waitTimerArmed(c, s.clock)

	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id2")
	waitTimerArmed(c, s.clock)
	s.clock.Advance(secrets.RotateRetryDelay)
	s.expectRotated(c, "some-backend-id2")
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
