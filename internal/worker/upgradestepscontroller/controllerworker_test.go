// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradestepscontroller

import (
	"context"
	"errors"
	stdtesting "testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	agent "github.com/juju/juju/agent"
	version "github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher/watchertest"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgradesteps"
)

type controllerWorkerSuite struct {
	baseSuite

	upgradeUUID    domainupgrade.UUID
	upgradeService *MockUpgradeService
}

func TestControllerWorkerSuite(t *stdtesting.T) {
	tc.Run(t, &controllerWorkerSuite{})
}

func (s *controllerWorkerSuite) TestAlreadyUpgraded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is already done

	done := make(chan struct{})
	s.lock.EXPECT().IsUnlocked().DoAndReturn(func() bool {
		defer close(done)
		return true
	})

	w := s.newWorker(c, nil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for lock to be checked")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) TestInvalidState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade if the state is valid

	s.expectAnyClock(make(chan time.Time))
	s.expectActiveUpgrade()

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	srv := s.upgradeService.EXPECT()
	// Watchers are now subscribed before UpgradeInfo (readiness barriers).
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	s.expectUpgradeInfo(c, upgrade.Error)
	done := s.expectAbort(c)

	w := s.newWorker(c, nil)
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial events for both watchers (consumed by addWatcher).
	s.dispatchChange(c, chCompleted)
	s.dispatchChange(c, chFailed)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for error state")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) TestWatchingFailures(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is active
	// - Subscribe to completed and failed watchers (readiness barriers)
	// - Read upgrade info (state check)
	// - Create an upgrade steps worker
	// - Watch for any other nodes to fail to complete

	s.expectAnyClock(make(chan time.Time))
	s.expectActiveUpgrade()

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	done := s.expectAbort(c)

	s.expectRunUpdates(c)

	sync := make(chan struct{})

	srv.SetControllerDone(gomock.Any(), s.upgradeUUID, "0").DoAndReturn(func(ctx context.Context, uuid domainupgrade.UUID, tag string) error {
		defer close(sync)
		return nil
	})

	w := s.newWorker(c, nil)
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial events for both watchers (consumed by addWatcher).
	s.dispatchChange(c, chCompleted)
	s.dispatchChange(c, chFailed)

	select {
	case <-sync:
		s.dispatchChange(c, chFailed)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting setting controller done")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting abort")
	}

	workertest.CleanKill(c, w)
}

// TestChangeDuringStartupFailure verifies that an Error transition arriving
// during the subscription window is not lost. The failure event is pre-seeded
// on a buffered channel alongside the initial event. The worker consumes the
// initial event as a readiness barrier, reads UpgradeInfo, starts the steps
// worker, then processes the Error event in the main loop, causing an abort.
// This exercises the race between watcher construction, subscription, and
// the initial query (rule 6).
func (s *controllerWorkerSuite) TestChangeDuringStartupFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyClock(make(chan time.Time))
	s.expectActiveUpgrade()

	// Pre-seed the failed watcher channel with the initial event AND the
	// Error change event on a buffered channel. The initial event is
	// consumed by addWatcher (readiness barrier); the Error event is
	// processed in the main loop after the steps worker is started.
	chFailed := make(chan struct{}, 2)
	chFailed <- struct{}{}
	chFailed <- struct{}{}
	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	chCompleted := make(chan struct{})
	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	done := s.expectAbort(c)

	s.expectRunUpdates(c)

	w := s.newWorker(c, func(base *upgradesteps.BaseWorker) {
		base.PreUpgradeSteps = func(_ agent.Config) error {
			return nil
		}
	})
	defer workertest.DirtyKill(c, w)

	// Dispatch the completed watcher initial event (readiness barrier).
	// The failed watcher initial event and the Error event are already
	// pre-seeded on the buffered channel.
	s.dispatchChange(c, chCompleted)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting abort")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) TestWatchingCompleted(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is active
	// - Subscribe to completed and failed watchers (readiness barriers)
	// - Read upgrade info (state check)
	// - Create an upgrade steps worker
	// - Watch for all other nodes to complete

	s.expectAnyClock(make(chan time.Time))
	s.expectActiveUpgrade()

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	done := s.expectComplete(c)

	s.expectRunUpdates(c)

	sync := make(chan struct{})

	srv.SetControllerDone(gomock.Any(), s.upgradeUUID, "0").DoAndReturn(func(ctx context.Context, uuid domainupgrade.UUID, tag string) error {
		defer close(sync)
		return nil
	})

	w := s.newWorker(c, nil)
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial events for both watchers (consumed by addWatcher).
	s.dispatchChange(c, chCompleted)
	s.dispatchChange(c, chFailed)

	select {
	case <-sync:
		s.dispatchChange(c, chCompleted)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting setting controller done")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting abort")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) TestUpgradeFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is active
	// - Subscribe to completed and failed watchers (readiness barriers)
	// - Read upgrade info (state check)
	// - Create an upgrade steps worker
	// - Upgrades failed with generic error. This causes the worker to abort.

	s.expectAnyClock(make(chan time.Time))
	s.expectActiveUpgrade()

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	done := s.expectAbort(c)

	s.expectRunUpdates(c)

	w := s.newWorker(c, func(base *upgradesteps.BaseWorker) {
		base.PreUpgradeSteps = func(_ agent.Config) error {
			return errors.New("boom")
		}
	})
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial events for both watchers (consumed by addWatcher).
	s.dispatchChange(c, chCompleted)
	s.dispatchChange(c, chFailed)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting abort")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) TestUpgradeFailureWithAPILostError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is active
	// - Subscribe to completed and failed watchers (readiness barriers)
	// - Read upgrade info (state check)
	// - Create an upgrade steps worker
	// - Upgrades failed with api lost error. This causes the worker to restart.

	s.expectAnyClock(make(chan time.Time))
	s.expectActiveUpgrade()

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	s.expectRunUpdates(c)

	w := s.newWorker(c, func(base *upgradesteps.BaseWorker) {
		base.PreUpgradeSteps = func(_ agent.Config) error {
			return upgradesteps.NewAPILostDuringUpgrade(errors.New("boom"))
		}
	})
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial events for both watchers (consumed by addWatcher).
	s.dispatchChange(c, chCompleted)
	s.dispatchChange(c, chFailed)

	// Manually wait for the worker to be done. This ensures that the worker
	// correctly terminates and we don't encounter a logic race condition for
	// the mocks in the tests.
	done := make(chan struct{})
	go func() {
		w.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to be done")
	}

	err := workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorMatches, `.*API connection lost during upgrade: boom`)
}

func (s *controllerWorkerSuite) TestUpgradeStepsComplete(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is active
	// - Subscribe to completed and failed watchers (readiness barriers)
	// - Read upgrade info (state check)
	// - Create an upgrade steps worker
	// - Upgrades performed.
	// - Upgrade steps worker completes.
	// - Dispatch the completed event.

	s.expectAnyClock(make(chan time.Time))
	s.expectActiveUpgrade()

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	sync := make(chan struct{})

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	done := s.expectComplete(c)

	s.expectRunUpdates(c)

	srv.SetControllerDone(gomock.Any(), s.upgradeUUID, "0").DoAndReturn(func(ctx context.Context, uuid domainupgrade.UUID, tag string) error {
		defer close(sync)
		return nil
	})

	w := s.newWorker(c, nil)
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial events for both watchers (consumed by addWatcher).
	s.dispatchChange(c, chCompleted)
	s.dispatchChange(c, chFailed)

	select {
	case <-sync:
		s.dispatchChange(c, chCompleted)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting setting controller done")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting complete")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) TestUpgradeFailsWhenKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is active
	// - Subscribe to completed and failed watchers (readiness barriers)
	// - Read upgrade info (state check)
	// - Create an upgrade steps worker
	// - When running the upgrade steps, kill the worker
	// - Expect the upgrade to be marked as failed

	s.expectAnyClock(make(chan time.Time))
	s.expectActiveUpgrade()

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	done := make(chan struct{})
	stepsStarted := make(chan struct{})
	releaseSteps := make(chan struct{})

	srv := s.upgradeService.EXPECT()
	// Watchers are subscribed before UpgradeInfo (readiness barriers).
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	s.expectRunUpdates(c)

	srv.SetDBUpgradeFailed(gomock.Any(), s.upgradeUUID).DoAndReturn(func(context.Context, domainupgrade.UUID) error {
		defer close(done)
		return nil
	})

	w := s.newWorker(c, func(base *upgradesteps.BaseWorker) {
		base.PreUpgradeSteps = func(_ agent.Config) error {
			close(stepsStarted)
			select {
			case <-releaseSteps:
			case <-c.Context().Done():
				return c.Context().Err()
			}
			return nil
		}
	})
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial events for both watchers (consumed by addWatcher).
	s.dispatchChange(c, chCompleted)
	s.dispatchChange(c, chFailed)

	select {
	case <-stepsStarted:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for upgrade steps to start")
	}
	w.Kill()
	close(releaseSteps)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for done")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) newWorker(c *tc.C, configureBase func(base *upgradesteps.BaseWorker)) *controllerWorker {
	baseWorker := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("9.9.9"))
	if configureBase != nil {
		configureBase(baseWorker)
	}
	w, err := newControllerWorker(baseWorker, s.upgradeService)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *controllerWorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	var err error
	s.upgradeUUID, err = domainupgrade.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.upgradeService = NewMockUpgradeService(ctrl)

	c.Cleanup(func() {
		s.upgradeUUID = ""
		s.upgradeService = nil
	})

	return ctrl
}

func (s *controllerWorkerSuite) expectActiveUpgrade() {
	s.lock.EXPECT().IsUnlocked().Return(false)
	s.upgradeService.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, nil)
}

func (s *controllerWorkerSuite) expectUpgradeInfo(c *tc.C, state upgrade.State) {
	s.upgradeService.EXPECT().UpgradeInfo(gomock.Any(), s.upgradeUUID).Return(upgrade.Info{
		State: state,
	}, nil)
}

func (s *controllerWorkerSuite) expectAbort(c *tc.C) chan struct{} {
	done := make(chan struct{})
	// Return an error during setting status and set db upgrade failed when
	// aborting to ensure that we ignore it.
	s.statusSetter.EXPECT().SetStatus(gomock.Any(), status.Error, gomock.Any(), gomock.Any()).Return(errors.New("should never be the cause of a failure"))
	s.upgradeService.EXPECT().SetDBUpgradeFailed(gomock.Any(), s.upgradeUUID).DoAndReturn(func(ctx context.Context, uuid domainupgrade.UUID) error {
		defer close(done)
		return errors.New("this should still abort the work flow")
	})
	return done
}

func (s *controllerWorkerSuite) expectComplete(c *tc.C) chan struct{} {
	done := make(chan struct{})
	// Return an error during setting status and set db upgrade failed when
	// aborting to ensure that we ignore it.
	s.statusSetter.EXPECT().SetStatus(gomock.Any(), status.Started, gomock.Any(), gomock.Any()).Return(errors.New("should never be the cause of a failure"))
	s.lock.EXPECT().Unlock().Do(func() {
		close(done)
	})
	return done
}

func (s *controllerWorkerSuite) expectRunUpdates(c *tc.C) {
	s.agent.EXPECT().CurrentConfig().Return(s.config).AnyTimes()
	s.agent.EXPECT().ChangeConfig(gomock.Any()).Return(nil).AnyTimes()
}
