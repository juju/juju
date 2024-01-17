// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/life"
)

// InsertMachine inserts a machine during bootstrap.
// TODO - this just creates a minimal row for now.
func InsertMachine(machineId string) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {

		createMachine := `
INSERT INTO machine (uuid, net_node_uuid, machine_id, life_id)
VALUES ($M.machine_uuid, $M.net_node_uuid, $M.machine_id, $M.life_id)
`
		createMachineStmt, err := sqlair.Prepare(createMachine, sqlair.M{})
		if err != nil {
			return errors.Trace(err)
		}

		createNode := `INSERT INTO net_node (uuid) VALUES ($M.net_node_uuid)`
		createNodeStmt, err := sqlair.Prepare(createNode, sqlair.M{})
		if err != nil {
			return errors.Trace(err)
		}

		nodeUUID, err := utils.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		machineUUID, err := utils.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}

		return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			createParams := sqlair.M{
				"machine_uuid":  machineUUID.String(),
				"net_node_uuid": nodeUUID.String(),
				"machine_id":    machineId,
				"life_id":       life.Alive,
			}
			if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
				return errors.Annotatef(err, "creating net node row for bootstrap machine %q", machineId)
			}
			if err := tx.Query(ctx, createMachineStmt, createParams).Run(); err != nil {
				return errors.Annotatef(err, "creating machine row for bootstrap machine %q", machineId)
			}
			return nil
		}))
	}
}
