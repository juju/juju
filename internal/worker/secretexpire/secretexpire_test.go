// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretexpire_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/secretexpire"
	"github.com/juju/juju/internal/worker/secretexpire/mocks"
	rotatemocks "github.com/juju/juju/internal/worker/secretrotate/mocks"
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

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *workerSuite) TearDownTest(c *tc.C) {
	s.BaseSuite.TearDownTest(c)
}

func (s *workerSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = testclock.NewClock(time.Now())
	s.facade = mocks.NewMockSecretManagerFacade(ctrl)
	s.triggerWatcher = rotatemocks.NewMockSecretTriggerWatcher(ctrl)
	s.expiryConfigChanges = make(chan []corewatcher.SecretTriggerChange)
	s.expiredSecrets = make(chan []string, 5)
	s.config = secretexpire.Config{
		Clock:               s.clock,
		SecretManagerFacade: s.facade,
		Logger:              loggertesting.WrapCheckLog(c),
		SecretOwners:        []names.Tag{names.NewApplicationTag("mariadb")},
		ExpireRevisions:     s.expiredSecrets,
	}
	return ctrl
}

func (s *workerSuite) TestValidateConfig(c *tc.C) {
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

func (s *workerSuite) testValidateConfig(c *tc.C, f func(*secretexpire.Config), expect string) {
	config := s.config
	f(&config)
	c.Check(config.Validate(), tc.ErrorMatches, expect)
}

func (s *workerSuite) expectWorker() {
	s.facade.EXPECT().WatchSecretRevisionsExpiryChanges(gomock.Any(), s.config.SecretOwners).Return(s.triggerWatcher, nil)
	s.triggerWatcher.EXPECT().Changes().AnyTimes().Return(s.expiryConfigChanges)
	s.triggerWatcher.EXPECT().Kill().MaxTimes(1)
	s.triggerWatcher.EXPECT().Wait().Return(nil).MinTimes(1)
}

func (s *workerSuite) TestStartStop(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectWorker()

	w, err := secretexpire.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) advanceClock(c *tc.C, d time.Duration) {
	err := s.clock.WaitAdvance(d, testing.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerSuite) expectNoExpiry(c *tc.C) {
	select {
	case uris := <-s.expiredSecrets:
		c.Fatalf("got unexpected secret expiry %q", uris)
	case <-time.After(testing.ShortWait):
	}
}

func (s *workerSuite) expectExpired(c *tc.C, expected ...string) {
	select {
	case uris, ok := <-s.expiredSecrets:
		c.Assert(ok, jc.IsTrue)
		c.Assert(uris, jc.SameContents, expected)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for secrets to be expired")
	}
}

func (s *workerSuite) TestExpires(c *tc.C) {
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
	s.advanceClock(c, time.Hour)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestRetrigger(c *tc.C) {
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
	s.advanceClock(c, time.Hour)
	s.expectExpired(c, uri.ID+"/666")

	// Secret not removed, will retrigger in 5 minutes.
	s.advanceClock(c, 2*time.Minute)
	s.expectNoExpiry(c)

	s.advanceClock(c, 3*time.Minute)
	s.expectExpired(c, uri.ID+"/666")

	// Remove secret, will not retrigger.
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:      uri,
		Revision: 666,
	}}
	s.advanceClock(c, 5*time.Minute)
	s.expectNoExpiry(c)
}

func (s *workerSuite) TestSecretUpdateBeforeExpires(c *tc.C) {
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
	s.advanceClock(c, time.Hour)
	s.expectNoExpiry(c)

	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: now.Add(time.Hour),
	}}
	s.advanceClock(c, 2*time.Hour)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestSecretUpdateBeforeExpiresNotTriggered(c *tc.C) {
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
	s.advanceClock(c, 30*time.Minute)
	s.expectNoExpiry(c)

	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: now.Add(2 * time.Hour),
	}}
	s.advanceClock(c, 30*time.Minute)
	s.expectNoExpiry(c)

	// Final sanity check.
	s.advanceClock(c, time.Hour)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestNewSecretTriggersBefore(c *tc.C) {
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
	s.advanceClock(c, 15*time.Minute)
	s.expectNoExpiry(c)

	// New secret with earlier expiry time triggers first.
	uri2 := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		Revision:        667,
		NextTriggerTime: now.Add(30 * time.Minute),
	}}
	time.Sleep(testing.ShortWait) // ensure advanceClock happens after timer is updated
	s.advanceClock(c, 15*time.Minute)
	s.expectExpired(c, uri2.ID+"/667")

	s.advanceClock(c, 30*time.Minute)
	s.expectExpired(c, uri.ID+"/666", uri2.ID+"/667")
}

func (s *workerSuite) TestManySecretsTrigger(c *tc.C) {
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
	s.advanceClock(c, time.Second) // ensure some fake time has elapsed

	uri2 := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		Revision:        667,
		NextTriggerTime: next,
	}}

	s.advanceClock(c, 90*time.Minute)
	s.expectExpired(c, uri.ID+"/666", uri2.ID+"/667")
}

func (s *workerSuite) TestDeleteSecretExpiry(c *tc.C) {
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
	s.advanceClock(c, 30*time.Minute)
	s.expectNoExpiry(c)

	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:      uri,
		Revision: 666,
	}}
	s.advanceClock(c, 30*time.Hour)
	s.expectNoExpiry(c)
}

func (s *workerSuite) TestManySecretsDeleteOne(c *tc.C) {
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
	s.advanceClock(c, time.Second) // ensure some fake time has elapsed

	uri2 := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		Revision:        667,
		NextTriggerTime: next,
	}}
	s.advanceClock(c, 15*time.Minute)
	s.expectNoExpiry(c)

	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:      uri2,
		Revision: 667,
	}}
	s.advanceClock(c, 15*time.Minute)
	// Secret 2 would have expired here.
	s.expectNoExpiry(c)

	s.advanceClock(c, 30*time.Minute)
	s.expectExpired(c, uri.ID+"/666")
}

func (s *workerSuite) TestExpiryGranularity(c *tc.C) {
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
	s.advanceClock(c, time.Second) // ensure some fake time has elapsed

	uri2 := secrets.NewURI()
	s.expiryConfigChanges <- []corewatcher.SecretTriggerChange{{
		URI:             uri2,
		Revision:        667,
		NextTriggerTime: now.Add(39 * time.Second),
	}}
	// First secret won't expire before the one minute granularity.
	s.advanceClock(c, 46*time.Second)
	s.expectExpired(c, uri.ID+"/666", uri2.ID+"/667")
}
