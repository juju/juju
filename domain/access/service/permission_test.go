// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockPermissionState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockPermissionState(ctrl)
	return ctrl
}

func (s *serviceSuite) TestCreatePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().CreatePermission(gomock.Any(), gomock.AssignableToTypeOf(uuid.UUID{}), gomock.AssignableToTypeOf(corepermission.UserAccessSpec{})).Return(corepermission.UserAccess{}, nil)

	spec := corepermission.UserAccessSpec{
		User: "testme",
		AccessSpec: corepermission.AccessSpec{
			Target: corepermission.ID{
				ObjectType: corepermission.Cloud,
				Key:        "aws",
			},
			Access: corepermission.AddModelAccess,
		},
	}
	_, err := NewPermissionService(s.state).CreatePermission(context.Background(), spec)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreatePermissionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	spec := corepermission.UserAccessSpec{
		User: "testme",
		AccessSpec: corepermission.AccessSpec{
			Target: corepermission.ID{
				ObjectType: corepermission.Cloud,
				Key:        "aws",
			},
			Access: corepermission.ReadAccess,
		},
	}
	_, err := NewPermissionService(s.state).CreatePermission(context.Background(), spec)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestDeletePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().DeletePermission(gomock.Any(), "testme", gomock.AssignableToTypeOf(corepermission.ID{})).Return(nil)
	err := NewPermissionService(s.state).DeletePermission(context.Background(), "testme", corepermission.ID{
		ObjectType: corepermission.Cloud,
		Key:        "aws",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeletePermissionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewPermissionService(s.state).DeletePermission(context.Background(), "testme", corepermission.ID{
		ObjectType: "faileme",
		Key:        "aws",
	})
	c.Assert(err, jc.ErrorIs, errors.NotValid, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestUpsertPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().UpsertPermission(gomock.Any(), gomock.AssignableToTypeOf(access.UpsertPermissionArgs{})).Return(nil)

	err := NewPermissionService(s.state).UpsertPermission(
		context.Background(),
		access.UpsertPermissionArgs{
			Access:  corepermission.AddModelAccess,
			AddUser: false,
			ApiUser: "admin",
			Change:  corepermission.Grant,
			Subject: "testme",
			Target: corepermission.ID{
				ObjectType: corepermission.Cloud,
				Key:        "aws",
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadUserAccessForTarget(gomock.Any(), "testme", gomock.AssignableToTypeOf(corepermission.ID{})).Return(corepermission.UserAccess{}, nil)

	_, err := NewPermissionService(s.state).ReadUserAccessForTarget(
		context.Background(),
		"testme",
		corepermission.ID{
			ObjectType: corepermission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewPermissionService(s.state).ReadUserAccessForTarget(
		context.Background(),
		"testme",
		corepermission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadUserAccessLevelForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), "testme", gomock.AssignableToTypeOf(corepermission.ID{})).Return(corepermission.NoAccess, nil)

	_, err := NewPermissionService(s.state).ReadUserAccessLevelForTarget(
		context.Background(),
		"testme",
		corepermission.ID{
			ObjectType: corepermission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessLevelForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewPermissionService(s.state).ReadUserAccessForTarget(
		context.Background(),
		"testme",
		corepermission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIs, errors.NotValid, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllUserAccessForTarget(gomock.Any(), gomock.AssignableToTypeOf(corepermission.ID{})).Return(nil, nil)

	_, err := NewPermissionService(s.state).ReadAllUserAccessForTarget(
		context.Background(),
		corepermission.ID{
			ObjectType: corepermission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadAllUserAccessForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewPermissionService(s.state).ReadAllUserAccessForTarget(
		context.Background(),
		corepermission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIs, errors.NotValid, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllUserAccessForUser(gomock.Any(), "testme").Return(nil, nil)

	_, err := NewPermissionService(s.state).ReadAllUserAccessForUser(
		context.Background(),
		"testme")
	c.Assert(err, jc.ErrorIsNil)
}
