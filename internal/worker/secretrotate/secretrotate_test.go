// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretrotate_test

import (
	"time"

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
	"github.com/juju/juju/internal/worker/secrettriggerschedule/scheduletest"
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
	s.rotateWatcher.EXPECT().Kill().AnyTimes()
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
	scheduletest.WaitTimerArmed(c, s.clock)
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(time.Hour),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(0)
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	// Final sanity check.
	s.clock.Advance(time.Hour)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestNewSecretTriggersBefore(c *gc.C) {
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	// New secret with earlier rotate time triggers first.
	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: now.Add(30 * time.Minute),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	s.expectRotated(c, uri2.String())
	scheduletest.WaitTimerArmed(c, s.clock) // timer re-armed for uri

	s.clock.Advance(30 * time.Minute)
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
	scheduletest.WaitTimerArmed(c, s.clock)

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: next,
	}}
	// Empty sync batch blocks until the previous handler completes; soonest is
	// unchanged so no timer re-arm occurs.
	s.rotateConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)

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
	scheduletest.WaitTimerArmed(c, s.clock)
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
	scheduletest.WaitTimerArmed(c, s.clock)

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: next,
	}}
	s.rotateConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI: uri2,
	}}
	s.rotateConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)
	s.clock.Advance(15 * time.Minute)
	// Secret 2 would have rotated here.
	s.expectNoRotates(c)

	s.clock.Advance(30 * time.Minute)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestRotateSubMinuteTriggerFiresExactly(c *gc.C) {
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
	scheduletest.WaitTimerArmed(c, s.clock)

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: now.Add(39 * time.Second),
	}}
	s.rotateConfigChanges <- nil
	scheduletest.AssertNoTimerArmed(c, s.clock)
	// Advance past both sub-minute trigger times so both rotate together.
	s.clock.Advance(46 * time.Second)
	s.expectRotated(c, uri.String(), uri2.String())
}

func (s *workerSuite) TestRotateSameTriggerReAdd(c *gc.C) {
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
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(time.Hour)
	s.expectRotated(c, uri.String())

	// Re-adding a trigger for the identical timestamp after firing must still
	// arm the timer and fire again.
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	scheduletest.WaitTimerArmed(c, s.clock)
	s.clock.Advance(0)
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
	scheduletest.WaitTimerArmed(c, clk)
	clk.Advance(time.Hour)
	s.expectRotated(c, uri.String())
}

func (s *workerSuite) TestRotateTimerFiresEarly(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	start := time.Now()
	clk := scheduletest.NewEarlyTimerClock(start)
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
	clk.WaitArmed(c)

	// Fire two minutes before the trigger is due.
	clk.Advance(58 * time.Minute)
	c.Assert(clk.Fire(clk.Now()), jc.IsTrue)
	clk.WaitArmed(c)
	s.expectNoRotates(c)

	clk.Advance(2 * time.Minute)
	c.Assert(clk.Fire(clk.Now()), jc.IsTrue)
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
	scheduletest.WaitTimerArmed(c, s.clock)

	s.clock.Advance(time.Hour)
	s.expectRotated(c, uriReenabled.String(), uriShortened.String())
	s.expectNoRotates(c)
}

func (s *workerSuite) TestChangesChannelClosed(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             secrets.NewURI(),
		NextTriggerTime: s.clock.Now().Add(time.Hour),
	}}
	scheduletest.WaitTimerArmed(c, s.clock)

	close(s.rotateConfigChanges)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "secret rotation change channel closed")
}
