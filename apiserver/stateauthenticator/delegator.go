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
// External users can inherit permissions from everyone@external, including
// first login before the external user's own DB row exists.
// Persisting external users is handled separately by admin.authenticate()
// after successful permission checks.
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
