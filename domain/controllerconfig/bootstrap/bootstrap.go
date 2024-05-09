// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
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

		insertStmt, err := sqlair.Prepare(`INSERT INTO controller_config (key, value) VALUES ($dbKeyValue.*)`, dbKeyValue{})
		if err != nil {
			return errors.Trace(err)
		}

		controllerStmt, err := sqlair.Prepare(`INSERT INTO controller (uuid) VALUES ($dbController.uuid)`, dbController{})
		if err != nil {
			return errors.Trace(err)
		}
		data := dbController{
			UUID: values[jujucontroller.ControllerUUIDKey],
		}

		updateKeyValues := make([]dbKeyValue, 0)
		for k, v := range values {
			if k == jujucontroller.ControllerUUIDKey {
				continue
			}
			updateKeyValues = append(updateKeyValues, dbKeyValue{
				Key:   k,
				Value: v,
			})
		}

		return errors.Trace(controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			// Insert the controller data.
			if err := tx.Query(ctx, controllerStmt, data).Run(); err != nil {
				return errors.Trace(err)
			}

			// Update the attributes.
			if err := tx.Query(ctx, insertStmt, updateKeyValues).Run(); err != nil {
				return errors.Trace(err)
			}

			return nil
		}))
	}
}

type dbKeyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

type dbController struct {
	UUID string `db:"uuid"`
}
