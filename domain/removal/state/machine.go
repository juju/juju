// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// MachineExists returns true if a machine exists with the input UUID.
func (st *State) MachineExists(ctx context.Context, uUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	machineUUID := entityUUID{UUID: uUUID}
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
