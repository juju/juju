// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/sqlair"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	removalerrors "github.com/juju/juju/domain/removal/errors"
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

// EnsureUnitNotAliveCascade ensures that there is no unit identified by the
// input unit UUID, that is still alive. If the unit is the last one on the
// machine, it will cascade and the machine is also set to dying. The
// affected machine UUID is returned.
func (st *State) EnsureUnitNotAliveCascade(ctx context.Context, uUUID string) (machineUUID string, err error) {
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

	var mUUID string
	if err := errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, updateUnitStmt, unitUUID).Run(); err != nil {
			return errors.Errorf("advancing unit life: %w", err)
		}

		mUUID, err = st.markMachineAsDyingIfAllUnitsAreNotAlive(ctx, tx, uUUID)
		if err != nil {
			return errors.Errorf("marking last unit on machine as dying: %w", err)
		}

		return nil
	})); err != nil {
		return "", err
	}

	return mUUID, nil
}

// markMachineAsDyingIfAllUnitsAreNotAlive checks if all the units on the
// machine are not alive. If this is the case, it marks the machine as dying.
func (st *State) markMachineAsDyingIfAllUnitsAreNotAlive(
	ctx context.Context, tx *sqlair.TX, uUUID string,
) (machineUUID string, err error) {
	unitUUID := entityUUID{UUID: uUUID}

	lastUnitStmt, err := st.Prepare(`
WITH units_alive AS (
    SELECT uuid, net_node_uuid
    FROM   unit
    WHERE  life_id = 0
), units_not_alive AS (
    SELECT uuid, net_node_uuid
    FROM   unit
    WHERE  life_id != 0
), machines AS (
    SELECT    m.uuid AS machine_uuid,
              m.net_node_uuid,
              COUNT(ua.uuid) AS unit_alive_count,
              COUNT(una.uuid) AS unit_not_alive_count,
			  COUNT(mp.parent_uuid) AS machine_parent_count
    FROM      machine AS m
    JOIN      net_node AS nn ON nn.uuid = m.net_node_uuid
    LEFT JOIN units_alive AS ua ON ua.net_node_uuid = nn.uuid
    LEFT JOIN units_not_alive AS una ON una.net_node_uuid = nn.uuid
	LEFT JOIN machine_parent AS mp ON mp.parent_uuid = m.uuid
    GROUP BY  m.uuid
)
SELECT unit_alive_count AS &unitMachineLifeSummary.alive_count,
       unit_not_alive_count AS &unitMachineLifeSummary.not_alive_count,
	   machine_parent_count AS &unitMachineLifeSummary.machine_parent_count,
       machine_uuid AS &unitMachineLifeSummary.uuid
FROM   machines
LEFT JOIN unit AS u ON u.net_node_uuid = machines.net_node_uuid
WHERE  u.uuid = $entityUUID.uuid;
    `, unitUUID, unitMachineLifeSummary{})
	if err != nil {
		return "", errors.Errorf("preparing unit count query: %w", err)
	}

	updateMachineStmt, err := st.Prepare(`
UPDATE machine
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, entityUUID{})
	if err != nil {
		return "", errors.Errorf("preparing machine life update: %w", err)
	}

	var result unitMachineLifeSummary
	if err := tx.Query(ctx, lastUnitStmt, unitUUID).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
		return "", nil
	} else if err != nil {
		return "", errors.Errorf("getting unit count: %w", err)
	} else if result.AliveCount > 0 {
		// Nothing to do.
		return "", nil
	} else if result.NotAliveCount == 0 {
		// No units on the machine are marked as dead or dying. If this is the
		// case then we can assume that the machine is still alive.
		return "", nil
	} else if result.MachineParentCount > 0 {
		// There are child machines associated with this machine.
		// We cannot mark the machine as dying if it has child machines.
		return "", nil
	}

	// We can use the outcome of the update to determine if the machine
	// was already dying or dead, or if it was successfully advanced to dying.
	var outcome sqlair.Outcome
	if err := tx.Query(ctx, updateMachineStmt, entityUUID{UUID: result.UUID}).Get(&outcome); err != nil {
		return "", errors.Errorf("advancing machine life: %w", err)
	}

	if affected, err := outcome.Result().RowsAffected(); err != nil {
		return "", errors.Errorf("getting affected rows: %w", err)
	} else if affected == 0 {
		// The machine was already dying or dead.
		return "", nil
	}

	return result.UUID, nil
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

	var life life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		life, err = st.getUnitLife(ctx, tx, uUUID)

		return errors.Capture(err)
	})

	return life, errors.Capture(err)
}

// GetApplicationNameAndUnitNameByUnitUUID retrieves the application name and
// unit name for a unit identified by the input UUID. If the unit does not
// exist, it returns an error.
func (st *State) GetApplicationNameAndUnitNameByUnitUUID(ctx context.Context, uUUID string) (string, string, error) {
	db, err := st.DB()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	unitUUID := entityUUID{UUID: uUUID}
	stmt, err := st.Prepare(`
SELECT    a.name AS &applicationUnitName.application_name,
          u.name AS &applicationUnitName.unit_name
FROM      unit AS u
LEFT JOIN application AS a ON a.uuid = u.application_uuid
WHERE     u.uuid = $entityUUID.uuid;`, applicationUnitName{}, unitUUID)
	if err != nil {
		return "", "", errors.Errorf("preparing unit application name and unit name query: %w", err)
	}

	var appUnitName applicationUnitName
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, unitUUID).Get(&appUnitName)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		} else if err != nil {
			return errors.Errorf("running unit application name and unit name query: %w", err)
		}
		return nil
	}); err != nil {
		return "", "", errors.Capture(err)
	}
	return appUnitName.ApplicationName, appUnitName.UnitName, nil
}

// MarkUnitAsDead marks the unit with the input UUID as dead.
func (st *State) MarkUnitAsDead(ctx context.Context, uUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	unitUUID := entityUUID{UUID: uUUID}
	updateStmt, err := st.Prepare(`
UPDATE unit
SET    life_id = 2
WHERE  uuid = $entityUUID.uuid
AND    life_id = 1`, unitUUID)
	if err != nil {
		return errors.Errorf("preparing unit life update: %w", err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if l, err := st.getUnitLife(ctx, tx, uUUID); err != nil {
			return errors.Errorf("getting unit life: %w", err)
		} else if l == life.Dead {
			return nil
		} else if l == life.Alive {
			return removalerrors.EntityStillAlive
		}

		err := tx.Query(ctx, updateStmt, unitUUID).Run()
		if err != nil {
			return errors.Errorf("marking unit as dead: %w", err)
		}

		return nil
	}))
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

	unitUUIDCount := entityAssociationCount{UUID: unitUUID}
	subordinateStmt, err := st.Prepare(`
SELECT count(*) AS &entityAssociationCount.count
FROM unit_principal
WHERE principal_uuid = $entityAssociationCount.uuid
`, unitUUIDCount)
	if err != nil {
		return errors.Capture(err)
	}

	unitUUIDRec := entityUUID{UUID: unitUUID}
	deleteUnitStmt, err := st.Prepare(`
DELETE FROM unit
WHERE  uuid = $entityUUID.uuid;`, unitUUIDRec)
	if err != nil {
		return errors.Errorf("preparing unit delete: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if uLife, err := st.getUnitLife(ctx, tx, unitUUID); err != nil {
			return errors.Errorf("getting unit life for unit %q: %w", unitUUID, err)
		} else if uLife == life.Alive {
			// The unit is still alive, we cannot delete it.
			return errors.Errorf("cannot delete unit %q as it is still alive", unitUUID).
				Add(removalerrors.EntityStillAlive)
		} else if uLife == life.Dying {
			// The unit is dying, we cannot delete it yet.
			return errors.Errorf("waiting for unit %q to be removed before deletion", unitUUID).
				Add(removalerrors.RemovalJobIncomplete)
		}

		var netNodeUUIDRec entityUUID
		if err := tx.Query(ctx, selectNetNodeStmt, unitUUIDRec).Get(&netNodeUUIDRec); errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		} else if err != nil {
			return errors.Errorf("getting net node UUID for unit %q: %w", unitUUID, err)
		}

		// Ensure that the unit has no associated subordinates.
		var numSubordinates entityAssociationCount
		err = tx.Query(ctx, subordinateStmt, unitUUIDCount).Get(&numSubordinates)
		if err != nil {
			return errors.Errorf("getting number of subordinates for unit %q: %w", unitUUID, err)
		} else if numSubordinates.Count > 0 {
			// It is required that all units have been completely removed
			// before the application can be removed.
			return errors.Errorf("cannot delete unit as it still associated subordinates").
				Add(removalerrors.RemovalJobIncomplete)
		}

		if err := st.deleteUnitAnnotations(ctx, tx, unitUUID); err != nil {
			return errors.Errorf("deleting annotations for unit %q: %w", unitUUID, err)
		}

		if err := st.deleteK8sPod(ctx, tx, unitUUID, netNodeUUIDRec.UUID); err != nil {
			return errors.Errorf("deleting cloud container for unit %q: %w", unitUUID, err)
		}

		if err := st.deleteForeignKeyUnitReferences(ctx, tx, unitUUID); err != nil {
			return errors.Errorf("deleting unit references for unit %q: %w", unitUUID, err)
		}

		// Get the charm UUID before we delete the unit.
		charmUUID, err := st.getCharmUUIDForUnit(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf("getting charm UUID for application: %w", err)
		}

		if err := tx.Query(ctx, deleteUnitStmt, unitUUIDRec).Run(); err != nil {
			return errors.Errorf("deleting unit for unit %q: %w", unitUUID, err)
		}

		// See if it's possible to delete the charm any more.
		if err := st.deleteCharmIfUnusedByUUID(ctx, tx, charmUUID); err != nil {
			return errors.Errorf("deleting charm if unused: %w", err)
		}

		return nil
	}))
}

func (st *State) getUnitLife(ctx context.Context, tx *sqlair.TX, uUUID string) (life.Life, error) {
	var unitLife entityLife
	unitUUID := entityUUID{UUID: uUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   unit
WHERE  uuid = $entityUUID.uuid;`, unitLife, unitUUID)
	if err != nil {
		return -1, errors.Errorf("preparing unit life query: %w", err)
	}

	err = tx.Query(ctx, stmt, unitUUID).Get(&unitLife)
	if errors.Is(err, sqlair.ErrNoRows) {
		return -1, applicationerrors.UnitNotFound
	} else if err != nil {
		return -1, errors.Errorf("running unit life query: %w", err)
	}

	return life.Life(unitLife.Life), nil
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

func (st *State) deleteK8sPod(ctx context.Context, tx *sqlair.TX, uUUID, netNodeUUID string) error {
	unitUUIDRec := unitUUID{UUID: uUUID}

	// Only delete the address if it's not on a machine (it's a k8s pod).
	// We don't want to delete the address if it's on a machine, because
	// the machine may still be alive and the address may be in use.

	selectK8sPodStmt, err := st.Prepare(`
SELECT COUNT(*) AS &entityAssociationCount.count
FROM   k8s_pod
WHERE  unit_uuid = $unitUUID.unit_uuid`, unitUUIDRec, entityAssociationCount{})
	if err != nil {
		return errors.Capture(err)
	}
	var k8sPodCount entityAssociationCount
	if err := tx.Query(ctx, selectK8sPodStmt, unitUUIDRec).Get(&k8sPodCount); errors.Is(err, sqlair.ErrNoRows) || k8sPodCount.Count == 0 {
		// No k8s pod, nothing to do.
		return nil
	} else if err != nil {
		return errors.Errorf("getting k8s pod count: %w", err)
	}

	// Delete the k8s pod ports and addresses.

	if err := st.deleteK8sPodPorts(ctx, tx, uUUID); err != nil {
		return errors.Errorf("removing cloud container ports: %w", err)
	}

	if err := st.deletedK8sPodAddresses(ctx, tx, netNodeUUID); err != nil {
		return errors.Errorf("removing cloud container addresses: %w", err)
	}

	deleteK8sPodStmt, err := st.Prepare(`
DELETE FROM k8s_pod
WHERE unit_uuid = $unitUUID.unit_uuid`, unitUUIDRec)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteK8sPodStmt, unitUUIDRec).Run(); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (st *State) deletedK8sPodAddresses(ctx context.Context, tx *sqlair.TX, netNodeID string) error {
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

func (st *State) deleteK8sPodPorts(ctx context.Context, tx *sqlair.TX, uUUID string) error {
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
		"unit_principal",
		"unit_resource",
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

func (st *State) getCharmUUIDForUnit(ctx context.Context, tx *sqlair.TX, uUUID string) (string, error) {
	appID := entityUUID{UUID: uUUID}

	stmt, err := st.Prepare(`
SELECT charm_uuid AS &entityUUID.uuid
FROM   unit
WHERE  uuid = $entityUUID.uuid`, appID)
	if err != nil {
		return "", errors.Errorf("preparing charm UUID query: %w", err)
	}

	var result entityUUID
	if err := tx.Query(ctx, stmt, appID).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
		// No charm associated with the unit, so we can skip this.
		return "", nil
	} else if err != nil {
		return "", errors.Errorf("running charm UUID query: %w", err)
	}
	return result.UUID, nil
}
