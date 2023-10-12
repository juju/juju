// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/mattn/go-sqlite3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreupgrade "github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher/watchertest"
)

type serviceSuite struct {
	baseServiceSuite

	state          *MockState
	watcherFactory *MockWatcherFactory

	srv *Service
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	s.srv = NewService(s.state, s.watcherFactory)
	return ctrl
}

func (s *serviceSuite) TestCreateUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateUpgrade(gomock.Any(), version.MustParse("3.0.0"), version.MustParse("3.0.1")).Return(s.upgradeUUID, nil)

	upgradeUUID, err := s.srv.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(upgradeUUID, gc.Equals, s.upgradeUUID)
}

func (s *serviceSuite) TestCreateUpgradeAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ucErr := sqlite3.Error{ExtendedCode: sqlite3.ErrConstraintUnique}
	s.state.EXPECT().CreateUpgrade(gomock.Any(), version.MustParse("3.0.0"), version.MustParse("3.0.1")).Return(s.upgradeUUID, ucErr)

	_, err := s.srv.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
}

func (s *serviceSuite) TestCreateUpgradeInvalidVersions(c *gc.C) {
	_, err := s.srv.CreateUpgrade(context.Background(), version.MustParse("3.0.1"), version.MustParse("3.0.0"))
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	_, err = s.srv.CreateUpgrade(context.Background(), version.MustParse("3.0.1"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestSetControllerReady(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetControllerReady(gomock.Any(), s.upgradeUUID, s.controllerUUID).Return(nil)

	err := s.srv.SetControllerReady(context.Background(), s.upgradeUUID, s.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetControllerReadyForeignKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fkErr := sqlite3.Error{ExtendedCode: sqlite3.ErrConstraintForeignKey}
	s.state.EXPECT().SetControllerReady(gomock.Any(), s.upgradeUUID, s.controllerUUID).Return(fkErr)

	err := s.srv.SetControllerReady(context.Background(), s.upgradeUUID, s.controllerUUID)
	c.Log(err)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestStartUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StartUpgrade(gomock.Any(), s.upgradeUUID).Return(nil)

	err := s.srv.StartUpgrade(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestStartUpgradeBeforeCreated(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StartUpgrade(gomock.Any(), s.upgradeUUID).Return(sql.ErrNoRows)

	err := s.srv.StartUpgrade(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestActiveUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, nil)

	activeUpgrade, err := s.srv.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeUpgrade, gc.Equals, s.upgradeUUID)
}

func (s *serviceSuite) TestActiveUpgradeNoUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(s.upgradeUUID, errors.Trace(sql.ErrNoRows))

	_, err := s.srv.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestCompleteDBUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetDBUpgradeCompleted(gomock.Any(), s.upgradeUUID).Return(nil)

	err := s.srv.SetDBUpgradeCompleted(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpgradeInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().UpgradeInfo(gomock.Any(), s.upgradeUUID).Return(coreupgrade.Info{}, nil)

	_, err := s.srv.UpgradeInfo(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestWatchForUpgradeReady(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.watcherFactory.EXPECT().NewValuePredicateWatcher(gomock.Any(), s.upgradeUUID.String(), gomock.Any(), gomock.Any()).Return(nw, nil)

	watcher, err := s.srv.WatchForUpgradeReady(context.Background(), s.upgradeUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)
}
