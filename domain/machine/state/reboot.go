// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/internal/database"
)

// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
//
// Reboot requests are handled through the "machine_requires_reboot" table which contains only
// machine UUID for which a reboot has been requested.
// This function is idempotent.
func (st *State) RequireMachineReboot(ctx context.Context, uuid string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	setRebootFlag := `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ($machineUUID.uuid)`
	setRebootFlagStmt, err := sqlair.Prepare(setRebootFlag, machineUUID{})
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, setRebootFlagStmt, machineUUID{uuid}).Run()
	})
	if database.IsErrConstraintPrimaryKey(err) {
		// if the same uuid is added twice, do nothing (idempotency)
		return nil
	}
	return errors.Annotatef(err, "requiring reboot of machine %q", uuid)
}

// CancelMachineReboot cancels the reboot of the machine referenced by its UUID if it has
// previously been required.
//
// It basically removes the uuid from the "machine_requires_reboot" table if present.
// This function is idempotent.
func (st *State) CancelMachineReboot(ctx context.Context, uuid string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}
	unsetRebootFlag := `DELETE FROM machine_requires_reboot WHERE machine_uuid = $machineUUID.uuid`
	unsetRebootFlagStmt, err := sqlair.Prepare(unsetRebootFlag, machineUUID{})
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, unsetRebootFlagStmt, machineUUID{uuid}).Run()
	})
	return errors.Annotatef(err, "cancelling reboot of machine %q", uuid)
}

// IsMachineRebootRequired checks if the specified machine requires a reboot.
//
// It queries the "machine_requires_reboot" table for the machine UUID to determine if a reboot is required.
// Returns a boolean value indicating if a reboot is required, and an error if any occur during the process.
func (st *State) IsMachineRebootRequired(ctx context.Context, uuid string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	var isRebootRequired bool
	isRebootFlag := `SELECT machine_uuid as &machineUUID.uuid  FROM machine_requires_reboot WHERE machine_uuid = $machineUUID.uuid`
	isRebootFlagStmt, err := sqlair.Prepare(isRebootFlag, machineUUID{})
	if err != nil {
		return false, errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var results machineUUID
		err := tx.Query(ctx, isRebootFlagStmt, machineUUID{uuid}).Get(&results)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}
		isRebootRequired = !errors.Is(err, sqlair.ErrNoRows)
		return nil
	})

	return isRebootRequired, errors.Annotatef(err, "requiring reboot of machine %q", uuid)
}
