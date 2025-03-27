// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradestepsmachine

import (
	"errors"
	"sync/atomic"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	agent "github.com/juju/juju/agent"
	version "github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
)

type machineWorkerSuite struct {
	baseSuite
}

var _ = gc.Suite(&machineWorkerSuite{})

func (s *machineWorkerSuite) TestAlreadyUpgraded(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.lock.EXPECT().IsUnlocked().DoAndReturn(func() bool {
		defer close(done)
		return true
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for lock to be checked")
	}

	workertest.CleanKill(c, w)
}

func (s *machineWorkerSuite) TestRunUpgrades(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.lock.EXPECT().IsUnlocked().Return(false)
	s.lock.EXPECT().Unlock().Do(func() {
		defer close(done)
	})
	s.agent.EXPECT().CurrentConfig().Return(s.config)
	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(fn agent.ConfigMutator) error {
		// In order to run the tests, we need to
		return fn(s.configSetter)
	})

	s.expectAnyClock(make(chan time.Time))
	// This is called twice, one to notify the upgrade has started and
	// one to notify the upgrade has completed.
	s.expectStatus(status.Started)
	s.expectStatus(status.Started)
	s.expectUpgradeVersion(version.MustParse("9.9.9"))

	var called int64

	baseWorker := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("9.9.9"))
	baseWorker.PreUpgradeSteps = func(_ agent.Config, isController bool) error {
		atomic.AddInt64(&called, 1)
		return nil
	}
	baseWorker.PerformUpgradeSteps = func(from version.Number, targets []upgrades.Target, context upgrades.Context) error {
		atomic.AddInt64(&called, 1)

		// Ensure that the targets are correct.
		c.Check(targets, jc.DeepEquals, []upgrades.Target{upgrades.HostMachine})
		return nil
	}
	w := newMachineWorker(baseWorker)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for lock to be checked")
	}

	c.Check(atomic.LoadInt64(&called), gc.Equals, int64(2))

	workertest.CleanKill(c, w)
}

func (s *machineWorkerSuite) TestRunUpgradesFailedWithAPIError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.lock.EXPECT().IsUnlocked().Return(false)

	s.agent.EXPECT().CurrentConfig().Return(s.config)

	s.expectAnyClock(make(chan time.Time))

	var called int64

	baseWorker := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("9.9.9"))
	baseWorker.PreUpgradeSteps = func(_ agent.Config, isController bool) error {
		defer close(done)

		atomic.AddInt64(&called, 1)
		return upgradesteps.NewAPILostDuringUpgrade(errors.New("boom"))
	}
	w := newMachineWorker(baseWorker)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for lock to be checked")
	}

	c.Check(atomic.LoadInt64(&called), gc.Equals, int64(1))

	err := workertest.CheckKill(c, w)
	c.Assert(err, gc.ErrorMatches, ".*API connection lost during upgrade: boom")
}

func (s *machineWorkerSuite) TestRunUpgradesFailedWithNotAPIError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.lock.EXPECT().IsUnlocked().Return(false)

	s.agent.EXPECT().CurrentConfig().Return(s.config)

	s.expectAnyClock(make(chan time.Time))

	var called int64

	baseWorker := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("9.9.9"))
	baseWorker.PreUpgradeSteps = func(_ agent.Config, isController bool) error {
		defer close(done)

		atomic.AddInt64(&called, 1)
		return errors.New("boom")
	}
	w := newMachineWorker(baseWorker)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for lock to be checked")
	}

	c.Check(atomic.LoadInt64(&called), gc.Equals, int64(1))

	err := workertest.CheckKill(c, w)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *machineWorkerSuite) newWorker(c *gc.C) *machineWorker {
	baseWorker := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("9.9.9"))
	return newMachineWorker(baseWorker)
}
