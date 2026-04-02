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
	"github.com/juju/juju/core/user"
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
	c.Cleanup(func() {
		s.accessService = nil
	})
	return ctrl
}

func (s *permissionDelegatorSuite) delegator() *PermissionDelegator {
	return &PermissionDelegator{AccessService: s.accessService}
}

// TestSubjectPermissionsLocalUserSuccess verifies that SubjectPermissions
// correctly returns the access level for a local user.
func (s *permissionDelegatorSuite) TestSubjectPermissionsLocalUserSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	aliceName := tc.Must1(c, user.NewName, "alice@local")
	target := permission.ID{
		ObjectType: permission.Controller,
		Key:        "controller-uuid",
	}
	s.accessService.EXPECT().
		ReadUserAccessLevelForTarget(gomock.Any(), aliceName, target).
		Return(permission.AdminAccess, nil)

	access, err := s.delegator().SubjectPermissions(c.Context(), "alice@local", target)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.AdminAccess)
}

// TestSubjectPermissionsAccessNotFound verifies that AccessNotFound from the
// access service is mapped to PermissionNotFound, which the caller (admin.go)
// treats as "no access" rather than a hard error.
func (s *permissionDelegatorSuite) TestSubjectPermissionsAccessNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	aliceName := tc.Must1(c, user.NewName, "alice@local")
	target := permission.ID{
		ObjectType: permission.Model,
		Key:        "model-uuid",
	}
	s.accessService.EXPECT().
		ReadUserAccessLevelForTarget(gomock.Any(), aliceName, target).
		Return(permission.NoAccess, accesserrors.AccessNotFound)

	access, err := s.delegator().SubjectPermissions(c.Context(), "alice@local", target)
	c.Assert(err, tc.ErrorIs, accesserrors.PermissionNotFound)
	c.Assert(access, tc.Equals, permission.NoAccess)
}

// TestSubjectPermissionsServiceError verifies that unexpected errors from the
// access service are propagated to the caller unchanged.
func (s *permissionDelegatorSuite) TestSubjectPermissionsServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	aliceName := tc.Must1(c, user.NewName, "alice@local")
	target := permission.ID{
		ObjectType: permission.Controller,
		Key:        "controller-uuid",
	}
	svcError := errors.New("database error")
	s.accessService.EXPECT().
		ReadUserAccessLevelForTarget(gomock.Any(), aliceName, target).
		Return(permission.NoAccess, svcError)

	_, err := s.delegator().SubjectPermissions(c.Context(), "alice@local", target)
	c.Assert(err, tc.ErrorIs, svcError)
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

// TestSubjectPermissionsDisabledExternalUser verifies that a disabled external
// user gets PermissionNotFound (via AccessNotFound from the state layer) rather
// than an unexpected error propagation. This is a regression test for a bug
// where disabled external users could inherit everyone@external permissions.
func (s *permissionDelegatorSuite) TestSubjectPermissionsDisabledExternalUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	jimName := tc.Must1(c, user.NewName, "jim@external")
	target := permission.ID{
		ObjectType: permission.Controller,
		Key:        "controller-uuid",
	}
	s.accessService.EXPECT().
		ReadUserAccessLevelForTarget(gomock.Any(), jimName, target).
		Return(permission.NoAccess, accesserrors.AccessNotFound)

	access, err := s.delegator().SubjectPermissions(c.Context(), "jim@external", target)
	c.Check(err, tc.ErrorIs, accesserrors.PermissionNotFound)
	c.Check(access, tc.Equals, permission.NoAccess)
}

// TestSubjectPermissionsRemovedExternalUser verifies that a removed external
// user gets PermissionNotFound (via AccessNotFound from the state layer) rather
// than falling through to everyone@external inheritance.
func (s *permissionDelegatorSuite) TestSubjectPermissionsRemovedExternalUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	jimName := tc.Must1(c, user.NewName, "jim@external")
	target := permission.ID{
		ObjectType: permission.Controller,
		Key:        "controller-uuid",
	}
	s.accessService.EXPECT().
		ReadUserAccessLevelForTarget(gomock.Any(), jimName, target).
		Return(permission.NoAccess, accesserrors.AccessNotFound)

	access, err := s.delegator().SubjectPermissions(c.Context(), "jim@external", target)
	c.Check(err, tc.ErrorIs, accesserrors.PermissionNotFound)
	c.Check(access, tc.Equals, permission.NoAccess)
}

// TestPermissionError verifies that PermissionError always returns ErrPerm,
// regardless of the tag or access level provided.
func (s *permissionDelegatorSuite) TestPermissionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tests := []struct {
		tag    names.Tag
		access permission.Access
	}{
		{names.NewUserTag("alice"), permission.AdminAccess},
		{names.NewUserTag("bob@external"), permission.LoginAccess},
		{names.NewMachineTag("0"), permission.NoAccess},
		{names.NewUserTag("admin"), permission.SuperuserAccess},
	}

	for _, t := range tests {
		err := s.delegator().PermissionError(t.tag, t.access)
		c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
	}
}
