// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"

	blockdevice "github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

// MachineExists returns true if a machine exists with the input UUID.
func (st *State) MachineExists(ctx context.Context, mUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	machineUUID := entityUUID{UUID: mUUID}
	existsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   machine
WHERE  uuid = $entityUUID.uuid`, machineUUID)
	if err != nil {
		return false, errors.Errorf("preparing machine exists query: %w", err)
	}

	var machineExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, machineUUID).Get(&machineUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running machine exists query: %w", err)
		}

		machineExists = true
		return nil
	})

	return machineExists, errors.Capture(err)
}

// EnsureMachineNotAliveCascade ensures that there is no machine identified by
// the input machine UUID, that is still alive.
func (st *State) EnsureMachineNotAliveCascade(ctx context.Context, mUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	machineUUID := entityUUID{UUID: mUUID}
	updateUnitStmt, err := st.Prepare(`
UPDATE machine
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, machineUUID)
	if err != nil {
		return errors.Errorf("preparing machine life update: %w", err)
	}

	if err := errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, updateUnitStmt, machineUUID).Run(); err != nil {
			return errors.Errorf("advancing machine life: %w", err)
		}
		return nil
	})); err != nil {
		return err
	}

	return nil
}

// GetMachineLife returns the life of the machine with the input UUID.
func (st *State) GetMachineLife(ctx context.Context, mUUID string) (life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return -1, errors.Capture(err)
	}

	var life life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		life, err = st.getMachineLife(ctx, tx, mUUID)

		return errors.Capture(err)
	})

	return life, errors.Capture(err)
}

// DeleteMachine deletes the specified machine and any dependent child records.
func (st *State) DeleteMachine(ctx context.Context, mUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for machine uuid.
	machineUUIDParam := entityUUID{UUID: mUUID}
	queryMachine := `
SELECT &entityUUID.*
FROM machine
WHERE uuid = $entityUUID.uuid;
`
	queryMachineStmt, err := st.Prepare(queryMachine, machineUUIDParam)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for deleting machine row.
	deleteMachine := `
DELETE FROM machine 
WHERE uuid = $entityUUID.uuid;
`
	deleteMachineStmt, err := st.Prepare(deleteMachine, machineUUIDParam)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for deleting net node row.
	// TODO (stickupkid): We need to ensure that no unit is still using this
	// net node. If it is, we need to return an error.
	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM machine WHERE uuid = $entityUUID.uuid)
`
	deleteNodeStmt, err := st.Prepare(deleteNode, machineUUIDParam)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryMachineStmt, machineUUIDParam).Get(&machineUUIDParam)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		} else if err != nil {
			return errors.Errorf("looking up machine UUID: %w", err)
		}

		// Remove all basic machine data associated with the machine.
		if err := st.removeBasicMachineData(ctx, tx, machineUUIDParam.UUID); err != nil {
			return errors.Errorf("removing basic machine data: %w", err)
		}

		// Remove block devices for the machine.
		// TODO (stickupkid): This is wrong! Just dump the block devices
		// removal logic into this package.
		if err := blockdevice.RemoveMachineBlockDevices(ctx, tx, machineUUIDParam.UUID); err != nil {
			return errors.Errorf("deleting block devices: %w", err)
		}

		if err := tx.Query(ctx, deleteMachineStmt, machineUUIDParam).Run(); err != nil {
			return errors.Errorf("deleting machine: %w", err)
		}

		// Remove the net node for the machine.
		if err := tx.Query(ctx, deleteNodeStmt, machineUUIDParam).Run(); err != nil {
			return errors.Errorf("deleting net node: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("deleting machine: %w", err)
	}
	return nil
}

func (st *State) removeBasicMachineData(ctx context.Context, tx *sqlair.TX, mUUID string) error {
	machineUUIDRec := entityUUID{UUID: mUUID}

	tables := []string{
		"machine_status",
		"machine_cloud_instance_status",
		"machine_cloud_instance",
		"machine_platform",
		"machine_agent_version",
		"machine_constraint",
		"machine_volume",
		"machine_filesystem",
		"machine_requires_reboot",
		"machine_lxd_profile",
		"machine_agent_presence",
		"machine_container_type",
	}

	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE machine_uuid = $entityUUID.uuid", table)
		stmt, err := st.Prepare(query, machineUUIDRec)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, machineUUIDRec).Run(); err != nil {
			return errors.Errorf("deleting reference to machine in table %q: %w", table, err)
		}
	}
	return nil
}

func (st *State) getMachineLife(ctx context.Context, tx *sqlair.TX, mUUID string) (life.Life, error) {
	var machineLife entityLife
	machineUUID := entityUUID{UUID: mUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   machine
WHERE  uuid = $entityUUID.uuid;`, machineLife, machineUUID)
	if err != nil {
		return -1, errors.Errorf("preparing machine life query: %w", err)
	}

	err = tx.Query(ctx, stmt, machineUUID).Get(&machineLife)
	if errors.Is(err, sqlair.ErrNoRows) {
		return -1, machineerrors.MachineNotFound
	} else if err != nil {
		return -1, errors.Errorf("running machine life query: %w", err)
	}

	return machineLife.Life, errors.Capture(err)
}
