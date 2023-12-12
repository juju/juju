// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/user/state"
)

// InsertAdminUser inserts the admin user into database.
func InsertAdminUser() func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err := state.CreateAdminUser(ctx, tx); err != nil {
				return errors.Annotate(err, "creating bootstrap admin user")
			}
			return nil
		}))
	}
}
