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
	AccessService AccessService
}

// SubjectPermissions ensures that the input entity is a user,
// then returns that user's access to the input subject.
func (p *PermissionDelegator) SubjectPermissions(
	ctx context.Context, entity authentication.Entity, target names.Tag,
) (permission.Access, error) {
	userTag, ok := entity.Tag().(names.UserTag)
	if !ok {
		return permission.NoAccess, errors.Errorf("%s is not a user", names.ReadableString(entity.Tag()))
	}
	userID := userTag.Id()

	permissionID, err := permission.ParseTagForID(target)
	if err != nil {
		return permission.NoAccess, errors.Trace(err)
	}

	// TODO(aflynn) why is this not ReadUserAccessLevelForTarget? We just throw
	// away the access.
	access, err := p.AccessService.ReadUserAccessForTarget(ctx, userID, permissionID)
	if err != nil {
		return permission.NoAccess, errors.Trace(err)
	}
	return access.Access, nil
}

func (p *PermissionDelegator) PermissionError(_ names.Tag, _ permission.Access) error {
	return apiservererrors.ErrPerm
}
