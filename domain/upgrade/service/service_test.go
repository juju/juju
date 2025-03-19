// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	coreupgrade "github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher/watchertest"
	upgradeerrors "github.com/juju/juju/domain/upgrade/errors"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	baseServiceSuite

	state          *MockState
	watcherFactory *MockWatcherFactory

	service *WatchableService
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	s.state.EXPECT().NamespaceForWatchUpgradeReady().Return("upgrade_info_controller_node").AnyTimes()
	s.state.EXPECT().NamespaceForWatchUpgradeState().Return("upgrade_info").AnyTimes()

	s.service = NewWatchableService(s.state, s.watcherFactory)
	return ctrl
}

func (s *serviceSuite) TestCreateUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateUpgrade(gomock.Any(), version.MustParse("3.0.0"), version.MustParse("3.0.1")).Return(s.upgradeUUID, nil)

	upgradeUUID, err := s.service.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(upgradeUUID, gc.Equals, s.upgradeUUID)
}

func (s *serviceSuite) TestCreateUpgradeAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateUpgrade(gomock.Any(), version.MustParse("3.0.0"), version.MustParse("3.0.1")).Return(s.upgradeUUID, upgradeerrors.AlreadyExists)

	_, err := s.service.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIs, upgradeerrors.AlreadyExists)
}

func (s *serviceSuite) TestCreateUpgradeInvalidVersions(c *gc.C) {
	_, err := s.service.CreateUpgrade(context.Background(), version.MustParse("3.0.1"), version.MustParse("3.0.0"))
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)

	_, err = s.service.CreateUpgrade(context.Background(), version.MustParse("3.0.1"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetControllerReady(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetControllerReady(gomock.Any(), s.upgradeUUID, s.controllerUUID).Return(nil)

	err := s.service.SetControllerReady(context.Background(), s.upgradeUUID, s.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetControllerReadyForeignKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetControllerReady(gomock.Any(), s.upgradeUUID, s.controllerUUID).Return(upgradeerrors.NotFound)

	err := s.service.SetControllerReady(context.Background(), s.upgradeUUID, s.controllerUUID)
	c.Log(err)
	c.Assert(err, jc.ErrorIs, upgradeerrors.NotFound)
}

func (s *serviceSuite) TestStartUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StartUpgrade(gomock.Any(), s.upgradeUUID).Return(nil)

	err := s.service.StartUpgrade(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestStartUpgradeBeforeCreated(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StartUpgrade(gomock.Any(), s.upgradeUUID).Return(upgradeerrors.NotFound)

	err := s.service.StartUpgrade(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIs, upgradeerrors.NotFound)
}

func (s *serviceSuite) TestActiveUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, nil)

	activeUpgrade, err := s.service.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeUpgrade, gc.Equals, s.upgradeUUID)
}

func (s *serviceSuite) TestActiveUpgradeNoUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, errors.Capture(upgradeerrors.NotFound))

	_, err := s.service.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIs, upgradeerrors.NotFound)
}

func (s *serviceSuite) TestSetDBUpgradeCompleted(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetDBUpgradeCompleted(gomock.Any(), s.upgradeUUID).Return(nil)

	err := s.service.SetDBUpgradeCompleted(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetDBUpgradeFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetDBUpgradeFailed(gomock.Any(), s.upgradeUUID).Return(nil)

	err := s.service.SetDBUpgradeFailed(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpgradeInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().UpgradeInfo(gomock.Any(), s.upgradeUUID).Return(coreupgrade.Info{}, nil)

	_, err := s.service.UpgradeInfo(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestWatchForUpgradeReady(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.watcherFactory.EXPECT().NewValueMapperWatcher(gomock.Any(), s.upgradeUUID.String(), gomock.Any(), gomock.Any()).Return(nw, nil)

	watcher, err := s.service.WatchForUpgradeReady(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)
}

func (s *serviceSuite) TestWatchForUpgradeState(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.watcherFactory.EXPECT().NewValueMapperWatcher(gomock.Any(), s.upgradeUUID.String(), gomock.Any(), gomock.Any()).Return(nw, nil)

	watcher, err := s.service.WatchForUpgradeState(context.Background(), s.upgradeUUID, coreupgrade.Started)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)
}

func (s *serviceSuite) TestIsUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, nil)

	upgrading, err := s.service.IsUpgrading(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(upgrading, jc.IsTrue)
}

func (s *serviceSuite) TestIsUpgradeNoUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, errors.Capture(upgradeerrors.NotFound))

	upgrading, err := s.service.IsUpgrading(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(upgrading, jc.IsFalse)
}

func (s *serviceSuite) TestIsUpgradeError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, errors.New("boom"))

	upgrading, err := s.service.IsUpgrading(context.Background())
	c.Assert(err, gc.ErrorMatches, `boom`)
	c.Assert(upgrading, jc.IsFalse)
}
