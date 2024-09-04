// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
)

// ModelService provides information about currently deployed models.
type ModelService interface {
	// Model returns the model associated with the provided uuid.
	Model(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error)
}

// AccessService provides information about users and permissions.
type AccessService interface {
	// ReadUserAccessLevelForTarget returns the subject's (user) access level
	// for the given user on the given target.
	// If the access level of a user cannot be found then
	// [accesserrors.AccessNotFound] is returned.
	ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target corepermission.ID) (corepermission.Access, error)
	// ReadAllUserAccessForTarget return a slice of user access for all users
	// with access to the given target.
	// If not user access can be found on the target it will return
	// [accesserrors.PermissionNotFound].
	ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error)
	// CreatePermission gives the user access per the provided spec.
	// If the user provided does not exist or is marked removed,
	// [accesserrors.PermissionNotFound] is returned.
	// If the user provided exists but is marked disabled,
	// [accesserrors.UserAuthenticationDisabled] is returned.
	// If a permission for the user and target key already exists,
	// [accesserrors.PermissionAlreadyExists] is returned.
	CreatePermission(ctx context.Context, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error)
	// UpdatePermission updates the permission on the target for the given
	// subject (user). The api user must have Superuser access or Admin access
	// on the target. If a subject does not exist and the args specify, it is
	// created using the subject and api user. Adding the user would typically
	// only happen for updates to model access. Access can be granted or revoked.
	// Revoking Read access will delete the permission.
	UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error
	// DeletePermission removes the given user's access to the given target.
	// A NotValid error is returned if the subject (user) string is empty, or
	// the target is not valid.
	DeletePermission(ctx context.Context, subject user.Name, target corepermission.ID) error
	// GetUserByName will retrieve the user specified by name from the database.
	// If the user does not exist an error that satisfies
	// accesserrors.UserNotFound will be returned.
	GetUserByName(ctx context.Context, name user.Name) (user.User, error)
}
