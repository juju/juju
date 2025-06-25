// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

// GetMachinesForExport returns all machines in the model for export.
func (st *State) GetMachinesForExport(ctx context.Context) ([]machine.ExportMachine, error) {
	db, err := st.DB()
	if err != nil {
		return nil, err
	}

	query := `
SELECT m.name AS &exportMachine.name,
	   m.uuid AS &exportMachine.uuid,
	   m.life_id AS &exportMachine.life_id,  
	   m.nonce AS &exportMachine.nonce
FROM machine AS m
JOIN machine_cloud_instance mci ON m.uuid = mci.machine_uuid
WHERE mci.instance_id IS NOT NULL AND mci.instance_id != '';`

	stmt, err := st.Prepare(query, exportMachine{})
	if err != nil {
		return nil, errors.Errorf("preparing query for machine export: %w", err)
	}

	var machines []exportMachine
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).GetAll(&machines); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("querying machines for export: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]machine.ExportMachine, len(machines))
	for i, m := range machines {
		result[i] = machine.ExportMachine{
			Name:  coremachine.Name(m.Name),
			UUID:  coremachine.UUID(m.UUID),
			Nonce: m.Nonce,
		}
	}
	return result, nil
}

// InsertMigratingMachine inserts a machine which is taken from the description
// model during migration into the machine table.
//
// The following errors may be returned:
// - [machineerrors.MachineAlreadyExists] if a machine with the same name
// already exists.
func (st *State) InsertMigratingMachine(ctx context.Context, machineName string, args machine.CreateMachineArgs) error {
	db, err := st.DB()
	if err != nil {
		return err
	}

	insertMachineArgs := insertMachineAndNetNodeArgs{
		machineName: machineName,
		machineUUID: args.MachineUUID.String(),
		platform:    args.Platform,
		nonce:       args.Nonce,
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		existingMachineUUID, err := getMachineUUIDFromName(ctx, tx, st, coremachine.Name(machineName))
		if err != nil {
			return err
		}
		if existingMachineUUID != "" {
			return errors.Errorf("machine %q already exists", machineName).Add(machineerrors.MachineAlreadyExists)
		}
		_, err = insertMachineAndNetNode(ctx, tx, st, st.clock, insertMachineArgs)
		return err
	})
	return errors.Capture(err)
}
