// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/model"
)

type serviceSuite struct {
	testing.IsolationSuite

	modelUUID model.UUID
	userUUID  user.UUID
	service   *Service
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) SetUpSuite(c *gc.C) {
	s.service = NewService(&fakeState{})
}

func (s *serviceSuite) TestCreatePermission(c *gc.C) {

	spec := UserAccessSpec{
		User:       names.NewUserTag("testme"),
		Target:     names.NewCloudTag("aws"),
		Access:     permission.AddModelAccess,
		AccessType: permission.Cloud,
	}
	_, err := s.service.CreatePermission(context.Background(), spec)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreatePermissionError(c *gc.C) {
	spec := UserAccessSpec{
		User:       names.NewUserTag("testme"),
		Target:     names.NewCloudTag("aws"),
		Access:     permission.ReadAccess,
		AccessType: permission.Cloud,
	}
	_, err := s.service.CreatePermission(context.Background(), spec)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestDeletePermission(c *gc.C) {
	err := s.service.DeletePermission(context.Background(), names.NewUserTag("testme"), names.NewCloudTag("aws"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeletePermissionError(c *gc.C) {
	err := s.service.DeletePermission(context.Background(), names.NewUserTag("testme"), names.NewMachineTag("7"))
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestUpdatePermission(c *gc.C) {
	err := s.service.UpdatePermission(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewCloudTag("aws"),
		permission.AddModelAccess)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdatePermissionError(c *gc.C) {
	err := s.service.UpdatePermission(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewUnitTag("aws/0"),
		permission.AddModelAccess)
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadUserAccessForTarget(c *gc.C) {
	_, err := s.service.ReadUserAccessForTarget(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewCloudTag("aws"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessForTargetError(c *gc.C) {
	_, err := s.service.ReadUserAccessForTarget(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewUnitTag("aws/0"))
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadUserAccessLevelForTarget(c *gc.C) {
	_, err := s.service.ReadUserAccessLevelForTarget(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewCloudTag("aws"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessLevelForTargetError(c *gc.C) {
	_, err := s.service.ReadUserAccessForTarget(
		context.Background(),
		names.NewUserTag("testme"),
		names.NewUnitTag("aws/0"))
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForTarget(c *gc.C) {
	_, err := s.service.ReadAllUserAccessForTarget(
		context.Background(),
		names.NewCloudTag("aws"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadAllUserAccessForTargetError(c *gc.C) {
	_, err := s.service.ReadAllUserAccessForTarget(
		context.Background(),
		names.NewUnitTag("aws/0"))
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForUser(c *gc.C) {
	_, err := s.service.ReadAllUserAccessForUser(
		context.Background(),
		names.NewUserTag("testme"))
	c.Assert(err, jc.ErrorIsNil)
}

type fakeState struct {
}

func (f *fakeState) CreatePermission(_ context.Context, _ UserAccessSpec) (permission.UserAccess, error) {
	return permission.UserAccess{}, nil
}

func (f *fakeState) DeletePermission(_ context.Context, _ names.UserTag, _ names.Tag) error {
	return nil
}

func (f *fakeState) UpdatePermission(_ context.Context, _ names.UserTag, _ names.Tag, _ permission.Access) error {
	return nil
}

func (f *fakeState) ReadUserAccessForTarget(_ context.Context, _ names.UserTag, _ names.Tag) (permission.UserAccess, error) {
	return permission.UserAccess{}, nil
}

func (f *fakeState) ReadAllUserAccessForUser(_ context.Context, _ names.UserTag) ([]permission.UserAccess, error) {
	return nil, nil
}

func (f *fakeState) ReadAllUserAccessForTarget(_ context.Context, _ names.Tag) ([]permission.UserAccess, error) {
	return nil, nil
}

func (f *fakeState) ReadAllAccessTypeForUser(_ context.Context, _ names.UserTag, _ permission.AccessType) ([]permission.UserAccess, error) {
	return nil, nil
}

func (f *fakeState) ReadUserAccessLevelForTarget(_ context.Context, _ names.UserTag, _ names.Tag) (permission.Access, error) {
	return permission.ReadAccess, nil
}
