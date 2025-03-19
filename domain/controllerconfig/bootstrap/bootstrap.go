// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// InsertInitialControllerConfig inserts the initial controller configuration
// into the database.
func InsertInitialControllerConfig(cfg jujucontroller.Config, controllerModelUUID coremodel.UUID) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		values, err := jujucontroller.EncodeToString(cfg)
		if err != nil {
			return errors.Capture(err)
		}

		if err = controllerModelUUID.Validate(); err != nil {
			return errors.Capture(err)
		}

		fields, _, err := jujucontroller.ConfigSchema.ValidationSchema()
		if err != nil {
			return errors.Capture(err)
		}

		for k := range values {
			if field, ok := fields[k]; ok {
				_, err := field.Coerce(values[k], []string{k})
				if err != nil {
					return errors.Errorf("unable to coerce controller config key %q: %w", k, err)
				}
			}
		}

		insertStmt, err := sqlair.Prepare(`INSERT INTO controller_config (key, value) VALUES ($dbKeyValue.*)`, dbKeyValue{})
		if err != nil {
			return errors.Capture(err)
		}

		controllerData := dbController{
			UUID:      values[jujucontroller.ControllerUUIDKey],
			ModelUUID: controllerModelUUID.String(),
		}
		controllerStmt, err := sqlair.Prepare(`INSERT INTO controller (uuid, model_uuid) VALUES ($dbController.*)`, controllerData)
		if err != nil {
			return errors.Capture(err)
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

		return errors.Capture(controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			// Insert the controller data.
			if err := tx.Query(ctx, controllerStmt, controllerData).Run(); err != nil {
				return errors.Capture(err)
			}

			// Update the attributes.
			if len(updateKeyValues) > 0 {
				if err := tx.Query(ctx, insertStmt, updateKeyValues).Run(); err != nil {
					return errors.Capture(err)
				}
			} else {
				return errors.Errorf("no controller config values to insert at bootstrap")
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
	// UUID is the unique identifier of the controller.
	UUID string `db:"uuid"`
	// ModelUUID is the uuid of the model this controller is in.
	ModelUUID string `db:"model_uuid"`
}
