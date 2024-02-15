// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/user"
	domainuser "github.com/juju/juju/domain/user"
	"github.com/juju/juju/domain/user/state"
	"github.com/juju/juju/internal/auth"
)

// AddUser is responsible for registering a new user in the system at bootstrap
// time. Sometimes it is required that we are need to register the existence of a
// user for ownership purposes but may not necessarily want to have local
// controller authentication for the user. The user created by this function is
// owned by itself.
//
// If the username passed to this function is invalid an error satisfying
// [github.com/juju/juju/domain/user/errors.UsernameNotValid] is returned.
func AddUser(name string) (user.UUID, func(context.Context, database.TxnRunner) error) {
	if err := domainuser.ValidateUsername(name); err != nil {
		return user.UUID(""), func(context.Context, database.TxnRunner) error {
			return fmt.Errorf("validating bootstrap add user %q: %w", name, err)
		}
	}

	uuid, err := user.NewUUID()
	if err != nil {
		return user.UUID(""), func(_ context.Context, _ database.TxnRunner) error {
			return fmt.Errorf("generating bootstrap user %q uuid: %w", name, err)
		}
	}

	return uuid, func(ctx context.Context, db database.TxnRunner) error {
		err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			return state.AddUser(ctx, tx, uuid, name, "", uuid)
		})

		if err != nil {
			return fmt.Errorf("adding bootstrap user %q: %w", name, err)
		}
		return nil
	}
}

// AddUserWithPassword is responsible for adding a new user in the system at
// bootstrap time with an associated authentication password. The user created
// by this function is owned by itself.
//
// If the username passed to this function is invalid an error satisfying
// [github.com/juju/juju/domain/user/errors.UsernameNotValid] is returned.
func AddUserWithPassword(name string, password auth.Password) (user.UUID, func(context.Context, database.TxnRunner) error) {
	defer password.Destroy()

	if err := domainuser.ValidateUsername(name); err != nil {
		return user.UUID(""), func(context.Context, database.TxnRunner) error {
			return fmt.Errorf("validating bootstrap add user %q with password: %w", name, err)
		}
	}

	uuid, err := user.NewUUID()
	if err != nil {
		return "", func(context.Context, database.TxnRunner) error {
			return fmt.Errorf(
				"generating uuid for bootstrap add user %q with password: %w",
				name, err,
			)
		}
	}

	salt, err := auth.NewSalt()
	if err != nil {
		return "", func(context.Context, database.TxnRunner) error {
			return fmt.Errorf(
				"generating salt for bootstrap add user %q with password: %w",
				name, err,
			)
		}
	}

	pwHash, err := auth.HashPassword(password, salt)
	if err != nil {
		return "", func(context.Context, database.TxnRunner) error {
			return fmt.Errorf(
				"generating password hash for bootstrap add user %q with password: %w",
				name, err,
			)
		}
	}

	return uuid, func(ctx context.Context, db database.TxnRunner) error {
		return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err = state.AddUserWithPassword(
				ctx,
				tx,
				uuid,
				name,
				name,
				uuid,
				pwHash,
				salt,
			); err != nil {
				return fmt.Errorf("adding bootstrap user %q with password: %w",
					name, err,
				)
			}
			return nil
		}))
	}
}
