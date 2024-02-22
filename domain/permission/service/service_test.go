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

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/permission/state"
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
	s.state.EXPECT().CreatePermission(gomock.Any(), gomock.AssignableToTypeOf(state.UserAccessSpec{})).Return(permission.UserAccess{}, nil)

	spec := UserAccessSpec{
		User: "testme",
		Target: permission.ID{
			ObjectType: permission.Cloud,
			Key:        "aws",
		},
		Access: permission.AddModelAccess,
	}
	_, err := NewService(s.state).CreatePermission(context.Background(), spec)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreatePermissionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	spec := UserAccessSpec{
		User: "testme",
		Target: permission.ID{
			ObjectType: permission.Cloud,
			Key:        "aws",
		},
		Access: permission.ReadAccess,
	}
	_, err := NewService(s.state).CreatePermission(context.Background(), spec)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestDeletePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().DeletePermission(gomock.Any(), "testme", gomock.AssignableToTypeOf(permission.ID{})).Return(nil)
	err := NewService(s.state).DeletePermission(context.Background(), "testme", permission.ID{
		ObjectType: permission.Cloud,
		Key:        "aws",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeletePermissionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).DeletePermission(context.Background(), "testme", permission.ID{
		ObjectType: "faileme",
		Key:        "aws",
	})
	c.Assert(err, jc.ErrorIs, errors.NotValid, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestUpsertPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().UpsertPermission(gomock.Any(), gomock.AssignableToTypeOf(state.UpsertPermissionArgs{})).Return(nil)

	err := NewService(s.state).UpsertPermission(
		context.Background(),
		UpsertPermissionArgs{
			Access:  permission.AddModelAccess,
			AddUser: false,
			ApiUser: "admin",
			Change:  Grant,
			Subject: "testme",
			Target: permission.ID{
				ObjectType: permission.Cloud,
				Key:        "aws",
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpsertPermissionArgsValidationFail(c *gc.C) {
	argsToTest := []UpsertPermissionArgs{
		{}, { // Missing Subject
			ApiUser: "admin",
		}, { // Missing Target
			ApiUser: "admin",
			Subject: "testme",
		}, { // Target and Access don't mesh
			Access:  permission.AddModelAccess,
			ApiUser: "admin",
			Subject: "testme",
			Target: permission.ID{
				ObjectType: permission.Cloud,
				Key:        "aws",
			},
		}, { // Invalid Change
			Access:  permission.AddModelAccess,
			ApiUser: "admin",
			Change:  "testing",
			Subject: "testme",
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        "aws",
			},
		}}
	for i, args := range argsToTest {
		c.Logf("Test %d", i)
		c.Check(args.validate(), jc.ErrorIs, errors.NotValid)
	}
}

func (s *serviceSuite) TestReadUserAccessForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadUserAccessForTarget(gomock.Any(), "testme", gomock.AssignableToTypeOf(permission.ID{})).Return(permission.UserAccess{}, nil)

	_, err := NewService(s.state).ReadUserAccessForTarget(
		context.Background(),
		"testme",
		permission.ID{
			ObjectType: permission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadUserAccessForTarget(
		context.Background(),
		"testme",
		permission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadUserAccessLevelForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), "testme", gomock.AssignableToTypeOf(permission.ID{})).Return(permission.NoAccess, nil)

	_, err := NewService(s.state).ReadUserAccessLevelForTarget(
		context.Background(),
		"testme",
		permission.ID{
			ObjectType: permission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadUserAccessLevelForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadUserAccessForTarget(
		context.Background(),
		"testme",
		permission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIs, errors.NotValid, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllUserAccessForTarget(gomock.Any(), gomock.AssignableToTypeOf(permission.ID{})).Return(nil, nil)

	_, err := NewService(s.state).ReadAllUserAccessForTarget(
		context.Background(),
		permission.ID{
			ObjectType: permission.Cloud,
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReadAllUserAccessForTargetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).ReadAllUserAccessForTarget(
		context.Background(),
		permission.ID{
			ObjectType: "faileme",
			Key:        "aws",
		})
	c.Assert(err, jc.ErrorIs, errors.NotValid, gc.Commentf("%+v", err))
}

func (s *serviceSuite) TestReadAllUserAccessForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ReadAllUserAccessForUser(gomock.Any(), "testme").Return(nil, nil)

	_, err := NewService(s.state).ReadAllUserAccessForUser(
		context.Background(),
		"testme")
	c.Assert(err, jc.ErrorIsNil)
}
