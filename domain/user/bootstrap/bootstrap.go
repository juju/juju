// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/user/state"
	"github.com/juju/juju/internal/auth"
)

// GenerateAdminUser inserts the admin user into database.
// It is used to bootstrap the database.
//
// Admin user is created with the following characteristics:
// 1. This is first user created in the system.
// 2. This user is used to owner of the first model created in the system.
// 3. This user is created with no password authorization by default.
func GenerateAdminUser(adminName string, password string) func(context.Context, database.TxnRunner) error {
	adminUserUUID, err := user.NewUUID()
	if err != nil {
		return func(context.Context, database.TxnRunner) error {
			return errors.Annotate(err, "generating uuid for bootstrap admin user")
		}
	}

	adminPassword := auth.NewPassword(password)

	salt, err := auth.NewSalt()
	if err != nil {
		return func(context.Context, database.TxnRunner) error {
			return errors.Annotate(err, "generating salt for bootstrap admin user")
		}
	}

	pwHash, err := auth.HashPassword(adminPassword, salt)
	if err != nil {
		return func(context.Context, database.TxnRunner) error {
			return errors.Annotate(err, "generating password hash for bootstrap admin user")
		}
	}

	return func(ctx context.Context, db database.TxnRunner) error {
		return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err = state.BootstrapAddUserWithPassword(
				ctx,
				tx,
				adminUserUUID,
				user.User{
					Name:        adminName,
					DisplayName: adminName,
				},
				adminUserUUID,
				pwHash,
				salt,
			); err != nil {
				return errors.Annotate(err, "creating bootstrap admin user")
			}
			return nil
		}))
	}
}
