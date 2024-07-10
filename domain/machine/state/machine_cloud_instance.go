// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain"
	machineerrors "github.com/juju/juju/domain/machine/errors"
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
		if instanceTags := tagsFromHardwareCharacteristics(machineUUID, &hardwareCharacteristics); len(instanceTags) > 0 {
			if err := tx.Query(ctx, setInstanceTagStmt, instanceTags).Run(); err != nil {
				return errors.Annotatef(domain.CoerceError(err), "inserting instance tags for machine %q", machineUUID)
			}
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

// InstanceId returns the cloud specific instance id for this machine.
// If the machine is not provisioned, it returns a NotProvisionedError.
func (st *State) InstanceId(ctx context.Context, mName machine.Name) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	machineIDParam := machineName{Name: mName}
	query := `
SELECT instance_id AS &instanceID.*
FROM machine AS m
    JOIN machine_cloud_instance AS mci ON m.uuid = mci.machine_uuid
WHERE m.name = $machineName.name;
`
	queryStmt, err := st.Prepare(query, machineIDParam, instanceID{})
	if err != nil {
		return "", errors.Trace(err)
	}

	var instanceId string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := instanceID{}
		err := tx.Query(ctx, queryStmt, machineIDParam).Get(&result)
		if err != nil {
			return errors.Annotatef(err, "querying instance for machine %q", mName)
		}

		instanceId = result.ID
		return nil
	})
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", errors.Annotatef(machineerrors.NotProvisioned, "machine: %q", mName)
		}
		return "", errors.Trace(err)
	}
	return instanceId, nil
}

// GetInstanceStatus returns the cloud specific instance status for the given
// machine.
// It returns a StatusNotSet if the instance status is not set.
// Idempotent.
func (st *State) GetInstanceStatus(ctx context.Context, mName machine.Name) (status.Status, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}
	machineStatus := machineInstanceStatus{Name: mName}
	statusQuery := `
SELECT mcis.status as &machineInstanceStatus.status
FROM machine as m
	JOIN machine_cloud_instance_status as mcis ON m.uuid = mcis.machine_uuid
WHERE m.name = $machineInstanceStatus.name;
`

	statusQueryStmt, err := st.Prepare(statusQuery, machineStatus)
	if err != nil {
		return "", errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, statusQueryStmt, machineStatus).Get(&machineStatus)
		if err != nil {
			return errors.Annotatef(err, "querying cloud instance status for machine %q", mName)
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", errors.Annotatef(machineerrors.StatusNotSet, "machine: %q", mName)
		}
		return "", errors.Trace(err)
	}
	internalStatus := machineStatus.Status
	// Convert the internal status id from the (instance_status_values table)
	// into the core status.Status type.
	var instanceStatus status.Status
	switch internalStatus {
	case 0:
		instanceStatus = status.Empty
	case 1:
		instanceStatus = status.Allocating
	case 2:
		instanceStatus = status.Running
	case 3:
		instanceStatus = status.ProvisioningError
	}
	return instanceStatus, nil
}

// InitialWatchInstanceStatement returns the table and the initial watch statement
// for the machine cloud instances.
func (s *State) InitialWatchInstanceStatement() (string, string) {
	return "machine_cloud_instance", "SELECT machine_uuid FROM machine_cloud_instance"
}
