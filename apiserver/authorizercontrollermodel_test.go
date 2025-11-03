// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"testing"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/core/permission"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"
)

// controllerModelAuthorizerSuite provides a set of tests for asserting the
// behaviour of [controllerModelAuthorizer].
type controllerModelAuthorizerSuite struct {
	modelAuthInfo       *MockModelAuthorizationInfo
	permissionDelegator *MockPermissionDelegator
}

// TestControllerModelAuthorizerSuite runs all the test contained within
// [controllerModelAuthorizerSuite].
func TestControllerModelAuthorizerSuite(t *testing.T) {
	tc.Run(t, &controllerModelAuthorizerSuite{})
}

// setupMocks sets up each of the mocks required for this test suite and returns
// the gomock controller.
func (s *controllerModelAuthorizerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelAuthInfo = NewMockModelAuthorizationInfo(ctrl)
	s.permissionDelegator = NewMockPermissionDelegator(ctrl)

	c.Cleanup(func() {
		s.modelAuthInfo = nil
		s.permissionDelegator = nil
	})
	return ctrl
}

// TestNonControllerModelFallThrough is testing that when the authorization
// request is not for the controller model the fall through authorizer is
// delegated to.
func (s *controllerModelAuthorizerSuite) TestNonControllerModelFallThrough(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerUUID := tc.Must(c, uuid.NewUUID)
	controllerTag := names.NewControllerTag(controllerUUID.String())
	userTag := tc.Must1(c, names.ParseUserTag, "user-fred")

	s.modelAuthInfo.EXPECT().IsAuthorizationForControllerModel(gomock.Any()).
		Return(false)

	var fallThroughCalled bool
	var fallThrough authentication.AuthorizerFunc = func(
		context.Context, authentication.AuthInfo,
	) error {
		fallThroughCalled = true
		return nil
	}

	authInfo := authentication.AuthInfo{
		Delegator: s.permissionDelegator,
		Tag:       userTag,
	}

	authorizer := controllerModelPermissionAuthorizer{
		controllerAdminAuthorizer: controllerAdminAuthorizer{
			controllerTag: controllerTag,
		},
		fallThroughAuthroizer:  fallThrough,
		ModelAuthorizationInfo: s.modelAuthInfo,
	}

	err := authorizer.Authorize(c.Context(), authInfo)
	c.Check(err, tc.ErrorIsNil)
	c.Check(fallThroughCalled, tc.IsTrue)
}

// TestSuperUserControllerModelAllowed is testing that when the authorization
// is for the controller model and the user has [permission.SuperuserAccess] on
// the controller authorization is allowed.
func (s *controllerModelAuthorizerSuite) TestSuperUserControllerModelAllowed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerUUID := tc.Must(c, uuid.NewUUID)
	controllerTag := names.NewControllerTag(controllerUUID.String())
	userTag := tc.Must1(c, names.ParseUserTag, "user-fred")

	s.modelAuthInfo.EXPECT().IsAuthorizationForControllerModel(gomock.Any()).
		Return(true)
	s.permissionDelegator.EXPECT().SubjectPermissions(
		gomock.Any(),
		"fred",
		corepermission.ID{
			ObjectType: corepermission.Controller,
			Key:        controllerUUID.String(),
		},
	).Return(permission.SuperuserAccess, nil)

	var fallThrough authentication.AuthorizerFunc = func(
		context.Context, authentication.AuthInfo,
	) error {
		c.Fatal("fall throguh authorizer should never have been called")
		return nil
	}

	authInfo := authentication.AuthInfo{
		Delegator: s.permissionDelegator,
		Tag:       userTag,
	}

	authorizer := controllerModelPermissionAuthorizer{
		controllerAdminAuthorizer: controllerAdminAuthorizer{
			controllerTag: controllerTag,
		},
		fallThroughAuthroizer:  fallThrough,
		ModelAuthorizationInfo: s.modelAuthInfo,
	}

	err := authorizer.Authorize(c.Context(), authInfo)
	c.Check(err, tc.ErrorIsNil)
}

// TestNonSuperUserPermissionNotAllowed is testing that for any other permission
// set in the controller that is not [permission.SuperuserAccess] the user is
// not authorized.
func (s *controllerModelAuthorizerSuite) TestNonSuperUserPermissionNotAllowed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := tc.Must(c, uuid.NewUUID)
	controllerTag := names.NewControllerTag(controllerUUID.String())
	userTag := tc.Must1(c, names.ParseUserTag, "user-fred")

	s.modelAuthInfo.EXPECT().IsAuthorizationForControllerModel(gomock.Any()).
		Return(true).AnyTimes()
	var fallThrough authentication.AuthorizerFunc = func(
		context.Context, authentication.AuthInfo,
	) error {
		c.Fatal("fall throguh authorizer should never have been called")
		return nil
	}

	notAllowedPermissions := []corepermission.Access{
		corepermission.NoAccess,
		corepermission.ReadAccess,
		corepermission.WriteAccess,
		corepermission.AdminAccess,
		corepermission.LoginAccess,
		corepermission.AddModelAccess,
		corepermission.ConsumeAccess,
	}

	// Test that each not allowed permission does not authorize for the
	// controller model.
	for _, perm := range notAllowedPermissions {
		c.Run(perm.String(), func(t *testing.T) {
			// Each sub tests gets a new delegator to make sure there is no
			// cross over.
			delegator := NewMockPermissionDelegator(ctrl)
			delegator.EXPECT().SubjectPermissions(
				gomock.Any(),
				"fred",
				corepermission.ID{
					ObjectType: corepermission.Controller,
					Key:        controllerUUID.String(),
				},
			).Return(perm, nil)

			authInfo := authentication.AuthInfo{
				Delegator: delegator,
				Tag:       userTag,
			}

			authorizer := controllerModelPermissionAuthorizer{
				controllerAdminAuthorizer: controllerAdminAuthorizer{
					controllerTag: controllerTag,
				},
				fallThroughAuthroizer:  fallThrough,
				ModelAuthorizationInfo: s.modelAuthInfo,
			}

			err := authorizer.Authorize(c.Context(), authInfo)
			c.Check(err, tc.NotNil)
		})
	}
}
