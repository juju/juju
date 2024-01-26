// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/cloud/state"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	"github.com/juju/juju/internal/uuid"
)

// InsertCloud inserts the initial cloud during bootstrap.
func InsertCloud(cloud cloud.Cloud) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			cloudUUID, err := uuid.NewUUID()
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

// SetCloudDefaults is responsible for setting a previously inserted cloud's
// default config values that will be used as part of the default values
// supplied to a models config. If no cloud exists for the specified name an
// error satisfying [github.com/juju/juju/domain/cloud/errors.NotFound] will be
// returned.
func SetCloudDefaults(
	cloudName string,
	defaults map[string]any,
) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		strDefaults, err := modelconfigservice.CoerceConfigForStorage(defaults)
		if err != nil {
			return fmt.Errorf("coercing cloud %q default values for storage: %w", cloudName, err)
		}

		err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			return state.SetCloudDefaults(ctx, tx, cloudName, strDefaults)
		})

		if err != nil {
			return fmt.Errorf("setting cloud %q bootstrap defaults: %w", cloudName, err)
		}
		return nil
	}
}
