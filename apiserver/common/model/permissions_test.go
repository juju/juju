// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
)

type PermissionSuite struct {
	testing.BaseSuite
}

func (r *PermissionSuite) TestHasModelAdminSuperUser(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	auth := mocks.NewMockAuthorizer(ctrl)
	auth.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, testing.ControllerTag).Return(nil)

	has, err := model.HasModelAdmin(c.Context(), auth, testing.ControllerTag, testing.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsTrue)
}

func (r *PermissionSuite) TestHasModelAdminYes(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	auth := mocks.NewMockAuthorizer(ctrl)
	auth.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission)
	auth.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, testing.ModelTag).Return(nil)

	has, err := model.HasModelAdmin(c.Context(), auth, testing.ControllerTag, testing.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsTrue)
}

func (r *PermissionSuite) TestHasModelAdminNo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	auth := mocks.NewMockAuthorizer(ctrl)
	auth.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission)
	auth.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, testing.ModelTag).Return(authentication.ErrorEntityMissingPermission)

	has, err := model.HasModelAdmin(c.Context(), auth, testing.ControllerTag, testing.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsFalse)
}

func (r *PermissionSuite) TestHasModelAdminError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	auth := mocks.NewMockAuthorizer(ctrl)
	auth.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission)
	someError := errors.New("error")
	auth.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, testing.ModelTag).Return(someError)

	has, err := model.HasModelAdmin(c.Context(), auth, testing.ControllerTag, testing.ModelTag)
	c.Assert(err, tc.ErrorIs, someError)
	c.Assert(has, tc.IsFalse)
}
