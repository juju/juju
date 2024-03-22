// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	domainuser "github.com/juju/juju/domain/user"
	"github.com/juju/juju/domain/user/state"
	"github.com/juju/juju/internal/auth"
	internaldatabase "github.com/juju/juju/internal/database"
)

// AddUser is responsible for registering a new user in the system at bootstrap
// time. Sometimes it is required that we are need to register the existence of a
// user for ownership purposes but may not necessarily want to have local
// controller authentication for the user. The user created by this function is
// owned by itself.
//
// If the username passed to this function is invalid an error satisfying
// [github.com/juju/juju/domain/user/errors.UsernameNotValid] is returned.
func AddUser(name string, access permission.AccessSpec) (user.UUID, internaldatabase.BootstrapOpt) {
	if err := domainuser.ValidateUserName(name); err != nil {
		return user.UUID(""), bootstrapErr(
			fmt.Errorf("validating bootstrap add user %q: %w", name, err))
	}

	uuid, err := user.NewUUID()
	if err != nil {
		return user.UUID(""), bootstrapErr(
			fmt.Errorf("generating bootstrap user %q uuid: %w", name, err))
	}

	state := state.NewSharedState(internaldatabase.DefaultStatementBase{})

	return uuid, func(ctx context.Context, controller, model database.TxnRunner) error {
		err := controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			return state.AddUser(ctx, tx, uuid, name, "", uuid, access)
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
func AddUserWithPassword(name string, password auth.Password, access permission.AccessSpec) (user.UUID, internaldatabase.BootstrapOpt) {
	defer password.Destroy()

	if err := domainuser.ValidateUserName(name); err != nil {
		return user.UUID(""), bootstrapErr(
			fmt.Errorf("validating bootstrap add user %q with password: %w",
				name, err))
	}

	uuid, err := user.NewUUID()
	if err != nil {
		return "", bootstrapErr(fmt.Errorf(
			"generating uuid for bootstrap add user %q with password: %w",
			name, err))
	}

	salt, err := auth.NewSalt()
	if err != nil {
		return "", bootstrapErr(fmt.Errorf(
			"generating salt for bootstrap add user %q with password: %w",
			name, err))
	}

	pwHash, err := auth.HashPassword(password, salt)
	if err != nil {
		return "", bootstrapErr(fmt.Errorf(
			"generating password hash for bootstrap add user %q with password: %w",
			name, err))
	}

	state := state.NewSharedState(internaldatabase.DefaultStatementBase{})

	return uuid, func(ctx context.Context, controller, model database.TxnRunner) error {
		return errors.Trace(controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err = state.AddUserWithPassword(
				ctx, tx,
				uuid,
				name, name,
				uuid,
				access,
				pwHash, salt,
			); err != nil {
				return fmt.Errorf("adding bootstrap user %q with password: %w",
					name, err,
				)
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
