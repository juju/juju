// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/version"
)

type baseWorkerSuite struct {
	baseSuite
}

var _ = gc.Suite(&baseWorkerSuite{})

func (s *baseWorkerSuite) TestAlreadyUpgraded(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("6.6.6"))

	s.lock.EXPECT().IsUnlocked().Return(true)

	upgraded := w.AlreadyUpgraded()
	c.Assert(upgraded, jc.IsTrue)
}

func (s *baseWorkerSuite) TestAlreadyUpgradedVersionMatching(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("6.6.6"))

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.lock.EXPECT().Unlock()

	upgraded := w.AlreadyUpgraded()
	c.Assert(upgraded, jc.IsTrue)
}

func (s *baseWorkerSuite) TestAlreadyUpgradedVersionNotMatching(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("9.9.9"))

	s.lock.EXPECT().IsUnlocked().Return(false)

	upgraded := w.AlreadyUpgraded()
	c.Assert(upgraded, jc.IsFalse)
}

func (s *baseWorkerSuite) TestRunUpgradeSteps(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyClock(make(chan time.Time))
	s.expectStatus(status.Started)
	s.expectUpgradeVersion(version.MustParse("6.6.6"))

	w := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("6.6.6"))
	fn := w.RunUpgradeSteps(context.Background(), []upgrades.Target{upgrades.Controller})
	err := fn(s.configSetter)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseWorkerSuite) TestRunUpgradeStepsFailureBreakableTrue(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyClock(make(chan time.Time))
	s.expectStatus(status.Started)

	w := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("6.6.6"))
	w.APICaller = &breakableAPICaller{
		APICaller: s.apiCaller,
		broken:    true,
	}
	w.PerformUpgradeSteps = func(from version.Number, targets []upgrades.Target, context upgrades.Context) error {
		return errors.New("boom")
	}

	fn := w.RunUpgradeSteps(context.Background(), []upgrades.Target{upgrades.Controller})
	err := fn(s.configSetter)
	c.Assert(err, gc.ErrorMatches, "API connection lost during upgrade: boom")
}

func (s *baseWorkerSuite) TestRunUpgradeStepsFailureBreakableFalse(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan time.Time)

	s.expectAnyClock(ch)
	s.expectStatus(status.Started)

	// Test are expected to retry if we fail to upgrade.
	//  - It will retry 5 times.
	//  - It will set the error status on each attempt.

	go func() {
		for i := 0; i < DefaultRetryAttempts; i++ {
			ch <- time.Now()
		}
	}()
	for i := 0; i < DefaultRetryAttempts; i++ {
		s.expectStatus(status.Error)
	}

	w := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("6.6.6"))
	w.APICaller = &breakableAPICaller{
		APICaller: s.apiCaller,
		broken:    false,
	}
	w.PerformUpgradeSteps = func(from version.Number, targets []upgrades.Target, context upgrades.Context) error {
		return errors.New("boom")
	}

	fn := w.RunUpgradeSteps(context.Background(), []upgrades.Target{upgrades.Controller})
	err := fn(s.configSetter)
	c.Assert(err, gc.ErrorMatches, "boom")
}

type breakableAPICaller struct {
	base.APICaller
	broken bool
}

func (b *breakableAPICaller) IsBroken(_ context.Context) bool {
	return b.broken
}
