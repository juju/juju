// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/secretbackendrotate"
	"github.com/juju/juju/internal/worker/secretbackendrotate/mocks"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	testing.BaseSuite

	clock  testclock.AdvanceableClock
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

	s.clock = testclock.NewDilatedWallClock(100 * time.Millisecond)
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(time.Hour)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(time.Hour),
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id",
		Name:            "some-backend",
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	// New secret with earlier rotate time triggers first.
	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: now.Add(30 * time.Minute),
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(15 * time.Minute)
	s.expectRotated(c, "some-backend-id2")

	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: next,
	}}

	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:   "some-backend-id",
		Name: "some-backend",
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(30 * time.Hour)
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: next,
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:   "some-backend-id2",
		Name: "some-backend2",
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated

	s.rotateConfigChanges <- []corewatcher.SecretBackendRotateChange{{
		ID:              "some-backend-id2",
		Name:            "some-backend2",
		NextTriggerTime: now.Add(39 * time.Second),
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	// First secret won't rotate before the one minute granularity.
	s.clock.Advance(46 * time.Second)
	s.expectRotated(c, "some-backend-id", "some-backend-id2")
}
