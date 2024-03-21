// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
	internaldatabase "github.com/juju/juju/internal/database"
)

// InsertInitialControllerConfig inserts the initial controller configuration
// into the database.
func InsertInitialControllerConfig(cfg jujucontroller.Config) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		values, err := jujucontroller.EncodeToString(cfg)
		if err != nil {
			return errors.Trace(err)
		}

		fields, _, err := jujucontroller.ConfigSchema.ValidationSchema()
		if err != nil {
			return errors.Trace(err)
		}

		for k := range values {
			if field, ok := fields[k]; ok {
				_, err := field.Coerce(values[k], []string{k})
				if err != nil {
					return errors.Annotatef(err, "unable to coerce controller config key %q", k)
				}
			}
		}

		query := "INSERT INTO controller_config (key, value) VALUES (?, ?)"

		return errors.Trace(controller.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			for k, v := range values {
				if _, err := tx.ExecContext(ctx, query, k, v); err != nil {
					if internaldatabase.IsErrConstraintPrimaryKey(errors.Cause(err)) {
						return errors.AlreadyExistsf("controller configuration key %q", k)
					}
					return errors.Annotatef(err, "inserting controller configuration %q, %v", k, v)
				}
			}
			return nil
		}))
	}
}
