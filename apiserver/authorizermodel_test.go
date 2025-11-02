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
)

// modelPermissionAuthorizerSuite exists to form a set of contract tests for the
// [modelPermissionAuthorizer].
type modelPermissionAuthorizerSuite struct {
	permissionDelegator *MockPermissionDelegator
}

// TestModelAdminAuthorizerSuite runs all of the tests that are apart of the
// [modelPermissionAuthorizerSuite].
func TestModelAdminAuthorizerSuite(t *testing.T) {
	tc.Run(t, &modelPermissionAuthorizerSuite{})
}

func (s *modelPermissionAuthorizerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.permissionDelegator = NewMockPermissionDelegator(ctrl)

	c.Cleanup(func() {
		s.permissionDelegator = nil
	})
	return ctrl
}

// TestNoModelScope is testing that when using a [modelPermissionAuthorizer] and
// the incoming request is not scoped to a model that no further processing
// occurs and an error is returned.
//
// We don't care in this test what the error contains we just want to see that
// one is returned and non of the mocks have missing expected calls. That is to
// say the authoirzer should never proceed to take the request any further if
// a model cannot be established from the context.
//
// If this test develops unexpected mock call failures it is a good sign that
// these new calls need to be moved below basic constraint checks such as this
// one.
func (s *modelPermissionAuthorizerSuite) TestNoModelScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userTag := tc.Must1(c, names.ParseUserTag, "user-fred")
	authInfo := authentication.AuthInfo{
		Delegator: s.permissionDelegator,
		Tag:       userTag,
	}

	authorizer := modelPermissionAuthorizer{
		perm: corepermission.AdminAccess,
	}
	err := authorizer.Authorize(c.Context(), authInfo)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestNotAUser is testing that when using a [modelPermissionAuthorizer] for an
// entity that is not a user no further processing is done.
//
// This test checks for a not supported error from the func but it should not be
// considered part of the formal contract.
//
// We want to see an immediate error here and that the mocks have no expected
// call failures. If this test develops unexpected mock call failures it is
// a good sign that that these new calls need to be moved below basic constraint
// checks such as this one.
func (s *modelPermissionAuthorizerSuite) TestNotAUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	modelTag := names.NewModelTag(modelUUID.String())

	// a set of known tags that are not supported by the authorizer.
	notSupportEntityTags := []names.Tag{
		names.NewApplicationTag("foo"),
		names.NewControllerAgentTag("123"),
		names.NewMachineTag("0"),
		names.NewModelTag(modelUUID.String()),
		names.NewUnitTag("foo/0"),
	}

	for _, tag := range notSupportEntityTags {
		c.Run(tag.String(), func(t *testing.T) {
			authInfo := authentication.AuthInfo{
				Delegator: s.permissionDelegator,
				ModelTag:  modelTag,
				Tag:       tag,
			}

			authorizer := modelPermissionAuthorizer{
				perm: corepermission.AdminAccess,
			}

			err := authorizer.Authorize(t.Context(), authInfo)
			tc.Check(t, err, tc.ErrorIs, coreerrors.NotSupported)
		})
	}
}

// TestAdminAllowed tests that if the user has admin access for a model
// no error is produced by the authorizer. i.e the operation is allowed.
func (s *modelPermissionAuthorizerSuite) TestAdminAllowed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	modelTag := names.NewModelTag(modelUUID.String())
	userTag := tc.Must1(c, names.ParseUserTag, "user-fred")

	s.permissionDelegator.EXPECT().SubjectPermissions(
		gomock.Any(),
		"fred",
		corepermission.ID{
			ObjectType: corepermission.Model,
			Key:        modelUUID.String(),
		},
	).Return(corepermission.AdminAccess, nil)

	authInfo := authentication.AuthInfo{
		Delegator: s.permissionDelegator,
		ModelTag:  modelTag,
		Tag:       userTag,
	}

	authorizer := modelPermissionAuthorizer{
		perm: corepermission.AdminAccess,
	}

	err := authorizer.Authorize(c.Context(), authInfo)
	c.Check(err, tc.ErrorIsNil)
}

// TestWriteAccessNotAllowed tests that if the user only has write access for a
// model but admin access is required an error is returned to the caller. i.e
// the operation is not allowed.
func (s *modelPermissionAuthorizerSuite) TestWriteAccessNotAllowed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	modelTag := names.NewModelTag(modelUUID.String())
	userTag := tc.Must1(c, names.ParseUserTag, "user-fred")

	s.permissionDelegator.EXPECT().SubjectPermissions(
		gomock.Any(),
		"fred",
		corepermission.ID{
			ObjectType: corepermission.Model,
			Key:        modelUUID.String(),
		},
	).Return(corepermission.WriteAccess, nil)

	authInfo := authentication.AuthInfo{
		Delegator: s.permissionDelegator,
		ModelTag:  modelTag,
		Tag:       userTag,
	}

	authorizer := modelPermissionAuthorizer{
		perm: corepermission.AdminAccess,
	}

	err := authorizer.Authorize(c.Context(), authInfo)
	c.Check(err, tc.NotNil)
}

// TestReadAccessNotAllowed tests that if the user only has read access for a
// model and admin permissions are required an error is returned to the caller.
// i.e the operation is not allowed.
func (s *modelPermissionAuthorizerSuite) TestReadAccessNotAllowed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	modelTag := names.NewModelTag(modelUUID.String())
	userTag := tc.Must1(c, names.ParseUserTag, "user-fred")

	s.permissionDelegator.EXPECT().SubjectPermissions(
		gomock.Any(),
		"fred",
		corepermission.ID{
			ObjectType: corepermission.Model,
			Key:        modelUUID.String(),
		},
	).Return(corepermission.ReadAccess, nil)

	authInfo := authentication.AuthInfo{
		Delegator: s.permissionDelegator,
		ModelTag:  modelTag,
		Tag:       userTag,
	}

	authorizer := modelPermissionAuthorizer{
		perm: corepermission.AdminAccess,
	}

	err := authorizer.Authorize(c.Context(), authInfo)
	c.Check(err, tc.NotNil)
}

// TestNoAccessNotAllowed tests that if the user has no access for a model an
// error is returned to the caller. i.e the operation is not allowed.
func (s *modelPermissionAuthorizerSuite) TestNoAccessNotAllowed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	modelTag := names.NewModelTag(modelUUID.String())
	userTag := tc.Must1(c, names.ParseUserTag, "user-fred")

	s.permissionDelegator.EXPECT().SubjectPermissions(
		gomock.Any(),
		"fred",
		corepermission.ID{
			ObjectType: corepermission.Model,
			Key:        modelUUID.String(),
		},
	).Return(corepermission.NoAccess, nil)

	authInfo := authentication.AuthInfo{
		Delegator: s.permissionDelegator,
		ModelTag:  modelTag,
		Tag:       userTag,
	}

	authorizer := modelPermissionAuthorizer{
		perm: corepermission.AdminAccess,
	}

	err := authorizer.Authorize(c.Context(), authInfo)
	c.Check(err, tc.NotNil)
}
