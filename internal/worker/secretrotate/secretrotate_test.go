// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretrotate_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/secretrotate"
	"github.com/juju/juju/internal/worker/secretrotate/mocks"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	testing.BaseSuite

	clock  testclock.AdvanceableClock
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

	s.clock = testclock.NewDilatedWallClock(100 * time.Millisecond)
	s.facade = mocks.NewMockSecretManagerFacade(ctrl)
	s.rotateWatcher = mocks.NewMockSecretTriggerWatcher(ctrl)
	s.rotateConfigChanges = make(chan []corewatcher.SecretTriggerChange)
	s.rotatedSecrets = make(chan []string, 5)
	s.config = secretrotate.Config{
		Clock:               s.clock,
		SecretManagerFacade: s.facade,
		Logger:              loggertesting.WrapCheckLog(c),
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(time.Hour)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(time.Hour),
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	// New secret with earlier rotate time triggers first.
	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: now.Add(30 * time.Minute),
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(15 * time.Minute)
	s.expectRotated(c, uri2.String())

	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: next,
	}}

	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(30 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI: uri,
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(30 * time.Hour)
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: next,
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	s.clock.Advance(15 * time.Minute)
	s.expectNoRotates(c)

	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI: uri2,
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
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
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated

	uri2 := secrets.NewURI()
	s.rotateConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		NextTriggerTime: now.Add(39 * time.Second),
	}}
	time.Sleep(100 * time.Millisecond) // ensure advanceClock happens after timer is updated
	// First secret won't rotate before the one minute granularity.
	s.clock.Advance(46 * time.Second)
	s.expectRotated(c, uri.String(), uri2.String())
}
