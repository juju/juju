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
	accesserrors "github.com/juju/juju/domain/access/errors"
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

	access, err := p.AccessService.ReadUserAccessLevelForTargetAddingMissingUser(ctx, userID, permissionID)
	if errors.Is(err, accesserrors.AccessNotFound) {
		return permission.NoAccess, accesserrors.PermissionNotFound
	} else if err != nil {
		return permission.NoAccess, errors.Trace(err)
	}
	return access, nil
}

func (p *PermissionDelegator) PermissionError(_ names.Tag, _ permission.Access) error {
	return apiservererrors.ErrPerm
}
