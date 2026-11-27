// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/secretbackendrotate"
	"github.com/juju/juju/internal/worker/secretbackendrotate/mocks"
	"github.com/juju/juju/internal/worker/secrettriggerschedule/scheduletest"
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
	s.rotateWatcher.EXPECT().Kill().AnyTimes()
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
	scheduletest.WaitTimerArmed(c, s.clock)
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(0)
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	// Final sanity check.
	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestNewBackendTriggersBefore(c *gc.C) {
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	// New secret with earlier rotate time triggers first.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: now.Add(30 * time.Minute),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	s.expectRotated(c, "some-backend-id2")
	scheduletest.WaitTimerArmed(c, s.clock) // confirmation timer armed on fire

	// The controller reports the new rotation time after the rotation.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
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
	scheduletest.WaitTimerArmed(c, s.clock)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: next,
	}}
	s.rotateConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)

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
	scheduletest.WaitTimerArmed(c, s.clock)
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
	scheduletest.WaitTimerArmed(c, s.clock)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: next,
	}}
	s.rotateConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:   "some-backend-id2",
		Name: "some-backend2",
	}}
	s.rotateConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	// Secret 2 would have rotated here.
	s.expectNoRotates(c)

	s.clock.Advance(30 * time.Minute)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestRotateSubMinuteTriggerFiresExactly(c *gc.C) {
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
	scheduletest.WaitTimerArmed(c, s.clock)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: now.Add(39 * time.Second),
	}}
	s.rotateConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)
	// Advance past both sub-minute trigger times so both rotate together.
	s.clock.Advance(46 * time.Second)
	s.expectRotated(c, "some-backend-id", "some-backend-id2")
}

func (s *workerSuite) TestSameTokenTriggerReAdd(c *gc.C) {
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
	scheduletest.WaitTimerArmed(c, s.clock) // confirmation timer armed on fire

	// The confirmation deadline is scheduled for next+RotateRetryDelay. Remove
	// the rotation, then re-add a trigger for that exact time.
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(secrets.RotateRetryDelay)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestTokenRotationExitsWhenUnconfirmed(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

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
	// errResetRequired so the runner created by NewWithReset restarts
	// it with a fresh watcher.
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(secrets.RotateRetryDelay)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches,
		"rotating secret backends: secret backend token rotation watcher reset required")
}

func (s *workerSuite) TestRotationSentForDueEntriesWhenUnconfirmed(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	now := s.clock.Now()
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}, {
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: now.Add(time.Hour + secrets.RotateRetryDelay),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")

	// No watcher update for some-backend-id: after RotateRetryDelay
	// its confirmation window expires at the same deadline
	// some-backend-id2 becomes due. The due rotation is still
	// performed before the worker exits with errResetRequired so the
	// runner created by NewWithReset restarts it with a fresh watcher.
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(secrets.RotateRetryDelay)
	s.expectRotated(c, "some-backend-id2")
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches,
		"rotating secret backends: secret backend token rotation watcher reset required")
}

func (s *workerSuite) TestStartWatcherError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.facade.EXPECT().WatchTokenRotationChanges().Return(nil, errors.New("watch boom"))

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "watch boom")
}

func (s *workerSuite) TestRotateBackendTokensError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.facade.EXPECT().WatchTokenRotationChanges().Return(s.rotateWatcher, nil)
	s.rotateWatcher.EXPECT().Changes().AnyTimes().Return(s.rotateConfigChanges)
	s.rotateWatcher.EXPECT().Kill().AnyTimes()
	s.rotateWatcher.EXPECT().Wait().Return(nil).MinTimes(1)
	s.facade.EXPECT().RotateBackendTokens("some-backend-id").Return(errors.New("rotate boom"))

	w, err := secretbackendrotate.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: s.clock.Now().Add(time.Hour),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	err = workertest.CheckKilled(c, w)
	c.Check(
		err,
		gc.ErrorMatches,
		`rotating secret backends: cannot rotate secret backend tokens for backend ids \["some-backend-id"\]: rotate boom`,
	)
}

func (s *workerSuite) TestChangesChannelClosed(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretbackendrotate.NewWorker(s.config)
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
	scheduletest.WaitTimerArmed(c, clk)
	clk.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestTokenRotationTimerFiresEarly(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	start := time.Now()
	clk := scheduletest.NewEarlyTimerClock(start)
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
	clk.WaitArmed(c)

	// Fire two minutes before the trigger is due.
	clk.Advance(58 * time.Minute)
	c.Assert(clk.Fire(clk.Now()), jc.IsTrue)
	clk.WaitArmed(c)
	s.expectNoRotates(c)

	clk.Advance(2 * time.Minute)
	c.Assert(clk.Fire(clk.Now()), jc.IsTrue)
	s.expectRotated(c, "some-backend-id")
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
	scheduletest.WaitTimerArmed(c, s.clock)

	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
}

func (s *workerSuite) TestStaleTriggerIgnoredDuringConfirmationWait(c *gc.C) {
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
	// Confirmation timer armed for RotateRetryDelay.
	scheduletest.WaitTimerArmed(c, s.clock)

	// A duplicate watcher event with the identical trigger time must not
	// re-arm the timer or cause a second RotateBackendTokens call.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next,
	}}
	s.rotateConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)

	s.clock.Advance(time.Minute)
	s.expectNoRotates(c)
}

func (s *workerSuite) TestShortenedTriggerAcceptedDuringConfirmationWait(c *gc.C) {
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id")
	scheduletest.WaitTimerArmed(c, s.clock)

	// An earlier trigger time while awaiting confirmation (operator shortening
	// the schedule) must be accepted immediately.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: next.Add(-10 * time.Minute),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
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
	scheduletest.WaitTimerArmed(c, s.clock)

	s.clock.Advance(time.Hour)
	s.expectRotated(c, "some-backend-id2")
	s.expectNoRotates(c)
}
