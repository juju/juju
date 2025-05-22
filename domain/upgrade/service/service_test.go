// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	coreupgrade "github.com/juju/juju/core/upgrade"
	watcher "github.com/juju/juju/core/watcher"
	eventsource "github.com/juju/juju/core/watcher/eventsource"
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

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	s.state.EXPECT().NamespaceForWatchUpgradeReady().Return("upgrade_info_controller_node").AnyTimes()
	s.state.EXPECT().NamespaceForWatchUpgradeState().Return("upgrade_info").AnyTimes()

	s.service = NewWatchableService(s.state, s.watcherFactory)

	c.Cleanup(func() {
		s.state = nil
		s.watcherFactory = nil
		s.service = nil
	})
	return ctrl
}

func (s *serviceSuite) TestCreateUpgrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateUpgrade(gomock.Any(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1")).Return(s.upgradeUUID, nil)

	upgradeUUID, err := s.service.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(upgradeUUID, tc.Equals, s.upgradeUUID)
}

func (s *serviceSuite) TestCreateUpgradeAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateUpgrade(gomock.Any(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1")).Return(s.upgradeUUID, upgradeerrors.AlreadyExists)

	_, err := s.service.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIs, upgradeerrors.AlreadyExists)
}

func (s *serviceSuite) TestCreateUpgradeInvalidVersions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.CreateUpgrade(c.Context(), semversion.MustParse("3.0.1"), semversion.MustParse("3.0.0"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	_, err = s.service.CreateUpgrade(c.Context(), semversion.MustParse("3.0.1"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetControllerReady(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetControllerReady(gomock.Any(), s.upgradeUUID, s.controllerUUID).Return(nil)

	err := s.service.SetControllerReady(c.Context(), s.upgradeUUID, s.controllerUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetControllerReadyForeignKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetControllerReady(gomock.Any(), s.upgradeUUID, s.controllerUUID).Return(upgradeerrors.NotFound)

	err := s.service.SetControllerReady(c.Context(), s.upgradeUUID, s.controllerUUID)
	c.Log(err)
	c.Assert(err, tc.ErrorIs, upgradeerrors.NotFound)
}

func (s *serviceSuite) TestStartUpgrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StartUpgrade(gomock.Any(), s.upgradeUUID).Return(nil)

	err := s.service.StartUpgrade(c.Context(), s.upgradeUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestStartUpgradeBeforeCreated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StartUpgrade(gomock.Any(), s.upgradeUUID).Return(upgradeerrors.NotFound)

	err := s.service.StartUpgrade(c.Context(), s.upgradeUUID)
	c.Assert(err, tc.ErrorIs, upgradeerrors.NotFound)
}

func (s *serviceSuite) TestActiveUpgrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, nil)

	activeUpgrade, err := s.service.ActiveUpgrade(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activeUpgrade, tc.Equals, s.upgradeUUID)
}

func (s *serviceSuite) TestActiveUpgradeNoUpgrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, errors.Capture(upgradeerrors.NotFound))

	_, err := s.service.ActiveUpgrade(c.Context())
	c.Assert(err, tc.ErrorIs, upgradeerrors.NotFound)
}

func (s *serviceSuite) TestSetDBUpgradeCompleted(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetDBUpgradeCompleted(gomock.Any(), s.upgradeUUID).Return(nil)

	err := s.service.SetDBUpgradeCompleted(c.Context(), s.upgradeUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetDBUpgradeFailed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetDBUpgradeFailed(gomock.Any(), s.upgradeUUID).Return(nil)

	err := s.service.SetDBUpgradeFailed(c.Context(), s.upgradeUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpgradeInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().UpgradeInfo(gomock.Any(), s.upgradeUUID).Return(coreupgrade.Info{}, nil)

	_, err := s.service.UpgradeInfo(c.Context(), s.upgradeUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestWatchForUpgradeReady(c *tc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.watcherFactory.EXPECT().NewNotifyMapperWatcher(gomock.Any(), gomock.Any()).DoAndReturn(func(_ eventsource.Mapper, fo eventsource.FilterOption, _ ...eventsource.FilterOption) (watcher.Watcher[struct{}], error) {
		c.Assert(fo.Namespace(), tc.Equals, "upgrade_info_controller_node")
		return nw, nil
	})

	watcher, err := s.service.WatchForUpgradeReady(c.Context(), s.upgradeUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(watcher, tc.NotNil)
}

func (s *serviceSuite) TestWatchForUpgradeState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.watcherFactory.EXPECT().NewNotifyMapperWatcher(gomock.Any(), gomock.Any()).DoAndReturn(func(_ eventsource.Mapper, fo eventsource.FilterOption, _ ...eventsource.FilterOption) (watcher.Watcher[struct{}], error) {
		c.Assert(fo.Namespace(), tc.Equals, "upgrade_info")
		return nw, nil
	})

	watcher, err := s.service.WatchForUpgradeState(c.Context(), s.upgradeUUID, coreupgrade.Started)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(watcher, tc.NotNil)
}

func (s *serviceSuite) TestIsUpgrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, nil)

	upgrading, err := s.service.IsUpgrading(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(upgrading, tc.IsTrue)
}

func (s *serviceSuite) TestIsUpgradeNoUpgrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, errors.Capture(upgradeerrors.NotFound))

	upgrading, err := s.service.IsUpgrading(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(upgrading, tc.IsFalse)
}

func (s *serviceSuite) TestIsUpgradeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, errors.New("boom"))

	upgrading, err := s.service.IsUpgrading(c.Context())
	c.Assert(err, tc.ErrorMatches, `boom`)
	c.Assert(upgrading, tc.IsFalse)
}
