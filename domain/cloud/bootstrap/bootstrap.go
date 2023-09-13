// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/cloud/state"
)

// InsertCloud inserts the initial cloud during bootstrap.
func InsertCloud(cloud cloud.Cloud) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		return errors.Trace(db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			cloudUUID, err := utils.NewUUID()
			if err != nil {
				return errors.Trace(err)
			}
			if err := state.CreateCloud(ctx, tx, cloudUUID.String(), cloud); err != nil {
				return errors.Annotate(err, "creating bootstrap cloud")
			}
			return nil
		}))
	}
}
