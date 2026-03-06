// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/permission"
	accesserrors "github.com/juju/juju/domain/access/errors"
)

type permissionDelegatorSuite struct {
	accessService *MockAccessService
}

func TestPermissionDelegatorSuite(t *stdtesting.T) {
	tc.Run(t, &permissionDelegatorSuite{})
}

func (s *permissionDelegatorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.accessService = NewMockAccessService(ctrl)
	return ctrl
}

func (s *permissionDelegatorSuite) delegator() *PermissionDelegator {
	return &PermissionDelegator{AccessService: s.accessService}
}

// TestSubjectPermissionsLocalUserSuccess verifies that SubjectPermissions
// correctly returns the access level for a local user.
func (s *permissionDelegatorSuite) TestSubjectPermissionsLocalUserSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := permission.ID{
		ObjectType: permission.Controller,
		Key:        "controller-uuid",
	}
	s.accessService.EXPECT().
		ReadUserAccessLevelForTarget(gomock.Any(), gomock.Any(), target).
		Return(permission.AdminAccess, nil)

	access, err := s.delegator().SubjectPermissions(c.Context(), "alice@local", target)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.AdminAccess)
}

// TestSubjectPermissionsExternalUserIsAPureRead verifies that SubjectPermissions
// for an external user is a pure read that never calls EnsureExternalUser.
// User creation is now an explicit step in admin.authenticate(), not a
// side-effect of permission reading. If EnsureExternalUser were called
// here, gomock would report an unexpected call.
func (s *permissionDelegatorSuite) TestSubjectPermissionsExternalUserIsAPureRead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := permission.ID{
		ObjectType: permission.Controller,
		Key:        "controller-uuid",
	}
	s.accessService.EXPECT().
		ReadUserAccessLevelForTarget(gomock.Any(), gomock.Any(), target).
		Return(permission.LoginAccess, nil)

	access, err := s.delegator().SubjectPermissions(c.Context(), "foo@external", target)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.LoginAccess)
}

// TestSubjectPermissionsAccessNotFound verifies that AccessNotFound from the
// access service is mapped to PermissionNotFound, which the caller (admin.go)
// treats as "no access" rather than a hard error.
func (s *permissionDelegatorSuite) TestSubjectPermissionsAccessNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := permission.ID{
		ObjectType: permission.Model,
		Key:        "model-uuid",
	}
	s.accessService.EXPECT().
		ReadUserAccessLevelForTarget(gomock.Any(), gomock.Any(), target).
		Return(permission.NoAccess, accesserrors.AccessNotFound)

	access, err := s.delegator().SubjectPermissions(c.Context(), "alice@local", target)
	c.Assert(err, tc.ErrorIs, accesserrors.PermissionNotFound)
	c.Assert(access, tc.Equals, permission.NoAccess)
}

// TestSubjectPermissionsServiceError verifies that unexpected errors from the
// access service are propagated to the caller unchanged.
func (s *permissionDelegatorSuite) TestSubjectPermissionsServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := permission.ID{
		ObjectType: permission.Controller,
		Key:        "controller-uuid",
	}
	s.accessService.EXPECT().
		ReadUserAccessLevelForTarget(gomock.Any(), gomock.Any(), target).
		Return(permission.NoAccess, errors.New("database error"))

	_, err := s.delegator().SubjectPermissions(c.Context(), "alice@local", target)
	c.Assert(err, tc.ErrorMatches, "database error")
}

// TestSubjectPermissionsInvalidUserName verifies that an invalid user name
// string is rejected before reaching the access service.
func (s *permissionDelegatorSuite) TestSubjectPermissionsInvalidUserName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := permission.ID{
		ObjectType: permission.Controller,
		Key:        "controller-uuid",
	}

	_, err := s.delegator().SubjectPermissions(c.Context(), "not a valid user!!!", target)
	c.Assert(err, tc.NotNil)
}

// TestPermissionError verifies that PermissionError always returns ErrPerm,
// regardless of the tag or access level provided.
func (s *permissionDelegatorSuite) TestPermissionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.delegator().PermissionError(names.NewUserTag("alice"), permission.AdminAccess)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

