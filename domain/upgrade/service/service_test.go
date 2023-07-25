// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"github.com/mattn/go-sqlite3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type serviceSuite struct {
	state *MockState

	srv *Service
}

var _ = gc.Suite(&serviceSuite{})

var (
	testUUID1 = utils.MustNewUUID().String()
	testUUID2 = utils.MustNewUUID().String()
)

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.srv = NewService(s.state, nil)
	return ctrl
}

func (s *serviceSuite) TestCreateUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateUpgrade(gomock.Any(), version.MustParse("3.0.0"), version.MustParse("3.0.1")).Return(testUUID1, nil)

	upgradeUUID, err := s.srv.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(upgradeUUID, gc.Equals, testUUID1)
}

func (s *serviceSuite) TestCreateUpgradeAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ucErr := sqlite3.Error{ExtendedCode: sqlite3.ErrConstraintUnique}
	s.state.EXPECT().CreateUpgrade(gomock.Any(), version.MustParse("3.0.0"), version.MustParse("3.0.1")).Return("", ucErr)

	_, err := s.srv.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)
}

func (s *serviceSuite) TestCreateUpgradeInvalidVersions(c *gc.C) {
	_, err := s.srv.CreateUpgrade(context.Background(), version.MustParse("3.0.1"), version.MustParse("3.0.0"))
	c.Assert(errors.IsNotValid(err), jc.IsTrue)

	_, err = s.srv.CreateUpgrade(context.Background(), version.MustParse("3.0.1"), version.MustParse("3.0.1"))
	c.Assert(errors.IsNotValid(err), jc.IsTrue)
}

func (s *serviceSuite) TestSetControllerReady(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetControllerReady(gomock.Any(), testUUID1, testUUID2).Return(nil)

	err := s.srv.SetControllerReady(context.Background(), testUUID1, testUUID2)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetControllerReadyForiegnKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fkErr := sqlite3.Error{ExtendedCode: sqlite3.ErrConstraintForeignKey}
	s.state.EXPECT().SetControllerReady(gomock.Any(), testUUID1, testUUID2).Return(fkErr)

	err := s.srv.SetControllerReady(context.Background(), testUUID1, testUUID2)
	c.Log(err)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *serviceSuite) TestStartUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StartUpgrade(gomock.Any(), testUUID1).Return(nil)

	err := s.srv.StartUpgrade(context.Background(), testUUID1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestStartUpgradeBeforeCreated(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StartUpgrade(gomock.Any(), testUUID1).Return(sql.ErrNoRows)

	err := s.srv.StartUpgrade(context.Background(), testUUID1)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *serviceSuite) TestActiveUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return(testUUID1, nil)

	activeUpgrade, err := s.srv.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeUpgrade, gc.Equals, testUUID1)
}

func (s *serviceSuite) TestActiveUpgradeNoUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ActiveUpgrade(gomock.Any()).Return("", errors.Trace(sql.ErrNoRows))

	_, err := s.srv.ActiveUpgrade(context.Background())
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}
