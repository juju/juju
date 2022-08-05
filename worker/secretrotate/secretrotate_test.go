// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretrotate_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/secretrotate"
	"github.com/juju/juju/worker/secretrotate/mocks"
)

type workerSuite struct {
	testing.BaseSuite

	clock  *testclock.Clock
	config secretrotate.Config

	facade              *mocks.MockSecretManagerFacade
	rotateWatcher       *mocks.MockSecretRotationWatcher
	rotateConfigChanges chan []corewatcher.SecretRotationChange
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
	s.rotateWatcher = mocks.NewMockSecretRotationWatcher(ctrl)
	s.rotateConfigChanges = make(chan []corewatcher.SecretRotationChange)
	s.rotatedSecrets = make(chan []string, 5)
	s.config = secretrotate.Config{
		Clock:               s.clock,
		SecretManagerFacade: s.facade,
		Logger:              loggo.GetLogger("test"),
		SecretOwner:         names.NewApplicationTag("mariadb"),
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
		config.SecretOwner = nil
	}, `nil SecretOwner not valid`)

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
	s.facade.EXPECT().WatchSecretsRotationChanges(s.config.SecretOwner.String()).Return(s.rotateWatcher, nil)
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

func (s *workerSuite) advanceClock(c *gc.C, d time.Duration) {
	err := s.clock.WaitAdvance(d+time.Minute, testing.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerSuite) expectNoRotates(c *gc.C) {
	select {
	case urls := <-s.rotatedSecrets:
		c.Fatalf("got unexpected secret rotation %q", urls)
	case <-time.After(testing.ShortWait):
	}
}

func (s *workerSuite) expectRotated(c *gc.C, expected ...string) {
	select {
	case urls, ok := <-s.rotatedSecrets:
		c.Assert(ok, jc.IsTrue)
		c.Assert(urls, jc.SameContents, expected)
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

	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: time.Hour,
		LastRotateTime: s.clock.Now(),
	}}
	s.advanceClock(c, time.Hour)
	s.expectRotated(c, uri.ShortString())
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
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: 2 * time.Hour,
		LastRotateTime: now,
	}}
	s.advanceClock(c, time.Hour)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: 3 * time.Hour,
		LastRotateTime: now,
	}}
	s.advanceClock(c, 2*time.Hour)
	s.expectRotated(c, uri.ShortString())
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
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: time.Hour,
		LastRotateTime: now,
	}}
	s.advanceClock(c, 30*time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: 2 * time.Hour,
		LastRotateTime: now,
	}}
	s.advanceClock(c, 30*time.Minute)
	s.expectNoRotates(c)

	// Final sanity check.
	s.advanceClock(c, time.Hour)
	s.expectRotated(c, uri.ShortString())
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
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: time.Hour,
		LastRotateTime: now,
	}}
	s.advanceClock(c, 15*time.Minute)
	s.expectNoRotates(c)

	// New secret with earlier rotate time triggers first.
	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri2,
		RotateInterval: 30 * time.Minute,
		LastRotateTime: now,
	}}
	time.Sleep(testing.ShortWait) // ensure advanceClock happens after timer is updated
	s.advanceClock(c, 15*time.Minute)
	s.expectRotated(c, uri2.ShortString())

	s.advanceClock(c, 30*time.Minute)
	s.expectRotated(c, uri.ShortString())
}

func (s *workerSuite) TestManySecretsTrigger(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	now := s.clock.Now()
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: time.Hour,
		LastRotateTime: now,
	}}
	s.advanceClock(c, time.Second) // ensure some fake time has elapsed

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri2,
		RotateInterval: time.Hour,
		LastRotateTime: now.Add(30 * time.Minute),
	}}

	s.advanceClock(c, 90*time.Minute)
	s.expectRotated(c, uri.ShortString(), uri2.ShortString())
}

func (s *workerSuite) TestDeleteSecretRotation(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretrotate.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: time.Hour,
		LastRotateTime: s.clock.Now(),
	}}
	s.advanceClock(c, 30*time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: 0,
	}}
	s.advanceClock(c, 30*time.Hour)
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
	uri := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: time.Hour,
		LastRotateTime: now,
	}}
	s.advanceClock(c, time.Second) // ensure some fake time has elapsed

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri2,
		RotateInterval: time.Hour,
		LastRotateTime: now.Add(-30 * time.Minute),
	}}
	s.advanceClock(c, 15*time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri2,
		RotateInterval: 0,
	}}
	s.advanceClock(c, 15*time.Minute)
	// Secret 2 would have rotated here.
	s.expectNoRotates(c)

	s.advanceClock(c, 30*time.Minute)
	s.expectRotated(c, uri.ShortString())
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
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri,
		RotateInterval: 45 * time.Second,
		LastRotateTime: now,
	}}
	err = s.clock.WaitAdvance(time.Second, testing.LongWait, 1) // ensure some fake time has elapsed
	c.Assert(err, jc.ErrorIsNil)

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretRotationChange{{
		URI:            uri2,
		RotateInterval: 50 * time.Second,
		LastRotateTime: now,
	}}
	// First secret won't rotate before the one minute granularity.
	err = s.clock.WaitAdvance(46*time.Second, testing.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	s.expectNoRotates(c)

	err = s.clock.WaitAdvance(14*time.Second, testing.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	s.expectRotated(c, uri.ShortString(), uri2.ShortString())
}
