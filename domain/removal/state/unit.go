// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// UnitExists returns true if a unit exists with the input UUID.
func (st *State) UnitExists(ctx context.Context, uUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	unitUUID := entityUUID{UUID: uUUID}
	existsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   unit
WHERE  uuid = $entityUUID.uuid`, unitUUID)
	if err != nil {
		return false, errors.Errorf("preparing unit exists query: %w", err)
	}

	var unitExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, unitUUID).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running unit exists query: %w", err)
		}

		unitExists = true
		return nil
	})

	return unitExists, errors.Capture(err)
}

// EnsureUnitNotAlive ensures that there is no unit
// identified by the input UUID, that is still alive.
func (st *State) EnsureUnitNotAlive(ctx context.Context, uUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	unitUUID := entityUUID{UUID: uUUID}
	updateUnitStmt, err := st.Prepare(`
UPDATE unit
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, unitUUID)
	if err != nil {
		return errors.Errorf("preparing unit life update: %w", err)
	}

	lastUnitStmt, err := st.Prepare(`
With machines AS (
	SELECT    m.uuid AS machine_uuid,
	          u.uuid AS unit_uuid,
			  COUNT(u.uuid) AS unit_count
	FROM      machine AS m
	JOIN      net_node AS nn ON nn.uuid = m.net_node_uuid
	LEFT JOIN unit AS u ON u.net_node_uuid = nn.uuid
	GROUP BY  m.uuid
)
SELECT unit_count AS &entityAssoicationCount.count,
	   machine_uuid AS &entityAssoicationCount.uuid
FROM   machines
WHERE  unit_uuid = $entityUUID.uuid;
	`, unitUUID, entityAssoicationCount{})
	if err != nil {
		return errors.Errorf("preparing unit count query: %w", err)
	}

	updateMachineStmt, err := st.Prepare(`
UPDATE machine
SET    life_id = 1
WHERE  uuid = $entityAssoicationCount.uuid
AND    life_id = 0`, entityAssoicationCount{})
	if err != nil {
		return errors.Errorf("preparing machine life update: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, updateUnitStmt, unitUUID).Run(); err != nil {
			return errors.Errorf("advancing unit life: %w", err)
		}

		var unitCount entityAssoicationCount
		if err := tx.Query(ctx, lastUnitStmt, unitUUID).Get(&unitCount); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting unit count: %w", err)
		} else if unitCount.Count != 1 {
			// The unit is not the last one on the machine.
			return nil
		}

		if err := tx.Query(ctx, updateMachineStmt, unitCount).Run(); err != nil {
			return errors.Errorf("advancing machine life: %w", err)
		}

		return nil
	}))
}

// UnitScheduleRemoval schedules a removal job for the unit with the
// input UUID, qualified with the input force boolean.
// We don't care if the unit does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) UnitScheduleRemoval(
	ctx context.Context, removalUUID, unitUUID string, force bool, when time.Time,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: 1,
		EntityUUID:    unitUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing unit removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling unit removal: %w", err)
		}
		return nil
	}))
}
