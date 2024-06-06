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
// data retrieved from the machine_cloud_instance table.
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
	machineUUIDQuery := instanceData{MachineUUID: machineUUID}
	retrieveHardwareCharacteristicsStmt, err := st.Prepare(retrieveHardwareCharacteristics, machineUUIDQuery, sqlair.M{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing retrieve hardware characteristics statement")
	}

	var row instanceData
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, retrieveHardwareCharacteristicsStmt, machineUUIDQuery).Get(&row))
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.Annotatef(errors.NotFound, "instance data for machine %q", machineUUID)
		}
		return nil, errors.Annotatef(domain.CoerceError(err), "querying instance data for machine %q", machineUUID)
	}
	return row.toHardwareCharacteristics(), nil
}

// SetInstanceData sets an entry in the instance data table along with
// the instance tags and the link to a lxd profile if any.
func (st *State) SetInstanceData(
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
			return errors.Annotatef(domain.CoerceError(err), "inserting instance data for machine %q", machineUUID)
		}
		for _, tag := range *hardwareCharacteristics.Tags {
			instanceTag := instanceTag{
				MachineUUID: machineUUID,
				Tag:         tag,
			}
			if err := tx.Query(ctx, setInstanceTagStmt, instanceTag).Run(); err != nil {
				return errors.Annotatef(domain.CoerceError(err), "inserting instance tag %q for machine %q", tag, machineUUID)
			}
		}
		return nil
	})
}

// DeleteInstanceData removes an entry in the instance data table along with
// the instance tags and the link to a lxd profile if any.
func (st *State) DeleteInstanceData(
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
	deleteInstanceDataStmt, err := st.Prepare(deleteInstanceData, instanceData{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteInstanceTags := `
DELETE FROM instance_tag 
WHERE machine_uuid=$instanceTag.machine_uuid
`
	deleteInstanceTagStmt, err := st.Prepare(deleteInstanceTags, instanceTag{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		instanceData := instanceData{
			MachineUUID: machineUUID,
		}
		if err := tx.Query(ctx, deleteInstanceDataStmt, instanceData).Run(); err != nil {
			return errors.Annotatef(domain.CoerceError(err), "deleting instance data for machine %q", machineUUID)
		}
		instanceTag := instanceTag{
			MachineUUID: machineUUID,
		}
		if err := tx.Query(ctx, deleteInstanceTagStmt, instanceTag).Run(); err != nil {
			return errors.Annotatef(domain.CoerceError(err), "deleting instance tags for machine %q", machineUUID)
		}
		return nil
	})
}
