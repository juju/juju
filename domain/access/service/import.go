// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/internal"
	"github.com/juju/juju/internal/errors"
)

// ImportExternalUsers creates external users from a migrated model on the
// target controller. Each user is created with [corepermission.EveryoneUserName]
// as creator (consistent with how external users are created on first
// authentication) and with their original creation date. If a user already
// exists on the target controller, their creation is silently skipped.
// Permission granting and last-login tracking are handled separately by the
// migration operation.
// The following error types are possible from this function:
//   - [accesserrors.UserNotFound]: If [corepermission.EveryoneUserName] does
//     not exist on the controller.
func (s *UserService) ImportExternalUsers(
	ctx context.Context,
	users []internal.ExternalUserImport,
) error {
	if len(users) == 0 {
		return nil
	}

	everyoneUUID, err := s.GetUserUUIDByName(ctx, corepermission.EveryoneUserName)
	if err != nil {
		return errors.Errorf(
			"getting %q UUID: %w", corepermission.EveryoneUserName, err,
		)
	}

	for _, u := range users {
		userUUID, err := user.NewUUID()
		if err != nil {
			return errors.Errorf("generating UUID for user %q: %w", u.Name, err)
		}

		err = s.st.AddUserWithCreatedAt(
			ctx, userUUID, u.Name, u.DisplayName, everyoneUUID, u.DateCreated,
		)
		if err != nil && !errors.Is(err, accesserrors.UserAlreadyExists) {
			return errors.Errorf("adding external user %q: %w", u.Name, err)
		}
	}

	return nil
}
