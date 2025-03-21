// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/internal/auth"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// AddUserWithPassword is responsible for adding a new user in the system at
// bootstrap time with an associated authentication password. The user created
// by this function is owned by itself.
//
// If the username passed to this function is invalid an error satisfying
// [github.com/juju/juju/domain/access/errors.UsernameNotValid] is returned.
func AddUserWithPassword(name user.Name, password auth.Password, access permission.AccessSpec) (user.UUID, internaldatabase.BootstrapOpt) {
	defer password.Destroy()

	if name.IsZero() {
		return "", bootstrapErr(errors.Errorf("%q: %w", name, usererrors.UserNameNotValid))
	}

	uuid, err := user.NewUUID()
	if err != nil {
		return "", bootstrapErr(errors.Errorf(
			"generating uuid for bootstrap add user %q with password: %w",
			name, err))
	}

	salt, err := auth.NewSalt()
	if err != nil {
		return "", bootstrapErr(errors.Errorf(
			"generating salt for bootstrap add user %q with password: %w",
			name, err))
	}

	pwHash, err := auth.HashPassword(password, salt)
	if err != nil {
		return "", bootstrapErr(errors.Errorf(
			"generating password hash for bootstrap add user %q with password: %w",
			name, err))
	}

	return uuid, func(ctx context.Context, controller, model database.TxnRunner) error {
		return errors.Capture(controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err = state.AddUserWithPassword(
				ctx, tx,
				uuid,
				name, name.Name(),
				uuid,
				access,
				pwHash, salt,
			); err != nil {
				return errors.Errorf("adding bootstrap user %q with password: %w",
					name, err)

			}
			return nil
		}))

	}
}

func bootstrapErr(err error) func(context.Context, database.TxnRunner, database.TxnRunner) error {
	return func(context.Context, database.TxnRunner, database.TxnRunner) error {
		return err
	}
}
