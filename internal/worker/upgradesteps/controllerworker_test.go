// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	version "github.com/juju/version/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	agent "github.com/juju/juju/agent"
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

var _ = gc.Suite(&controllerWorkerSuite{})

func (s *controllerWorkerSuite) TestAlreadyUpgraded(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is already done

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

func (s *controllerWorkerSuite) TestInvalidState(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade if the state is valid

	s.expectUpgradeInfo(c, upgrade.Error)
	done := s.expectAbort(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for error state")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) TestWatchingFailures(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is already done
	// - Register the watchers
	// - Create an upgrade steps worker
	// - Watch for any other nodes to fail to complete

	s.expectAnyClock(make(chan time.Time))
	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	done := s.expectAbort(c)

	s.expectRunUpdates(c)

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial event.
	s.dispatchChange(c, chCompleted)
	s.dispatchChange(c, chFailed)

	s.dispatchChange(c, chFailed)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting abort")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) TestWatchingCompleted(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is already done
	// - Register the watchers
	// - Create an upgrade steps worker
	// - Watch for all other nodes to complete

	s.expectAnyClock(make(chan time.Time))
	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	done := s.expectComplete(c)

	s.expectRunUpdates(c)

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	sync := make(chan struct{})

	srv.SetControllerDone(gomock.Any(), s.upgradeUUID, "0").DoAndReturn(func(ctx context.Context, uuid domainupgrade.UUID, tag string) error {
		defer close(sync)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial event.
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

func (s *controllerWorkerSuite) TestUpgradeFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is already done
	// - Register the watchers
	// - Create an upgrade steps worker
	// - Upgrades failed with generic error. This causes the worker to abort.

	s.expectAnyClock(make(chan time.Time))
	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	done := s.expectAbort(c)

	s.expectRunUpdates(c)

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	w := s.newWorker(c)
	w.base.PreUpgradeSteps = func(_ agent.Config, _ bool) error {
		return errors.New("boom")
	}
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial event.
	s.dispatchChange(c, chCompleted)
	s.dispatchChange(c, chFailed)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting abort")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) TestUpgradeFailureWithAPILostError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is already done
	// - Register the watchers
	// - Create an upgrade steps worker
	// - Upgrades failed with api lost error. This causes the worker to restart.

	s.expectAnyClock(make(chan time.Time))
	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	s.expectRunUpdates(c)

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	w := s.newWorker(c)
	w.base.PreUpgradeSteps = func(_ agent.Config, _ bool) error {
		return upgradesteps.NewAPILostDuringUpgrade(errors.New("boom"))
	}
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial event.
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
	c.Assert(err, gc.ErrorMatches, `.*API connection lost during upgrade: boom`)
}

func (s *controllerWorkerSuite) TestUpgradeStepsComplete(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is already done
	// - Register the watchers
	// - Create an upgrade steps worker
	// - Upgrades performed.
	// - Upgrade steps worker completes.
	// - Dispatch the completed event.

	s.expectAnyClock(make(chan time.Time))
	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	done := s.expectComplete(c)

	s.expectRunUpdates(c)

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
	srv.SetControllerDone(gomock.Any(), s.upgradeUUID, "0").DoAndReturn(func(ctx context.Context, uuid domainupgrade.UUID, tag string) error {
		defer close(sync)
		return nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial event.
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

func (s *controllerWorkerSuite) TestUpgradeFailsWhenKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Walk through the upgrade process:
	// - Check if the upgrade is already done
	// - Check if the active and upgrade info is available
	// - Register the watchers
	// - Send initial events
	// - When running the upgrade steps, kill the worker
	// - Expect the upgrade to be marked as failed

	s.expectAnyClock(make(chan time.Time))
	s.expectUpgradeInfo(c, upgrade.DBCompleted)
	s.expectRunUpdates(c)

	chCompleted := make(chan struct{})
	chFailed := make(chan struct{})

	completedWatcher := watchertest.NewMockNotifyWatcher(chCompleted)
	defer workertest.DirtyKill(c, completedWatcher)

	failedWatcher := watchertest.NewMockNotifyWatcher(chFailed)
	defer workertest.DirtyKill(c, failedWatcher)

	done := make(chan struct{})
	kill := make(chan worker.Worker)

	srv := s.upgradeService.EXPECT()
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.StepsCompleted).Return(completedWatcher, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.Error).Return(failedWatcher, nil)

	srv.SetDBUpgradeFailed(gomock.Any(), s.upgradeUUID).Return(nil)

	w := s.newWorker(c)
	w.base.PreUpgradeSteps = func(_ agent.Config, _ bool) error {
		select {
		case w := <-kill:
			defer close(done)
			w.Kill()
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for kill")
		}
		return nil
	}
	defer workertest.DirtyKill(c, w)

	// Dispatch the initial event.
	s.dispatchChange(c, chCompleted)
	s.dispatchChange(c, chFailed)

	select {
	case kill <- w:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for kill")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for done")
	}

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) newWorker(c *gc.C) *controllerWorker {
	baseWorker := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("9.9.9"))
	w, err := newControllerWorker(baseWorker, s.upgradeService)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *controllerWorkerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	var err error
	s.upgradeUUID, err = domainupgrade.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.upgradeService = NewMockUpgradeService(ctrl)

	return ctrl
}

func (s *controllerWorkerSuite) expectUpgradeInfo(c *gc.C, state upgrade.State) {
	s.lock.EXPECT().IsUnlocked().Return(false)
	s.upgradeService.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, nil)
	s.upgradeService.EXPECT().UpgradeInfo(gomock.Any(), s.upgradeUUID).Return(upgrade.Info{
		State: state,
	}, nil)
}

func (s *controllerWorkerSuite) expectAbort(c *gc.C) chan struct{} {
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

func (s *controllerWorkerSuite) expectComplete(c *gc.C) chan struct{} {
	done := make(chan struct{})
	// Return an error during setting status and set db upgrade failed when
	// aborting to ensure that we ignore it.
	s.statusSetter.EXPECT().SetStatus(gomock.Any(), status.Started, gomock.Any(), gomock.Any()).Return(errors.New("should never be the cause of a failure"))
	s.lock.EXPECT().Unlock().Do(func() {
		close(done)
	})
	return done
}

func (s *controllerWorkerSuite) expectRunUpdates(c *gc.C) {
	s.agent.EXPECT().CurrentConfig().Return(s.config).AnyTimes()
	s.agent.EXPECT().ChangeConfig(gomock.Any()).Return(nil).AnyTimes()
}
