// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/sqlair"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
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
func (st *State) EnsureUnitNotAlive(ctx context.Context, uUUID string) (machineUUID string, err error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	unitUUID := entityUUID{UUID: uUUID}
	updateUnitStmt, err := st.Prepare(`
UPDATE unit
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, unitUUID)
	if err != nil {
		return "", errors.Errorf("preparing unit life update: %w", err)
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
		return "", errors.Errorf("preparing unit count query: %w", err)
	}

	updateMachineStmt, err := st.Prepare(`
UPDATE machine
SET    life_id = 1
WHERE  uuid = $entityAssoicationCount.uuid
AND    life_id = 0`, entityAssoicationCount{})
	if err != nil {
		return "", errors.Errorf("preparing machine life update: %w", err)
	}

	var mUUID string
	if err := errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
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

		mUUID = unitCount.UUID

		return nil
	})); err != nil {
		return "", err
	}

	return mUUID, nil
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

// GetUnitLife returns the life of the unit with the input UUID.
func (st *State) GetUnitLife(ctx context.Context, uUUID string) (life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return -1, errors.Capture(err)
	}

	var unitLife entityLife
	unitUUID := entityUUID{UUID: uUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   unit
WHERE  uuid = $entityUUID.uuid;`, unitLife, unitUUID)
	if err != nil {
		return -1, errors.Errorf("preparing unit life query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, unitUUID).Get(&unitLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		} else if err != nil {
			return errors.Errorf("running unit life query: %w", err)
		}

		return nil
	})

	return unitLife.Life, errors.Capture(err)
}

// DeleteUnit removes a unit from the database completely.
func (st *State) DeleteUnit(ctx context.Context, unitUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Get the net node UUID for the unit.
	selectNetNodeStmt, err := st.Prepare(`
SELECT    nn.uuid AS &entityUUID.uuid
FROM      unit AS u
LEFT JOIN net_node AS nn ON nn.uuid = u.net_node_uuid
WHERE     u.uuid = $entityUUID.uuid;`, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing unit net node query: %w", err)
	}

	unitUUIDRec := entityUUID{UUID: unitUUID}
	deleteUnitStmt, err := st.Prepare(`
DELETE FROM unit
WHERE  uuid = $entityUUID.uuid;`, unitUUIDRec)
	if err != nil {
		return errors.Errorf("preparing unit delete: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var netNodeUUIDRec entityUUID
		if err := tx.Query(ctx, selectNetNodeStmt, unitUUIDRec).Get(&netNodeUUIDRec); errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		} else if err != nil {
			return errors.Errorf("getting net node UUID for unit %q: %w", unitUUID, err)
		}

		if err := st.deleteUnitAnnotations(ctx, tx, unitUUID); err != nil {
			return errors.Errorf("deleting annotations for unit %q: %w", unitUUID, err)
		}

		if err := st.deleteCloudContainer(ctx, tx, unitUUID, netNodeUUIDRec.UUID); err != nil {
			return errors.Errorf("deleting cloud container for unit %q: %w", unitUUID, err)
		}

		if err := st.deleteForeignKeyUnitReferences(ctx, tx, unitUUID); err != nil {
			return errors.Errorf("deleting unit references for unit %q: %w", unitUUID, err)
		}

		if err := tx.Query(ctx, deleteUnitStmt, unitUUIDRec).Run(); err != nil {
			return errors.Errorf("deleting unit for unit %q: %w", unitUUID, err)
		}

		return nil
	}))
}

func (st *State) deleteUnitAnnotations(ctx context.Context, tx *sqlair.TX, uUUID string) error {
	unitUUIDRec := unitUUID{UUID: uUUID}

	deleteUnitAnnotationStmt, err := st.Prepare(`
DELETE FROM annotation_unit
WHERE  uuid = $unitUUID.unit_uuid`, unitUUIDRec)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteUnitAnnotationStmt, unitUUIDRec).Run(); err != nil {
		return errors.Errorf("removing unit annotations: %w", err)
	}
	return nil
}

func (st *State) deleteCloudContainer(ctx context.Context, tx *sqlair.TX, uUUID, netNodeUUID string) error {
	unitUUIDRec := unitUUID{UUID: uUUID}

	if err := st.deleteCloudContainerPorts(ctx, tx, uUUID); err != nil {
		return errors.Errorf("removing cloud container ports: %w", err)
	}

	if err := st.deleteCloudContainerAddresses(ctx, tx, netNodeUUID); err != nil {
		return errors.Errorf("removing cloud container addresses: %w", err)
	}

	deleteCloudContainerStmt, err := st.Prepare(`
DELETE FROM k8s_pod
WHERE unit_uuid = $unitUUID.unit_uuid`, unitUUIDRec)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteCloudContainerStmt, unitUUIDRec).Run(); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (st *State) deleteCloudContainerAddresses(ctx context.Context, tx *sqlair.TX, netNodeID string) error {
	netNodeIDRec := entityUUID{UUID: netNodeID}

	deleteAddressStmt, err := st.Prepare(`
DELETE FROM ip_address
WHERE device_uuid IN (
    SELECT device_uuid FROM link_layer_device lld
    WHERE lld.net_node_uuid = $entityUUID.uuid
)
`, netNodeIDRec)
	if err != nil {
		return errors.Capture(err)
	}
	deleteDeviceStmt, err := st.Prepare(`
DELETE FROM link_layer_device
WHERE net_node_uuid = $entityUUID.uuid`, netNodeIDRec)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteAddressStmt, netNodeIDRec).Run(); err != nil {
		return errors.Errorf("removing cloud container addresses for %q: %w", netNodeID, err)
	}
	if err := tx.Query(ctx, deleteDeviceStmt, netNodeIDRec).Run(); err != nil {
		return errors.Errorf("removing cloud container link layer devices for %q: %w", netNodeID, err)
	}
	return nil
}

func (st *State) deleteCloudContainerPorts(ctx context.Context, tx *sqlair.TX, uUUID string) error {
	unitUUIDRec := unitUUID{UUID: uUUID}

	deleteStmt, err := st.Prepare(`
DELETE FROM k8s_pod_port
WHERE unit_uuid = $unitUUID.unit_uuid`, unitUUIDRec)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteStmt, unitUUIDRec).Run(); err != nil {
		return errors.Errorf("removing cloud container ports: %w", err)
	}
	return nil
}

func (st *State) deleteForeignKeyUnitReferences(ctx context.Context, tx *sqlair.TX, uUUID string) error {
	unitUUIDRec := entityUUID{UUID: uUUID}

	for _, table := range []string{
		"unit_agent_version",
		"unit_state",
		"unit_state_charm",
		"unit_state_relation",
		"unit_agent_status",
		"unit_workload_status",
		"unit_workload_version",
		"k8s_pod_status",
		"port_range",
		"unit_constraint",
	} {
		deleteUnitReference := fmt.Sprintf(`DELETE FROM %s WHERE unit_uuid = $entityUUID.uuid`, table)
		deleteUnitReferenceStmt, err := st.Prepare(deleteUnitReference, unitUUIDRec)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, deleteUnitReferenceStmt, unitUUIDRec).Run(); err != nil {
			return errors.Errorf("deleting reference to unit in table %q: %w", table, err)
		}
	}
	return nil
}
