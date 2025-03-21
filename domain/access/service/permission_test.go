// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	corepermission "github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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
	s.state.EXPECT().CreatePermission(gomock.Any(), gomock.AssignableToTypeOf(uuid.UUID{}), gomock.AssignableToTypeOf(corepermission.UserAccessSpec{})).Return(corepermission.UserAccess{}, nil)

	spec := corepermission.UserAccessSpec{
		User: usertesting.GenNewName(c, "testme"),
		AccessSpec: corepermission.AccessSpec{
			Target: corepermission.ID{
				ObjectType: corepermission.Cloud,
				Key:        "aws",
			},
			Access: corepermission.AddModelAccess,
		},
	}
	_, err := NewService(s.state).CreatePermission(context.Background(), spec)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreatePermissionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	spec := corepermission.UserAccessSpec{
		User: usertesting.GenNewName(c, "testme"),
		AccessSpec: corepermission.AccessSpec{
			Target: corepermission.ID{
				ObjectType: corepermission.Cloud,
				Key:        "aws",
			},
			Access: corepermission.ReadAccess,
		},
	}
	_, err := NewService(s.state).CreatePermission(context.Background(), spec)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestDeletePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().DeletePermission(gomock.Any(), usertesting.GenNewName(c, "testme"), gomock.AssignableToTypeOf(corepermission.ID{})).Return(nil)
	err := NewService(s.state).DeletePermission(context.Background(), usertesting.GenNewName(c, "testme"), corepermission.ID{
		ObjectType: corepermission.Cloud,
		Key:        "aws",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeletePermissionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).DeletePermission(context.Background(), usertesting.GenNewName(c, "testme"), corepermission.ID{
		ObjectType: "faileme",
		Key:        "aws",
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestUpsertPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().UpdatePermission(gomock.Any(), gomock.AssignableToTypeOf(access.UpdatePermissionArgs{})).Return(nil)

	err := NewService(s.state).UpdatePermission(
		context.Background(),
		access.UpdatePermissionArgs{
			AccessSpec: corepermission.AccessSpec{
				Access: corepermission.AddModelAccess,
				Target: corepermission.ID{
					ObjectType: corepermission.Cloud,
					Key:        "aws",
				},
			},
			Change:  corepermission.Grant,
			Subject: usertesting.GenNewName(c, "testme"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadUserAccessForTarget(gomock.Any(), usertesting.GenNewName(c, "testme"), gomock.AssignableToTypeOf(corepermission.ID{})).Return(corepermission.UserAccess{}, nil)

	_, err := NewService(s.state).ReadUserAccessForTarget(
		context.Background(),
		usertesting.GenNewName(c, "testme"),
		corepermission.ID{
			ObjectType: corepermission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadUserAccessForTarget(
		context.Background(),
		usertesting.GenNewName(c, "testme"),
		corepermission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(errors.Is(err, coreerrors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadUserAccessLevelForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "testme"), gomock.AssignableToTypeOf(corepermission.ID{})).Return(corepermission.NoAccess, nil)

	_, err := NewService(s.state).ReadUserAccessLevelForTarget(
		context.Background(),
		usertesting.GenNewName(c, "testme"),
		corepermission.ID{
			ObjectType: corepermission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessLevelForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadUserAccessForTarget(
		context.Background(),
		usertesting.GenNewName(c, "testme"),
		corepermission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllUserAccessForTarget(gomock.Any(), gomock.AssignableToTypeOf(corepermission.ID{})).Return(nil, nil)

	_, err := NewService(s.state).ReadAllUserAccessForTarget(
		context.Background(),
		corepermission.ID{
			ObjectType: corepermission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadAllUserAccessForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadAllUserAccessForTarget(
		context.Background(),
		corepermission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllUserAccessForUser(gomock.Any(), usertesting.GenNewName(c, "testme")).Return(nil, nil)

	_, err := NewService(s.state).ReadAllUserAccessForUser(
		context.Background(),
		usertesting.GenNewName(c, "testme"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadAllAccessForUserAndObjectType(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllAccessForUserAndObjectType(gomock.Any(), usertesting.GenNewName(c, "testme"), corepermission.Cloud).Return(nil, nil)

	_, err := NewService(s.state).ReadAllAccessForUserAndObjectType(
		context.Background(),
		usertesting.GenNewName(c, "testme"),
		corepermission.Cloud)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadAllAccessForUserAndObjectTypeError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadAllAccessForUserAndObjectType(
		context.Background(),
		usertesting.GenNewName(c, "testme"),
		"failme")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestAllModelAccessForCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().AllModelAccessForCloudCredential(gomock.Any(), gomock.AssignableToTypeOf(credential.Key{})).Return(nil, nil)

	_, err := NewService(s.state).AllModelAccessForCloudCredential(
		context.Background(),
		credential.Key{})
	c.Assert(err, jc.ErrorIsNil)
}
