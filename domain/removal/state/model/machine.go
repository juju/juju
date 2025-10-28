// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// MachineExists returns true if a machine exists with the input UUID.
func (st *State) MachineExists(ctx context.Context, mUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	machineUUID := entityUUID{UUID: mUUID}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
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
// the input machine UUID, that is still alive. This will mark all units on the
// machine as dying, as well as any child container machines that are also
// running on the same parent machine. The returned units and child machines
// uuids can then be used to ensure the units and machines are correctly
// cascaded to the dying state.
func (st *State) EnsureMachineNotAliveCascade(
	ctx context.Context, mUUID string, force bool,
) (internal.CascadedMachineLives, error) {
	var cascaded internal.CascadedMachineLives

	db, err := st.DB(ctx)
	if err != nil {
		return cascaded, errors.Capture(err)
	}

	machineUUID := entityUUID{UUID: mUUID}
	updateMachineStmt, err := st.Prepare(`
UPDATE machine
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, machineUUID)
	if err != nil {
		return cascaded, errors.Errorf("preparing machine life update: %w", err)
	}

	updateInstanceStmt, err := st.Prepare(`
UPDATE machine_cloud_instance
SET    life_id = 1
WHERE  machine_uuid = $entityUUID.uuid
AND    life_id = 0;`, machineUUID)
	if err != nil {
		return cascaded, errors.Errorf("preparing machine cloud instance life update: %w", err)
	}

	// Mark any container machines (0/lxd/0) that are also on the same machine
	// as dying. Also mark, any units on the machine as dying as well. This
	// is the inverse of the marking the last unit on the machine as dying.

	selectContainerMachines, err := st.Prepare(`
SELECT    mp.machine_uuid AS &entityUUID.uuid
FROM      machine_parent AS mp
JOIN      machine AS m ON mp.parent_uuid = m.uuid
WHERE     mp.parent_uuid = $entityUUID.uuid
AND       m.life_id = 0;`, machineUUID)
	if err != nil {
		return cascaded, errors.Errorf("preparing container machine selection query: %w", err)
	}

	updateContainerStmt, err := st.Prepare(`
UPDATE machine
SET    life_id = 1
WHERE  uuid IN ($uuids[:])
AND    life_id = 0;`, uuids{})
	if err != nil {
		return cascaded, errors.Errorf("preparing container machine life update: %w", err)
	}

	updateContainerInstanceStmt, err := st.Prepare(`
UPDATE machine_cloud_instance
SET    life_id = 1
WHERE  machine_uuid IN ($uuids[:])
AND    life_id = 0;`, uuids{})
	if err != nil {
		return cascaded, errors.Errorf("preparing container machine instance life update: %w", err)
	}

	// Select any units that are directly on the parent machine.
	selectUnitStmt, err := st.Prepare(`
SELECT u.uuid AS &entityUUID.uuid
FROM   unit AS u
JOIN   machine  AS m ON m.net_node_uuid = u.net_node_uuid
WHERE  m.uuid IN ($uuids[:])
AND    u.life_id = 0;`, machineUUID, uuids{})
	if err != nil {
		return cascaded, errors.Errorf("preparing unit selection query: %w", err)
	}

	if err := errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Remove any container machines that are on the same parent machine
		// as the input machine.
		var machineUUIDs []entityUUID
		err := tx.Query(ctx, selectContainerMachines, machineUUID).GetAll(&machineUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("selecting container machines: %w", err)
		}

		if !force && len(machineUUIDs) > 0 {
			return errors.Errorf(
				"cannot set machine %q to dying without force: %w", mUUID, removalerrors.MachineHasContainers)
		}

		var parentUnitUUIDs []entityUUID
		err = tx.Query(ctx, selectUnitStmt, uuids{machineUUID.UUID}).GetAll(&parentUnitUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("selecting parent units: %w", err)
		}

		if !force && len(parentUnitUUIDs) > 0 {
			return errors.Errorf(
				"cannot set machine %q to dying without force: %w", mUUID, removalerrors.MachineHasUnits)
		}

		if err := tx.Query(ctx, updateMachineStmt, machineUUID).Run(); err != nil {
			return errors.Errorf("advancing machine life: %w", err)
		}

		if err := tx.Query(ctx, updateInstanceStmt, machineUUID).Run(); err != nil {
			return errors.Errorf("advancing machine cloud instance life: %w", err)
		}

		var childUnitUUIDs []entityUUID
		if len(machineUUIDs) > 0 {
			cascaded.MachineUUIDs = transform.Slice(machineUUIDs, func(u entityUUID) string {
				return u.UUID
			})

			if err := tx.Query(ctx, updateContainerStmt, uuids(cascaded.MachineUUIDs)).Run(); err != nil {
				return errors.Errorf("advancing container machine life: %w", err)
			}
			if err := tx.Query(ctx, updateContainerInstanceStmt, uuids(cascaded.MachineUUIDs)).Run(); err != nil {
				return errors.Errorf("advancing container machine instance life: %w", err)
			}

			// If there are any container machines, we also need to
			// select any units that are on those machines.
			// Note that this is safe because:
			// 1. The UI requires force if the machine has any containers
			//    or units.
			// 2. If this was cascaded from application or unit, we already
			//    determined that only the dying unit was attached to this
			//    machine (in which case there will be no containers).
			err := tx.Query(ctx, selectUnitStmt, uuids(cascaded.MachineUUIDs)).GetAll(&childUnitUUIDs)
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("selecting container units: %w", err)
			}
		}

		cascaded.CascadedStorageLives, err = st.ensureMachineStorageInstancesNotAliveCascade(ctx, tx, mUUID)
		if err != nil {
			return errors.Errorf("advancing machine storage entity lives: %w", err)
		}

		// If there are no units to update, we can return early.
		if len(parentUnitUUIDs)+len(childUnitUUIDs) == 0 {
			return nil
		}

		const (
			checkEmptyMachine = false
			destroyStorage    = false
		)
		cascaded.UnitUUIDs = transform.Slice(append(parentUnitUUIDs, childUnitUUIDs...), func(u entityUUID) string {
			return u.UUID
		})
		for _, u := range cascaded.UnitUUIDs {
			uc, err := st.ensureUnitNotAliveCascade(ctx, tx, u, checkEmptyMachine, destroyStorage)
			if err != nil {
				return errors.Errorf("cascading unit %q life advancement: %w", u, err)
			}
			// We don't expect storage instances to advance because we pass
			// destroyStorage as false, but we can have dying unit attachments.
			cascaded.StorageAttachmentUUIDs = append(cascaded.StorageAttachmentUUIDs, uc.StorageAttachmentUUIDs...)
		}

		return nil
	})); err != nil {
		return cascaded, errors.Capture(err)
	}

	return cascaded, nil
}

// ensureMachineStorageInstancesNotAliveCascade transitions any "alive"
// storage instances attached to this machine to "dying" along with said
// attachments. If any attached file-systems or volumes were provisioned
// with machine scope, those will be "dying" too.
func (st *State) ensureMachineStorageInstancesNotAliveCascade(
	ctx context.Context, tx *sqlair.TX, mUUID string,
) (internal.CascadedStorageLives, error) {
	var cascaded internal.CascadedStorageLives
	machineUUID := entityUUID{UUID: mUUID}

	q := `
WITH all_instances AS (
    SELECT iv.storage_instance_uuid
    FROM   storage_instance i 
           JOIN storage_instance_volume iv ON i.uuid = iv.storage_instance_uuid
           JOIN storage_volume_attachment va ON iv.storage_volume_uuid = va.storage_volume_uuid
           JOIN machine m ON va.net_node_uuid = m.net_node_uuid
    WHERE  m.uuid = $entityUUID.uuid
    AND    i.life_id = 0
    UNION
    SELECT if.storage_instance_uuid
    FROM   storage_instance i 
           JOIN storage_instance_filesystem if ON i.uuid = if.storage_instance_uuid
           JOIN storage_filesystem_attachment fa ON if.storage_filesystem_uuid = fa.storage_filesystem_uuid
           JOIN machine m ON fa.net_node_uuid = m.net_node_uuid
    WHERE  m.uuid = $entityUUID.uuid
    AND    i.life_id = 0
)
SELECT storage_instance_uuid as &entityUUID.uuid FROM all_instances`

	stmt, err := st.Prepare(q, machineUUID)
	if err != nil {
		return cascaded, errors.Errorf("preparing machine storage instance query: %w", err)
	}

	var sUUIDs []entityUUID
	if err := tx.Query(ctx, stmt, machineUUID).GetAll(&sUUIDs); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return cascaded, nil
		}
		return cascaded, errors.Errorf("running machine storage instance query: %w", err)
	}

	for _, sUUID := range sUUIDs {
		c, err := st.ensureStorageInstanceNotAliveCascade(ctx, tx, sUUID)
		if err != nil {
			return cascaded, errors.Errorf("killing storage instance %q: %w", sUUID.UUID, err)
		}
		cascaded.StorageInstanceUUIDs = append(cascaded.StorageInstanceUUIDs, sUUID.UUID)
		cascaded = cascaded.MergeInstance(c)
	}
	return cascaded, nil
}

// MachineScheduleRemoval schedules a removal job for the machine with the
// input UUID, qualified with the input force boolean.
// We don't care if the machine does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) MachineScheduleRemoval(
	ctx context.Context, removalUUID, machineUUID string, force bool, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.MachineJob),
		EntityUUID:    machineUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing machine removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling machine removal: %w", err)
		}
		return nil
	}))
}

// GetMachineLife returns the life of the machine with the input UUID.
func (st *State) GetMachineLife(ctx context.Context, mUUID string) (life.Life, error) {
	db, err := st.DB(ctx)
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

// GetInstanceLife returns the life of the machine instance with the input UUID.
func (st *State) GetInstanceLife(ctx context.Context, mUUID string) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var life life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		life, err = st.getInstanceLife(ctx, tx, mUUID)

		return errors.Capture(err)
	})

	return life, errors.Capture(err)
}

// GetMachineNetworkInterfaces returns the network interfaces for the
// machine with the input UUID. This is used to release any addresses that the
// machine has allocated.
// This will only return interfaces that have a non-null MAC address and
// if the machine is a container machine (i.e. lxd container machine).
func (st *State) GetMachineNetworkInterfaces(ctx context.Context, machineUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	selectStmt, err := st.Prepare(`
SELECT  lld.mac_address AS &linkLayerDevice.hardware_address
FROM    machine AS m
JOIN    net_node AS n ON n.uuid = m.net_node_uuid
JOIN    machine_parent AS mp ON mp.machine_uuid = m.uuid
JOIN    link_layer_device AS lld ON lld.net_node_uuid = n.uuid
WHERE   m.uuid = $entityUUID.uuid
AND     m.life_id = 1
AND     lld.mac_address IS NOT NULL;`, entityUUID{UUID: machineUUID}, linkLayerDevice{})
	if err != nil {
		return nil, errors.Errorf("preparing machine network interfaces selection: %w", err)
	}
	var interfaces []linkLayerDevice
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, selectStmt, entityUUID{UUID: machineUUID}).
			GetAll(&interfaces)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting machine %q network interfaces: %w", machineUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.Slice(interfaces, func(v linkLayerDevice) string {
		return v.HardwareAddress
	}), nil
}

// MarkMachineAsDead marks the machine with the input UUID as dead.
// The following errors are returned:
// - [machineerrors.MachineNotFound] if the machine does not exist.
// - [removalerrors.EntityStillAlive] if the machine is alive.
// - [removalerrors.MachineHasContainers] if the machine hosts containers.
// - [removalerrors.MachineHasUnits] if the machine hosts units.
func (st *State) MarkMachineAsDead(ctx context.Context, mUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	machineUUID := entityUUID{UUID: mUUID}
	updateStmt, err := st.Prepare(`
UPDATE machine
SET    life_id = 2
WHERE  uuid = $entityUUID.uuid
AND    life_id = 1`, machineUUID)
	if err != nil {
		return errors.Errorf("preparing machine life update: %w", err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if l, err := st.getMachineLife(ctx, tx, mUUID); err != nil {
			return errors.Errorf("getting machine life: %w", err)
		} else if l == life.Dead {
			return nil
		} else if l == life.Alive {
			return removalerrors.EntityStillAlive
		}

		err = st.checkNoMachineDependents(ctx, tx, machineUUID)
		if err != nil {
			return errors.Capture(err)
		}

		err := tx.Query(ctx, updateStmt, machineUUID).Run()
		if err != nil {
			return errors.Errorf("marking machine as dead: %w", err)
		}

		return nil
	}))
}

// MarkInstanceAsDead marks the machine cloud instance with the input UUID as
// dead.
// The following errors are returned:
// - [machineerrors.MachineNotFound] if the machine does not exist.
// - [removalerrors.EntityStillAlive] if the machine is alive.
// - [removalerrors.MachineHasContainers] if the machine hosts containers.
// - [removalerrors.MachineHasUnits] if the machine hosts units.
func (st *State) MarkInstanceAsDead(ctx context.Context, mUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	machineUUID := entityUUID{UUID: mUUID}
	updateStmt, err := st.Prepare(`
UPDATE machine_cloud_instance
SET    life_id = 2
WHERE  machine_uuid = $entityUUID.uuid
AND    life_id = 1`, machineUUID)
	if err != nil {
		return errors.Errorf("preparing instance life update: %w", err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if l, err := st.getInstanceLife(ctx, tx, mUUID); err != nil {
			return errors.Errorf("getting machine instance life: %w", err)
		} else if l == life.Dead {
			return nil
		} else if l == life.Alive {
			return removalerrors.EntityStillAlive
		}

		err = st.checkNoMachineDependents(ctx, tx, machineUUID)
		if err != nil {
			return errors.Capture(err)
		}

		err := tx.Query(ctx, updateStmt, machineUUID).Run()
		if err != nil {
			return errors.Errorf("marking machine instance as dead: %w", err)
		}

		return nil
	}))
}

// DeleteMachine deletes the specified machine and any dependent child records.
func (st *State) DeleteMachine(ctx context.Context, mUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	type node entityUUID
	type machine entityUUID
	machineUUIDParam := entityUUID{UUID: mUUID}

	// Prepare query to fetch net node for machine
	getNode := `
SELECT net_node_uuid AS &node.uuid 
FROM   machine 
WHERE  uuid = $machine.uuid
`
	getNodeStmt, err := st.Prepare(getNode, machine{}, node{})
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for deleting machine row.
	deleteMachine := `
DELETE FROM machine 
WHERE uuid = $machine.uuid;
`
	deleteMachineStmt, err := st.Prepare(deleteMachine, machine{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		mLife, err := st.getMachineLife(ctx, tx, machineUUIDParam.UUID)
		if err != nil {
			return errors.Errorf("getting machine life: %w", err)
		} else if mLife == life.Alive {
			return errors.Errorf("cannot delete machine %q, machine is still alive", machineUUIDParam.UUID).
				Add(removalerrors.EntityStillAlive)
		} else if mLife == life.Dying {
			return errors.Errorf("waiting for machine to be dead before deletion").
				Add(removalerrors.RemovalJobIncomplete)
		}

		// Delete all tasks related to the machine, and eventually removes
		// operations if they are empty after tasks deletion.
		_, err = st.cleanupTasksAndOperationsByMachineUUID(ctx, tx, machineUUIDParam.UUID)
		if err != nil {
			return errors.Errorf("deleting operation: %w", err)
		}

		// Check to see if the machine_cloud_instance is still alive. If it is,
		// we cannot delete the machine. It is expected that the provisioner
		// will have removed the instance before calling this method.
		iLife, err := st.getInstanceLife(ctx, tx, machineUUIDParam.UUID)
		if err != nil {
			return errors.Errorf("getting machine instance life: %w", err)
		} else if iLife == life.Alive {
			return errors.Errorf("cannot delete machine %q, instance is still alive", machineUUIDParam.UUID)
		} else if iLife == life.Dying {
			return errors.Errorf("waiting for instance to be dead before deletion").Add(removalerrors.RemovalJobIncomplete)
		}

		err = st.checkNoMachineDependents(ctx, tx, machineUUIDParam)
		if err != nil {
			return errors.Errorf("checking for dependents: %w", err).Add(removalerrors.RemovalJobIncomplete)
		}

		// Remove all basic machine data associated with the machine.
		if err := st.removeBasicMachineData(ctx, tx, machineUUIDParam.UUID); err != nil {
			return errors.Errorf("removing basic machine data: %w", err)
		}

		// Get the net node for the machine.
		var node node
		if err := tx.Query(ctx, getNodeStmt, machine(machineUUIDParam)).Get(&node); err != nil {
			return errors.Errorf("getting net node: %w", err)
		}

		// Remove the machine entry
		if err := tx.Query(ctx, deleteMachineStmt, machine(machineUUIDParam)).Run(); err != nil {
			return errors.Errorf("deleting machine: %w", err)
		}

		// Remove the machine's net node.
		if err := st.removeNetNode(ctx, tx, node.UUID); err != nil {
			return errors.Errorf("removing machine network: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("deleting machine: %w", err)
	}
	return nil
}

func (st *State) checkNoMachineDependents(ctx context.Context, tx *sqlair.TX, machineUUIDParam entityUUID) error {
	countContainersOnMachine, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM machine_parent
WHERE parent_uuid = $entityUUID.uuid
`, count{}, machineUUIDParam)
	if err != nil {
		return errors.Capture(err)
	}

	countUnitsOnMachine, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM unit
JOIN machine ON machine.net_node_uuid = unit.net_node_uuid
WHERE machine.uuid = $entityUUID.uuid
AND unit.life_id != 2
`, count{}, machineUUIDParam)
	if err != nil {
		return errors.Capture(err)
	}

	var containerCount count
	err = tx.Query(ctx, countContainersOnMachine, machineUUIDParam).Get(&containerCount)
	if err != nil {
		return errors.Errorf("getting container count: %w", err)
	} else if containerCount.Count > 0 {
		return errors.Errorf("cannot delete machine %q, it hosts has %d container(s)", machineUUIDParam.UUID, containerCount.Count).
			Add(removalerrors.MachineHasContainers)
	}

	var unitCount count
	err = tx.Query(ctx, countUnitsOnMachine, machineUUIDParam).Get(&unitCount)
	if err != nil {
		return errors.Errorf("getting unit count: %w", err)
	} else if unitCount.Count > 0 {
		return errors.Errorf("cannot delete machine %q, it hosts has %d unit(s)", machineUUIDParam.UUID, unitCount.Count).
			Add(removalerrors.MachineHasUnits)
	}

	return nil
}

func (st *State) removeBasicMachineData(ctx context.Context, tx *sqlair.TX, mUUID string) error {
	machineUUIDRec := entityUUID{UUID: mUUID}

	tables := []string{
		"DELETE FROM machine_volume WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_filesystem WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_manual WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_agent_version WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM instance_tag WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_status WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_cloud_instance_status WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_cloud_instance WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_container_type WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_platform WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_agent_version WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_constraint WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_requires_reboot WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_lxd_profile WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_agent_presence WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_container_type WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_ssh_host_key WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_parent WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM block_device_link_device WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM block_device WHERE machine_uuid = $entityUUID.uuid",
		// TODO(storage): remove these once storage removal is complete.
		"DELETE FROM machine_filesystem WHERE machine_uuid = $entityUUID.uuid",
		"DELETE FROM machine_volume WHERE machine_uuid = $entityUUID.uuid",
	}

	for _, table := range tables {
		stmt, err := st.Prepare(table, machineUUIDRec)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, machineUUIDRec).Run(); err != nil {
			return errors.Errorf("deleting reference to machine in table %q: %w", table, err)
		}
	}
	return nil
}

func (st *State) removeNetNode(ctx context.Context, tx *sqlair.TX, netNodeUUID string) error {
	type node entityUUID
	netNodeUUIDRec := node{UUID: netNodeUUID}

	// Start by deleting any IP addresses associated with the net node.
	q := `
DELETE FROM provider_ip_address
WHERE address_uuid IN (
    SELECT uuid FROM ip_address WHERE net_node_uuid = $node.uuid
)`
	stmt, err := st.Prepare(q, node{})
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, stmt, netNodeUUIDRec).Run(); err != nil {
		return errors.Errorf("deleting net node IP address provider IDs: %w", err)
	}

	stmt, err = st.Prepare("DELETE FROM ip_address WHERE net_node_uuid = $node.uuid", node{})
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, stmt, netNodeUUIDRec).Run(); err != nil {
		return errors.Errorf("deleting net node IP addresses: %w", err)
	}

	// Get all link-layer devices associated with the net node.
	selectDevices := `
SELECT uuid AS &entityUUID.uuid
FROM   link_layer_device
WHERE  net_node_uuid = $node.uuid`
	devStmt, err := st.Prepare(selectDevices, entityUUID{}, netNodeUUIDRec)
	if err != nil {
		return errors.Errorf("preparing devices for deletion statement: %w", err)
	}

	var devsToDelete []entityUUID
	if err := tx.Query(ctx, devStmt, netNodeUUIDRec).GetAll(&devsToDelete); err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("running devices for deletion query: %w", err)
		}
	}

	llDevicesToDelete := uuids(transform.Slice(devsToDelete, func(d entityUUID) string { return d.UUID }))

	// Delete all relations that reference the link-layer devices and the devices themselves.
	deleteDevicesRelations := []string{
		`DELETE FROM link_layer_device_parent WHERE device_uuid IN ($uuids[:]) OR parent_uuid IN ($uuids[:])`,
		`DELETE FROM provider_link_layer_device WHERE device_uuid IN ($uuids[:])`,
		`DELETE FROM link_layer_device_dns_domain WHERE device_uuid IN ($uuids[:])`,
		`DELETE FROM link_layer_device_dns_address WHERE device_uuid IN ($uuids[:])`,
		`DELETE FROM link_layer_device WHERE uuid IN ($uuids[:])`,
	}
	for _, query := range deleteDevicesRelations {
		stmt, err := st.Prepare(query, llDevicesToDelete)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, llDevicesToDelete).Run(); err != nil {
			removeErr := errors.Errorf("deleting reference to machine network in query %q: %w", query, err)
			if database.IsErrConstraintForeignKey(err) {
				removeErr = removeErr.Add(removalerrors.RemovalJobIncomplete)
			}
			return removeErr
		}
	}

	// Delete the net node.

	deleteByNetNode := []string{
		// TODO (stickupkid): We need to ensure that no unit is still using this
		// net node. If it is, we need to return an error.
		"DELETE FROM net_node WHERE uuid = $node.uuid",
	}

	for _, query := range deleteByNetNode {
		stmt, err := st.Prepare(query, node{})
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, netNodeUUIDRec).Run(); err != nil {
			removeErr := errors.Errorf("deleting net node in query %q: %w", query, err)
			if database.IsErrConstraintForeignKey(err) {
				removeErr = removeErr.Add(removalerrors.RemovalJobIncomplete)
			}
			return removeErr
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

	return life.Life(machineLife.Life), nil
}

func (st *State) getInstanceLife(ctx context.Context, tx *sqlair.TX, mUUID string) (life.Life, error) {
	var instance entityLife
	machineUUID := entityUUID{UUID: mUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   machine_cloud_instance
WHERE  machine_uuid = $entityUUID.uuid;`, instance, machineUUID)
	if err != nil {
		return -1, errors.Errorf("preparing machine instance life query: %w", err)
	}

	err = tx.Query(ctx, stmt, machineUUID).Get(&instance)
	if errors.Is(err, sqlair.ErrNoRows) {
		return -1, machineerrors.MachineNotFound
	} else if err != nil {
		return -1, errors.Errorf("running machine instance life query: %w", err)
	}

	return life.Life(instance.Life), nil
}
