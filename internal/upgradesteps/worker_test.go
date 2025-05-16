// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"
	"errors"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/upgrades"
)

type baseWorkerSuite struct {
	baseSuite
}

func TestBaseWorkerSuite(t *stdtesting.T) { tc.Run(t, &baseWorkerSuite{}) }
func (s *baseWorkerSuite) TestAlreadyUpgraded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newBaseWorker(c, semversion.MustParse("6.6.6"), semversion.MustParse("6.6.6"))

	s.lock.EXPECT().IsUnlocked().Return(true)

	upgraded := w.AlreadyUpgraded()
	c.Assert(upgraded, tc.IsTrue)
}

func (s *baseWorkerSuite) TestAlreadyUpgradedVersionMatching(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newBaseWorker(c, semversion.MustParse("6.6.6"), semversion.MustParse("6.6.6"))

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.lock.EXPECT().Unlock()

	upgraded := w.AlreadyUpgraded()
	c.Assert(upgraded, tc.IsTrue)
}

func (s *baseWorkerSuite) TestAlreadyUpgradedVersionNotMatching(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newBaseWorker(c, semversion.MustParse("6.6.6"), semversion.MustParse("9.9.9"))

	s.lock.EXPECT().IsUnlocked().Return(false)

	upgraded := w.AlreadyUpgraded()
	c.Assert(upgraded, tc.IsFalse)
}

func (s *baseWorkerSuite) TestRunUpgradeSteps(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyClock(make(chan time.Time))
	s.expectStatus(status.Started)
	s.expectUpgradeVersion(semversion.MustParse("6.6.6"))

	w := s.newBaseWorker(c, semversion.MustParse("6.6.6"), semversion.MustParse("6.6.6"))
	fn := w.RunUpgradeSteps(c.Context(), []upgrades.Target{upgrades.Controller})
	err := fn(s.configSetter)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseWorkerSuite) TestRunUpgradeStepsFailureBreakableTrue(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyClock(make(chan time.Time))
	s.expectStatus(status.Started)

	w := s.newBaseWorker(c, semversion.MustParse("6.6.6"), semversion.MustParse("6.6.6"))
	w.APICaller = &breakableAPICaller{
		APICaller: s.apiCaller,
		broken:    true,
	}
	w.PerformUpgradeSteps = func(from semversion.Number, targets []upgrades.Target, context upgrades.Context) error {
		return errors.New("boom")
	}

	fn := w.RunUpgradeSteps(c.Context(), []upgrades.Target{upgrades.Controller})
	err := fn(s.configSetter)
	c.Assert(err, tc.ErrorMatches, "API connection lost during upgrade: boom")
}

func (s *baseWorkerSuite) TestRunUpgradeStepsFailureBreakableFalse(c *tc.C) {
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

	w := s.newBaseWorker(c, semversion.MustParse("6.6.6"), semversion.MustParse("6.6.6"))
	w.APICaller = &breakableAPICaller{
		APICaller: s.apiCaller,
		broken:    false,
	}
	w.PerformUpgradeSteps = func(from semversion.Number, targets []upgrades.Target, context upgrades.Context) error {
		return errors.New("boom")
	}

	fn := w.RunUpgradeSteps(c.Context(), []upgrades.Target{upgrades.Controller})
	err := fn(s.configSetter)
	c.Assert(err, tc.ErrorMatches, "boom")
}

type breakableAPICaller struct {
	base.APICaller
	broken bool
}

func (b *breakableAPICaller) IsBroken(_ context.Context) bool {
	return b.broken
}
