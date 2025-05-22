// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	corepermission "github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state *MockState
}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) TestCreatePermission(c *tc.C) {
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
	_, err := NewService(s.state).CreatePermission(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestCreatePermissionError(c *tc.C) {
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
	_, err := NewService(s.state).CreatePermission(c.Context(), spec)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestDeletePermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().DeletePermission(gomock.Any(), usertesting.GenNewName(c, "testme"), gomock.AssignableToTypeOf(corepermission.ID{})).Return(nil)
	err := NewService(s.state).DeletePermission(c.Context(), usertesting.GenNewName(c, "testme"), corepermission.ID{
		ObjectType: corepermission.Cloud,
		Key:        "aws",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestDeletePermissionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).DeletePermission(c.Context(), usertesting.GenNewName(c, "testme"), corepermission.ID{
		ObjectType: "faileme",
		Key:        "aws",
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("%+v", err))
}

func (s *serviceSuite) TestUpsertPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().UpdatePermission(gomock.Any(), gomock.AssignableToTypeOf(access.UpdatePermissionArgs{})).Return(nil)

	err := NewService(s.state).UpdatePermission(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessForTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadUserAccessForTarget(gomock.Any(), usertesting.GenNewName(c, "testme"), gomock.AssignableToTypeOf(corepermission.ID{})).Return(corepermission.UserAccess{}, nil)

	_, err := NewService(s.state).ReadUserAccessForTarget(
		c.Context(),
		usertesting.GenNewName(c, "testme"),
		corepermission.ID{
			ObjectType: corepermission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessForTargetError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadUserAccessForTarget(
		c.Context(),
		usertesting.GenNewName(c, "testme"),
		corepermission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(errors.Is(err, coreerrors.NotValid), tc.IsTrue, tc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadUserAccessLevelForTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "testme"), gomock.AssignableToTypeOf(corepermission.ID{})).Return(corepermission.NoAccess, nil)

	_, err := NewService(s.state).ReadUserAccessLevelForTarget(
		c.Context(),
		usertesting.GenNewName(c, "testme"),
		corepermission.ID{
			ObjectType: corepermission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessLevelForTargetError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadUserAccessForTarget(
		c.Context(),
		usertesting.GenNewName(c, "testme"),
		corepermission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllUserAccessForTarget(gomock.Any(), gomock.AssignableToTypeOf(corepermission.ID{})).Return(nil, nil)

	_, err := NewService(s.state).ReadAllUserAccessForTarget(
		c.Context(),
		corepermission.ID{
			ObjectType: corepermission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestReadAllUserAccessForTargetError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadAllUserAccessForTarget(
		c.Context(),
		corepermission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForUser(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllUserAccessForUser(gomock.Any(), usertesting.GenNewName(c, "testme")).Return(nil, nil)

	_, err := NewService(s.state).ReadAllUserAccessForUser(
		c.Context(),
		usertesting.GenNewName(c, "testme"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestReadAllAccessForUserAndObjectType(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllAccessForUserAndObjectType(gomock.Any(), usertesting.GenNewName(c, "testme"), corepermission.Cloud).Return(nil, nil)

	_, err := NewService(s.state).ReadAllAccessForUserAndObjectType(
		c.Context(),
		usertesting.GenNewName(c, "testme"),
		corepermission.Cloud)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestReadAllAccessForUserAndObjectTypeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadAllAccessForUserAndObjectType(
		c.Context(),
		usertesting.GenNewName(c, "testme"),
		"failme")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("%+v", err))
}

func (s *serviceSuite) TestAllModelAccessForCloudCredential(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().AllModelAccessForCloudCredential(gomock.Any(), gomock.AssignableToTypeOf(credential.Key{})).Return(nil, nil)

	_, err := NewService(s.state).AllModelAccessForCloudCredential(
		c.Context(),
		credential.Key{})
	c.Assert(err, tc.ErrorIsNil)
}
