// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/httpcontext"
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

// controllerAdminAuthorizer checks that the user being authorized has
// [permission.SuperuserAccess] on the controller.
//
// controllerAdminAuthorizer implements the
// [github.com/juju/juju/apiserver/authentication.Authorizer] interface.
type controllerAdminAuthorizer struct {
	controllerTag names.Tag
}

// Authorize checks that the authorization request is for a user that has
// [permission.SuperuserAccess] on the controller. No other permissions are
// considered valid for this authorizer.
//
// Authorize implements the
// [github.com/juju/juju/apiserver/authentication.Authorizer] interface.
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

// modelPermissionAuthorizer checks that the authenticated user has the given
// permission on a model.
//
// modelPermissionAuthorizer implements the
// [github.com/juju/juju/apiserver/authentication.Authorizer] interface.
type modelPermissionAuthorizer struct {
	perm permission.Access
}

// Authorize checks that the authorization request is for a valid model uuid and
// a user. If both of these facts are true then it checks that the user has the
// permission defined in [modelPermissionAuthorizer.perm] on the model.
//
// This authorizer will only check for exact permissions on the model. If the
// user has write access on the model and this authorizer is set to check for
// read permissions it will still fail the authorization. To support permission
// heirarchy use multiple [modelPermissionAuthorizer]s.
//
// Authorize implements the
// [github.com/juju/juju/apiserver/authentication.Authorizer] interface.
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

// ModelAuthorizationInfo provides information to authorizer's about the model
// that is being used in the authorization request.
type ModelAuthorizationInfo interface {
	// IsAuthorizationForControllerModel returns true of false based on if the
	// current authorization request is for the controller's model.
	IsAuthorizationForControllerModel(context.Context) bool
}

// ModelAuthorizationInfoFunc provides a func type implementation of
// [ModelAuthorizationInfo].
type ModelAuthorizationInfoFunc func(context.Context) bool

// IsAuthorizationForControllerModel proxies the call through to the func of
// [ModelAuthorizationInfoFunc] returning the result.
//
// Implements [ModelAuthorizationInfo] interface.
func (m ModelAuthorizationInfoFunc) IsAuthorizationForControllerModel(
	c context.Context,
) bool {
	return m(c)
}

// modelAuthorizationInfoForRequest returns a [ModelAuthorizationInfo] capable
// of answering authorization info off of the request context.
func modelAuthorizationInfoForRequest() ModelAuthorizationInfoFunc {
	return httpcontext.RequestIsForControllerModel
}

// controllerModelPermissionAuthorizer checks if the authorization request is
// for the controller model and if so confirms the user has
// [permission.SuperuserAccess] on the controller by using the
// [controllerModelPermissionAuthorizer.controllerAdminAuthorizer]. If the
// authorization request is not for the controller model then the request is
// delegated to
// [controllerModelPermissionAuthorizer.fallThroughAuthorizer].
type controllerModelPermissionAuthorizer struct {
	controllerAdminAuthorizer
	fallThroughAuthroizer authentication.Authorizer
	ModelAuthorizationInfo
}

// Authorize checks if the authorization request is being made to the controller
// model and that the user being authorized has [permission.SuperuserAccess] on
// the controller. If the authorization request is not for the controller model
// then the request is passed on to the fallThroughAuthorizer.
//
// Authorize implements the
// [github.com/juju/juju/apiserver/authentication.Authorizer] interface.
func (a controllerModelPermissionAuthorizer) Authorize(
	ctx context.Context, authInfo authentication.AuthInfo,
) error {
	isControllerModel := a.ModelAuthorizationInfo.IsAuthorizationForControllerModel(ctx)
	if !isControllerModel {
		// We can defer through to the fallThroughAuthorizer.
		return a.fallThroughAuthroizer.Authorize(ctx, authInfo)
	}

	// Authorization is for the controller model, must pass the
	// [controllerAdminAuthorizer] checks.
	return a.controllerAdminAuthorizer.Authorize(ctx, authInfo)
}
