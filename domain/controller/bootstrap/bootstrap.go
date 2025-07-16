// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// InsertInitialController inserts the initial controller information into the
// database.
func InsertInitialController(cert, privateKey, caPrivateKey, systemIdentity string) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		controllerData := dbController{
			Cert:           cert,
			PrivateKey:     privateKey,
			CAPrivateKey:   caPrivateKey,
			SystemIdentity: systemIdentity,
		}
		controllerStmt, err := sqlair.Prepare(`
UPDATE controller 
SET cert=$dbController.cert,
    private_key=$dbController.private_key,
	ca_private_key=$dbController.ca_private_key,
	system_identity=$dbController.system_identity;
`, controllerData)
		if err != nil {
			return errors.Capture(err)
		}

		return errors.Capture(controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			// Insert the controller data.
			if err := tx.Query(ctx, controllerStmt, controllerData).Run(); err != nil {
				return errors.Capture(err)
			}

			return nil
		}))
	}
}

type dbController struct {
	Cert           string `db:"cert"`
	PrivateKey     string `db:"private_key"`
	CAPrivateKey   string `db:"ca_private_key"`
	SystemIdentity string `db:"system_identity"`
}
