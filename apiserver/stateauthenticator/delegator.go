// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/permission"
)

// PermissionDelegator implements authentication.PermissionDelegator
type PermissionDelegator struct {
	AccessService UserService
}

// SubjectPermissions ensures that the input entity is a user,
// then returns that user's access to the input subject.
func (p *PermissionDelegator) SubjectPermissions(
	entity authentication.Entity, target names.Tag,
) (permission.Access, error) {
	userTag, ok := entity.Tag().(names.UserTag)
	if !ok {
		return permission.NoAccess, errors.Errorf("%s is not a user", names.ReadableString(entity.Tag()))
	}
	userID := userTag.Id()

	var permissionID permission.ID
	// TODO (manadart 2024-05-27): Follow up with this.
	// checkUserPermissions in admin.go checks for access to a controller tag,
	// which is constituted by a controller ID, but we appear to be granting
	// access to an entity called "controller".
	if _, ok := target.(names.ControllerTag); ok {
		permissionID = permission.ID{
			ObjectType: permission.Controller,
			Key:        "controller",
		}
	} else {
		var err error
		permissionID, err = permission.ParseTagForID(target)
		if err != nil {
			return permission.NoAccess, errors.Trace(err)
		}
	}

	access, err := p.AccessService.ReadUserAccessForTarget(context.TODO(), userID, permissionID)
	if err != nil {
		return permission.NoAccess, errors.Trace(err)
	}
	return access.Access, nil
}

func (p *PermissionDelegator) PermissionError(_ names.Tag, _ permission.Access) error {
	return apiservererrors.ErrPerm
}
