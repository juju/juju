// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/life"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// InsertMachine inserts a machine during bootstrap.
// TODO - this just creates a minimal row for now.
func InsertMachine(machineId string) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {

		createMachine := `
INSERT INTO machine (uuid, net_node_uuid, name, life_id)
VALUES ($M.machine_uuid, $M.net_node_uuid, $M.name, $M.life_id)
`
		createMachineStmt, err := sqlair.Prepare(createMachine, sqlair.M{})
		if err != nil {
			return errors.Capture(err)
		}

		createNode := `INSERT INTO net_node (uuid) VALUES ($M.net_node_uuid)`
		createNodeStmt, err := sqlair.Prepare(createNode, sqlair.M{})
		if err != nil {
			return errors.Capture(err)
		}

		nodeUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		machineUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}

		return errors.Capture(model.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			createParams := sqlair.M{
				"machine_uuid":  machineUUID.String(),
				"net_node_uuid": nodeUUID.String(),
				"name":          machineId,
				"life_id":       life.Alive,
			}
			if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
				return errors.Errorf("creating net node row for bootstrap machine %q: %w", machineId, err)
			}
			if err := tx.Query(ctx, createMachineStmt, createParams).Run(); err != nil {
				return errors.Errorf("creating machine row for bootstrap machine %q: %w", machineId, err)
			}
			return nil
		}))

	}
}
