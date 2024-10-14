// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
)

// CreateTestController creates a test controller with the given name.
func CreateTestController(c *gc.C, txnRunner database.TxnRunnerFactory, controllerUUID controller.UUID, modelUUID model.UUID) {
	runner, err := txnRunner()
	c.Assert(err, jc.ErrorIsNil)

	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`
INSERT INTO controller (uuid, model_uuid)
VALUES (?, ?)
		`, controllerUUID.String(), modelUUID.String())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}
