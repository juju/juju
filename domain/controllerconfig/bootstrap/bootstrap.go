// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
	jujudatabase "github.com/juju/juju/database"
)

// InsertInitialControllerConfig inserts the initial controller configuration
// into the database.
func InsertInitialControllerConfig(cfg controller.Config) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		values, err := controller.EncodeToString(cfg)
		if err != nil {
			return errors.Trace(err)
		}

		fields, _, err := controller.ConfigSchema.ValidationSchema()
		if err != nil {
			return errors.Trace(err)
		}

		for k := range values {
			if field, ok := fields[k]; ok {
				_, err := field.Coerce(values[k], []string{k})
				if err != nil {
					return errors.Annotatef(err, "coerce controller config key %q", k)
				}
			}
		}

		query := "INSERT INTO controller_config (key, value) VALUES (?, ?)"

		return errors.Trace(db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			for k, v := range values {
				if _, err := tx.ExecContext(ctx, query, k, v); err != nil {
					if jujudatabase.IsErrConstraintPrimaryKey(errors.Cause(err)) {
						return errors.AlreadyExistsf("controller configuration key %q", k)
					}
					return errors.Annotatef(err, "inserting controller configuration %q, %v", k, v)
				}
			}
			return nil
		}))
	}
}
