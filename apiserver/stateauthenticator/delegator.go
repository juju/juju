// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
)

// PermissionDelegator implements authentication.PermissionDelegator
type PermissionDelegator struct {
	AccessService AccessService
}

// SubjectPermissions ensures that the input entity is a user,
// then returns that user's access to the input target.
//
// This method is a pure permission read with no side effects.
// External user creation was previously performed here via
// EnsureExternalUserIfAuthorized (which checked everyone@external
// permissions before inserting). That call has been removed because:
//   - User creation is now an explicit step in admin.authenticate(),
//     which calls EnsureExternalUser unconditionally for all externally
//     authenticated entities.
//   - The everyone@external permission check is already performed by
//     ReadUserAccessLevelForTarget, which returns the higher of the
//     user's own access and everyone@external's access. If neither has
//     access, AccessNotFound is returned and the caller denies login.
func (p *PermissionDelegator) SubjectPermissions(
	ctx context.Context, userName string, target permission.ID,
) (permission.Access, error) {

	name, err := user.NewName(userName)
	if err != nil {
		return permission.NoAccess, errors.Trace(err)
	}

	access, err := p.AccessService.ReadUserAccessLevelForTarget(ctx, name, target)
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
