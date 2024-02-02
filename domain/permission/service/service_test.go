// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) TestCreatePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().CreatePermission(gomock.Any(), gomock.AssignableToTypeOf(UserAccessSpec{})).Return(permission.UserAccess{}, nil)

	spec := UserAccessSpec{
		User:       names.NewUserTag("testme"),
		Target:     names.NewCloudTag("aws"),
		Access:     permission.AddModelAccess,
		AccessType: permission.Cloud,
	}
	_, err := NewService(s.state).CreatePermission(context.Background(), spec)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreatePermissionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	spec := UserAccessSpec{
		User:       names.NewUserTag("testme"),
		Target:     names.NewCloudTag("aws"),
		Access:     permission.ReadAccess,
		AccessType: permission.Cloud,
	}
	_, err := NewService(s.state).CreatePermission(context.Background(), spec)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

var targetTagCond = func(obtained any) bool {
	_, ok1 := obtained.(names.CloudTag)
	_, ok2 := obtained.(names.ControllerTag)
	_, ok3 := obtained.(names.ModelTag)
	_, ok4 := obtained.(names.ApplicationOfferTag)
	return ok1 || ok2 || ok3 || ok4
}

func (s *serviceSuite) TestDeletePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().DeletePermission(gomock.Any(), gomock.AssignableToTypeOf(names.UserTag{}), gomock.Cond(targetTagCond)).Return(nil)

	err := NewService(s.state).DeletePermission(context.Background(), names.NewUserTag("testme"), names.NewCloudTag("aws"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeletePermissionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).DeletePermission(context.Background(), names.NewUserTag("testme"), names.NewMachineTag("7"))
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestUpdatePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().UpdatePermission(gomock.Any(), gomock.AssignableToTypeOf(names.UserTag{}), gomock.Cond(targetTagCond), permission.AddModelAccess).Return(nil)

	err := NewService(s.state).UpdatePermission(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewCloudTag("aws"),
		permission.AddModelAccess)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdatePermissionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).UpdatePermission(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewUnitTag("aws/0"),
		permission.AddModelAccess)
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadUserAccessForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadUserAccessForTarget(gomock.Any(), gomock.AssignableToTypeOf(names.UserTag{}), gomock.Cond(targetTagCond)).Return(permission.UserAccess{}, nil)

	_, err := NewService(s.state).ReadUserAccessForTarget(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewCloudTag("aws"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadUserAccessForTarget(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewUnitTag("aws/0"))
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadUserAccessLevelForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), gomock.AssignableToTypeOf(names.UserTag{}), gomock.Cond(targetTagCond)).Return(permission.NoAccess, nil)

	_, err := NewService(s.state).ReadUserAccessLevelForTarget(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewCloudTag("aws"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessLevelForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadUserAccessForTarget(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewUnitTag("aws/0"))
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllUserAccessForTarget(gomock.Any(), gomock.Cond(targetTagCond)).Return(nil, nil)

	_, err := NewService(s.state).ReadAllUserAccessForTarget(
		context.Background(),
		names.NewCloudTag("aws"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadAllUserAccessForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadAllUserAccessForTarget(
		context.Background(),
		names.NewUnitTag("aws/0"))
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllUserAccessForUser(gomock.Any(), gomock.AssignableToTypeOf(names.UserTag{})).Return(nil, nil)

	_, err := NewService(s.state).ReadAllUserAccessForUser(
		context.Background(),
		names.NewUserTag("testme"))
	c.Assert(err, jc.ErrorIsNil)
}
