// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/errors"
)

// tagKindAuthorizer checks that the authenticated entity is one of the allowed
// tag kinds.
//
// Deprecated: This authorizer should not be used for any new code. Make a new
// authorizer that is targeted to the specific context of the authorization.
type tagKindAuthorizer []string

// Authorize is part of the httpcontext.Authorizer interface.
func (a tagKindAuthorizer) Authorize(_ context.Context, authInfo authentication.AuthInfo) error {
	tagKind := authInfo.Tag.Kind()
	for _, kind := range a {
		if tagKind == kind {
			return nil
		}
	}
	return errors.Errorf("tag kind %q is not supported", tagKind).Add(
		coreerrors.NotValid,
	)
}

type controllerAdminAuthorizer struct {
	controllerTag names.Tag
}

// Authorize is part of the httpcontext.Authorizer interface.
func (a controllerAdminAuthorizer) Authorize(ctx context.Context, authInfo authentication.AuthInfo) error {
	userTag, ok := authInfo.Tag.(names.UserTag)
	if !ok {
		return errors.New("authorization is not for a user").Add(
			coreerrors.NotSupported,
		)
	}

	has, err := common.HasPermission(ctx,
		func(ctx context.Context, userName user.Name, subject permission.ID) (permission.Access, error) {
			return authInfo.Delegator.SubjectPermissions(ctx, userName.String(), subject)
		},
		userTag, permission.SuperuserAccess, a.controllerTag,
	)
	if err != nil {
		return errors.Capture(err)
	}
	if !has {
		return errors.Errorf("%s is not a controller admin", names.ReadableString(authInfo.Tag))
	}
	return nil
}

// modelPermissionAuthorizer checks that the authenticated user
// has the given permission on a model.
type modelPermissionAuthorizer struct {
	perm permission.Access
}

// Authorize is part of the httpcontext.Authorizer interface.
func (a modelPermissionAuthorizer) Authorize(ctx context.Context, authInfo authentication.AuthInfo) error {
	userTag, ok := authInfo.Tag.(names.UserTag)
	if !ok {
		return errors.New("authorization is not for a user").Add(
			coreerrors.NotSupported,
		)
	}
	if !names.IsValidModel(authInfo.ModelTag.Id()) {
		return errors.New("authorization is for invalid model").Add(
			coreerrors.NotValid,
		)
	}
	has, err := common.HasPermission(ctx,
		func(ctx context.Context, user user.Name, subject permission.ID) (permission.Access, error) {
			return authInfo.Delegator.SubjectPermissions(ctx, user.String(), subject)
		},
		userTag, a.perm, authInfo.ModelTag,
	)
	if err != nil {
		return errors.Capture(err)
	}
	if !has {
		return errors.Errorf("%s does not have %q permission", names.ReadableString(authInfo.Tag), a.perm)
	}
	return nil
}
