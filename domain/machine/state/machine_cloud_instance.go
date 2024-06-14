// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/domain"
)

// HardwareCharacteristics returns the hardware characteristics struct with
// data retrieved from the machine cloud instance table.
func (st *State) HardwareCharacteristics(
	ctx context.Context,
	machineUUID string,
) (*instance.HardwareCharacteristics, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	retrieveHardwareCharacteristics := `
SELECT (*) AS (&instanceData.*)
FROM   machine_cloud_instance 
WHERE  machine_uuid = $instanceData.machine_uuid`
	machineUUIDQuery := instanceData{
		MachineUUID: machineUUID,
	}
	retrieveHardwareCharacteristicsStmt, err := st.Prepare(retrieveHardwareCharacteristics, machineUUIDQuery)
	if err != nil {
		return nil, errors.Annotate(err, "preparing retrieve hardware characteristics statement")
	}

	var row instanceData
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, retrieveHardwareCharacteristicsStmt, machineUUIDQuery).Get(&row))
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.Annotatef(errors.NotFound, "machine cloud instance for machine %q", machineUUID)
		}
		return nil, errors.Annotatef(domain.CoerceError(err), "querying machine cloud instance for machine %q", machineUUID)
	}
	return row.toHardwareCharacteristics(), nil
}

// SetMachineCloudInstance sets an entry in the machine cloud instance table
// along with the instance tags and the link to a lxd profile if any.
func (st *State) SetMachineCloudInstance(
	ctx context.Context,
	machineUUID string,
	instanceID instance.Id,
	hardwareCharacteristics instance.HardwareCharacteristics,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	setInstanceData := `
INSERT INTO machine_cloud_instance (*)
VALUES ($instanceData.*)
`
	setInstanceDataStmt, err := st.Prepare(setInstanceData, instanceData{})
	if err != nil {
		return errors.Trace(err)
	}

	setInstanceTags := `
INSERT INTO instance_tag (*)
VALUES ($instanceTag.*)
`
	setInstanceTagStmt, err := st.Prepare(setInstanceTags, instanceTag{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		instanceData := instanceData{
			MachineUUID:          machineUUID,
			InstanceID:           string(instanceID),
			Arch:                 hardwareCharacteristics.Arch,
			Mem:                  hardwareCharacteristics.Mem,
			RootDisk:             hardwareCharacteristics.RootDisk,
			RootDiskSource:       hardwareCharacteristics.RootDiskSource,
			CPUCores:             hardwareCharacteristics.CpuCores,
			CPUPower:             hardwareCharacteristics.CpuPower,
			AvailabilityZoneUUID: hardwareCharacteristics.AvailabilityZone,
			VirtType:             hardwareCharacteristics.VirtType,
		}
		if err := tx.Query(ctx, setInstanceDataStmt, instanceData).Run(); err != nil {
			return errors.Annotatef(domain.CoerceError(err), "inserting machine cloud instance for machine %q", machineUUID)
		}
		instanceTags := tagsFromHardwareCharacteristics(machineUUID, &hardwareCharacteristics)
		if err := tx.Query(ctx, setInstanceTagStmt, instanceTags).Run(); err != nil {
			return errors.Annotatef(domain.CoerceError(err), "inserting instance tags for machine %q", machineUUID)
		}
		return nil
	})
}

// DeleteMachineCloudInstance removes an entry in the machine cloud instance table
// along with the instance tags and the link to a lxd profile if any.
func (st *State) DeleteMachineCloudInstance(
	ctx context.Context,
	machineUUID string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	deleteInstanceData := `
DELETE FROM machine_cloud_instance 
WHERE machine_uuid=$instanceData.machine_uuid
`
	machineUUIDQuery := instanceData{
		MachineUUID: machineUUID,
	}
	deleteInstanceDataStmt, err := st.Prepare(deleteInstanceData, machineUUIDQuery)
	if err != nil {
		return errors.Trace(err)
	}

	deleteInstanceTags := `
DELETE FROM instance_tag 
WHERE machine_uuid=$instanceTag.machine_uuid
`
	machineUUIDQueryTag := instanceTag{
		MachineUUID: machineUUID,
	}
	deleteInstanceTagStmt, err := st.Prepare(deleteInstanceTags, machineUUIDQueryTag)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, deleteInstanceDataStmt, machineUUIDQuery).Run(); err != nil {
			return errors.Annotatef(domain.CoerceError(err), "deleting machine cloud instance for machine %q", machineUUID)
		}
		if err := tx.Query(ctx, deleteInstanceTagStmt, machineUUIDQueryTag).Run(); err != nil {
			return errors.Annotatef(domain.CoerceError(err), "deleting instance tags for machine %q", machineUUID)
		}
		return nil
	})
}
