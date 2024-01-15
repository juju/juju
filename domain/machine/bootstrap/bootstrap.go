// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/machine/state"
)

// InsertBootstrapMachine inserts the initial machine during bootstrap.
func InsertBootstrapMachine(machineId string) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err := state.CreateMachine(ctx, tx, machineId); err != nil {
				return errors.Annotate(err, "creating bootstrap machine")
			}
			return nil
		}))
	}
}
