// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/uuid"
)

// controllerAdminAuthorizerSuite exists to form a set of contract tests for the
// [controllerAdminAuthorizer].
type controllerAdminAuthorizerSuite struct {
	permissionDelegator *MockPermissionDelegator
}

// TestControllerAdminAuthorizerSuite runs all of the tests that are apart of
// the [controllerAdminAuthorizerSuite].
func TestControllerAdminAuthorizerSuite(t *testing.T) {
	tc.Run(t, &controllerAdminAuthorizerSuite{})
}

func (s *controllerAdminAuthorizerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.permissionDelegator = NewMockPermissionDelegator(ctrl)

	c.Cleanup(func() {
		s.permissionDelegator = nil
	})
	return ctrl
}

// TestNotAUser is testing that when using a [controllerAdminAuthorizer] for an
// entity that is not a user no further processing is done.
//
// This test checks for a not supported error from the func but it should not be
// considered part of the formal contract.
//
// We want to see an immediate error here and that the mocks have no expected
// call failures. If this test develops unexpected mock call failures it is
// a good sign that that these new calls need to be moved below basic constraint
// checks such as this one.
func (s *controllerAdminAuthorizerSuite) TestNotAUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerUUID := tc.Must(c, uuid.NewUUID)
	controllerTag := names.NewControllerTag(controllerUUID.String())
	modelUUID := tc.Must(c, coremodel.NewUUID)

	// a set of known tags that are not supported by the authorizer.
	notSupportEntityTags := []names.Tag{
		names.NewApplicationTag("foo"),
		names.NewControllerAgentTag("123"),
		names.NewMachineTag("0"),
		names.NewModelTag(modelUUID.String()),
		names.NewUnitTag("foo/0"),
	}

	for _, tag := range notSupportEntityTags {
		c.Run(tag.String(), func(c *testing.T) {
			authInfo := authentication.AuthInfo{
				Delegator: s.permissionDelegator,
				Tag:       tag,
			}

			authorizer := controllerAdminAuthorizer{
				controllerTag: controllerTag,
			}

			err := authorizer.Authorize(c.Context(), authInfo)
			tc.Check(c, err, tc.ErrorIs, coreerrors.NotSupported)
		})
	}
}

// TestNonSuperUserPermissionNotAllowed is testing that for any other permission
// set in the controller that is not super user the user is not authorized.
func (s *controllerAdminAuthorizerSuite) TestNonSuperUserPermissionNotAllowed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := tc.Must(c, uuid.NewUUID)
	controllerTag := names.NewControllerTag(controllerUUID.String())
	userTag := tc.Must1(c, names.ParseUserTag, "user-fred")

	notAllowedPermissions := []corepermission.Access{
		corepermission.NoAccess,
		corepermission.ReadAccess,
		corepermission.WriteAccess,
		corepermission.AdminAccess,
		corepermission.LoginAccess,
		corepermission.AddModelAccess,
		corepermission.ConsumeAccess,
	}

	for _, perm := range notAllowedPermissions {
		c.Run(perm.String(), func(c *testing.T) {
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

			authorizer := controllerAdminAuthorizer{
				controllerTag: controllerTag,
			}

			err := authorizer.Authorize(c.Context(), authInfo)
			tc.Check(c, err, tc.NotNil)
		})
	}
}

// TestSuperUserPermissionAllowed tests that when a user has the super user
// permission that authorize without error through the
// [controllerAdminAuthorizer].
func (s *controllerAdminAuthorizerSuite) TestSuperUserPermissionAllowed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := tc.Must(c, uuid.NewUUID)
	controllerTag := names.NewControllerTag(controllerUUID.String())
	userTag := tc.Must1(c, names.ParseUserTag, "user-fred")

	s.permissionDelegator.EXPECT().SubjectPermissions(
		gomock.Any(),
		"fred",
		corepermission.ID{
			ObjectType: corepermission.Controller,
			Key:        controllerUUID.String(),
		},
	).Return(corepermission.SuperuserAccess, nil)

	authInfo := authentication.AuthInfo{
		Delegator: s.permissionDelegator,
		Tag:       userTag,
	}

	authorizer := controllerAdminAuthorizer{
		controllerTag: controllerTag,
	}

	err := authorizer.Authorize(c.Context(), authInfo)
	c.Check(err, tc.ErrorIsNil)
}
