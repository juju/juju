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
// then returns that user's access to the input subject.
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

// EnsureExternalUser implements authentication.ExternalUserEnsurer.
// For macaroon authentication, external users are only created if they have
// inherited permissions from everyone@external on the given targets.
func (p *PermissionDelegator) EnsureExternalUser(ctx context.Context, subject user.Name, targets []permission.ID) error {
	for _, target := range targets {
		if err := p.AccessService.EnsureExternalUserIfAuthorized(ctx, subject, target); err != nil {
			return err
		}
	}
	return nil
}

func (p *PermissionDelegator) PermissionError(_ names.Tag, _ permission.Access) error {
	return apiservererrors.ErrPerm
}
