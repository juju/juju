// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/dependency"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/testing"
	upgrade "github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher/watchertest"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	upgradeerrors "github.com/juju/juju/domain/upgrade/errors"
	jujuversion "github.com/juju/juju/version"
)

// baseSuite is embedded in both the worker and manifold tests.
// Tests should not go on this suite directly.

type workerSuite struct {
	baseSuite

	upgradeUUID domainupgrade.UUID
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestNewLockSameVersionUnlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfig.EXPECT().UpgradedToVersion().Return(jujuversion.Current)
	c.Assert(NewLock(s.agentConfig).IsUnlocked(), jc.IsTrue)
}

func (s *workerSuite) TestNewLockOldVersionLocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfig.EXPECT().UpgradedToVersion().Return(version.Number{})
	c.Assert(NewLock(s.agentConfig).IsUnlocked(), jc.IsFalse)
}

func (s *workerSuite) TestLockAlreadyUnlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.lock.EXPECT().IsUnlocked().Return(true)

	w, err := NewUpgradeDatabaseWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKill(c, w)
	c.Check(err, jc.ErrorIs, dependency.ErrUninstall)
}

func (s *workerSuite) TestLockIsUnlockedIfMatchingVersions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.lock.EXPECT().Unlock()

	cfg := s.getConfig()
	cfg.FromVersion = jujuversion.Current
	cfg.ToVersion = jujuversion.Current

	w, err := NewUpgradeDatabaseWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKill(c, w)
	c.Check(err, jc.ErrorIs, dependency.ErrUninstall)
}

func (s *workerSuite) TestWatchUpgradeInsteadOfPerforming(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that the update hasn't already happened.
	s.lock.EXPECT().IsUnlocked().Return(false)

	cfg := s.getConfig()

	ch := make(chan struct{})

	watcher := watchertest.NewMockNotifyWatcher(ch)
	defer workertest.DirtyKill(c, watcher)

	// Walk through the upgrade process:
	//  - Create Upgrade, but it's already started.
	//  - Get the active upgrade.
	//  - Watch for the upgrade to complete.

	srv := s.upgradeService.EXPECT()
	srv.CreateUpgrade(gomock.Any(), cfg.FromVersion, cfg.ToVersion).Return(domainupgrade.UUID(""), upgradeerrors.ErrUpgradeAlreadyStarted)
	srv.ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, nil)
	srv.WatchForUpgradeState(gomock.Any(), s.upgradeUUID, upgrade.DBCompleted).Return(watcher, nil)

	// We expect the lock to be unlocked when the upgrade completes.
	s.lock.EXPECT().Unlock()

	w, err := NewUpgradeDatabaseWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case ch <- struct{}{}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting to enqueue change")
	}

	err = workertest.CheckKill(c, w)
	c.Check(err, jc.ErrorIs, dependency.ErrUninstall)
}

func (s *workerSuite) TestWatchUpgradeError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that the update hasn't already happened.
	s.lock.EXPECT().IsUnlocked().Return(false)

	cfg := s.getConfig()

	ch := make(chan struct{})

	watcher := watchertest.NewMockNotifyWatcher(ch)
	defer workertest.DirtyKill(c, watcher)

	// Walk through the upgrade process:
	//  - Create Upgrade, but it's already started.
	//  - Get the active upgrade, but it doesn't exist.

	srv := s.upgradeService.EXPECT()
	srv.CreateUpgrade(gomock.Any(), cfg.FromVersion, cfg.ToVersion).Return(domainupgrade.UUID(""), upgradeerrors.ErrUpgradeAlreadyStarted)
	srv.ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, errors.NotFoundf("no upgrade"))

	w, err := NewUpgradeDatabaseWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKill(c, w)
	c.Check(err, jc.ErrorIs, dependency.ErrBounce)
}

func (s *workerSuite) getConfig() Config {
	return Config{
		DBUpgradeCompleteLock: s.lock,
		Agent:                 s.agent,
		Logger:                s.logger,
		UpgradeService:        s.upgradeService,
		DBGetter:              s.dbGetter,
		FromVersion:           version.MustParse("3.0.0"),
		ToVersion:             version.MustParse("6.6.6"),
	}
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.upgradeUUID = domainupgrade.UUID(utils.MustNewUUID().String())

	return ctrl
}
