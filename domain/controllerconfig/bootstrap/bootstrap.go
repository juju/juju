// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
)

func InsertInitialControllerConfig(cfg controller.Config) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		fields, _, err := controller.ConfigSchema.ValidationSchema()
		if err != nil {
			return errors.Trace(err)
		}

		return errors.Trace(db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			for k, v := range cfg {
				if field, ok := fields[k]; ok {
					if v, err = field.Coerce(v, []string{k}); err != nil {
						return errors.Trace(err)
					}
				}

				q := "INSERT INTO controller_config (key, value) VALUES (?, ?)"
				if _, err := tx.ExecContext(ctx, q, k, v); err != nil {
					return errors.Annotatef(err, "inserting controller configuration %q, %v", k, v)
				}
			}
			return nil
		}))
	}
}
