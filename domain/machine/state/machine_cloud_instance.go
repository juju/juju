// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
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

// DeleteMachineCloudInstance removes an entry in the machine cloud instance
// table along with the instance tags and the link to a lxd profile if any.
func (st *State) DeleteMachineCloudInstance(
	ctx context.Context,
	mUUID string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for deleting machine cloud instance.
	deleteInstanceQuery := `
DELETE FROM machine_cloud_instance 
WHERE machine_uuid=$machineUUID.uuid
`
	machineUUIDParam := machineUUID{
		UUID: mUUID,
	}
	deleteInstanceStmt, err := st.Prepare(deleteInstanceQuery, machineUUIDParam)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for deleting instance tags.
	deleteInstanceTagsQuery := `
DELETE FROM instance_tag 
WHERE machine_uuid=$machineUUID.uuid
`
	deleteInstanceTagStmt, err := st.Prepare(deleteInstanceTagsQuery, machineUUIDParam)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for deleting cloud instance status.
	deleteInstanceStatusQuery := `DELETE FROM machine_cloud_instance_status WHERE machine_uuid=$machineUUID.uuid`
	deleteInstanceStatusStmt, err := st.Prepare(deleteInstanceStatusQuery, machineUUIDParam)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for deleting cloud instance status data.
	deleteInstanceStatusDataQuery := `DELETE FROM machine_cloud_instance_status_data WHERE machine_uuid=$machineUUID.uuid`
	deleteInstanceStatusDataStmt, err := st.Prepare(deleteInstanceStatusDataQuery, machineUUIDParam)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Delete the machine cloud instance status. No need to return error if
		// no status is set for the instance while deleting.
		if err := tx.Query(ctx, deleteInstanceStatusStmt, machineUUIDParam).Run(); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(domain.CoerceError(err), "deleting machine cloud instance status for machine %q", mUUID)
		}

		// Delete the machine cloud instance status data. No need to return
		// error if no status data is set for the instance while deleting.
		if err := tx.Query(ctx, deleteInstanceStatusDataStmt, machineUUIDParam).Run(); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(domain.CoerceError(err), "deleting machine cloud instance status data for machine %q", mUUID)
		}

		// Delete the machine cloud instance.
		if err := tx.Query(ctx, deleteInstanceStmt, machineUUIDParam).Run(); err != nil {
			return errors.Annotatef(domain.CoerceError(err), "deleting machine cloud instance for machine %q", mUUID)
		}

		// Delete the machine cloud instance tags.
		if err := tx.Query(ctx, deleteInstanceTagStmt, machineUUIDParam).Run(); err != nil {
			return errors.Annotatef(domain.CoerceError(err), "deleting instance tags for machine %q", mUUID)
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
// It returns NotFound if the machine does not exist.
// It returns a StatusNotSet if the instance status is not set.
// Idempotent.
func (st *State) GetInstanceStatus(ctx context.Context, mName machine.Name) (status.StatusInfo, error) {
	db, err := st.DB()
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}

	// Prepare query for machine uuid (to be used in
	// machine_cloud_instance_status and machine_cloud_instance_status_data
	// tables)
	machineNameParam := machineName{Name: mName}
	machineUUID := machineUUID{}
	uuidQuery := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name`
	uuidQueryStmt, err := st.Prepare(uuidQuery, machineNameParam, machineUUID)
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}

	// Prepare query for machine cloud instance status
	instanceStatusParam := machineInstanceStatus{}
	statusQuery := `SELECT &machineInstanceStatus.* FROM machine_cloud_instance_status WHERE machine_uuid = $machineUUID.uuid`
	statusQueryStmt, err := st.Prepare(statusQuery, machineUUID, instanceStatusParam)
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}

	// Prepare query for machine cloud instance status data
	statusDataQuery := `SELECT &machineInstanceStatusData.* FROM machine_cloud_instance_status_data WHERE machine_uuid = $machineUUID.uuid`
	statusDataQueryStmt, err := st.Prepare(statusDataQuery, machineUUID, machineInstanceStatusData{})
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}

	var instanceStatusData []machineInstanceStatusData
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid
		err := tx.Query(ctx, uuidQueryStmt, machineNameParam).Get(&machineUUID)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.NotFoundf("machine %q", mName)
			}
			return errors.Annotatef(err, "querying uuid for machine %q", mName)
		}

		// Query for the machine cloud instance status
		err = tx.Query(ctx, statusQueryStmt, machineUUID).Get(&instanceStatusParam)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(machineerrors.StatusNotSet, "machine: %q", mName)
			}
			return errors.Annotatef(err, "querying machine status for machine %q", mName)
		}

		// Query for the machine cloud instance status data, no need to return
		// error if we don't have any status data.
		err = tx.Query(ctx, statusDataQueryStmt, machineUUID).GetAll(&instanceStatusData)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "querying machine status data for machine %q", mName)
		}

		return nil
	})

	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}

	// Transform the status data slice into a status.Data map.
	statusDataResult := transform.SliceToMap(instanceStatusData, func(d machineInstanceStatusData) (string, interface{}) {
		return d.Key, d.Data
	})

	instanceStatus := status.StatusInfo{
		Message: instanceStatusParam.Message,
		Since:   instanceStatusParam.Updated,
		Data:    statusDataResult,
	}

	// Convert the internal status id from the (instance_status_values table)
	// into the core status.Status type.
	instanceStatus.Status = instanceStatusParam.toCoreInstanceStatusValue()

	return instanceStatus, nil
}

// SetInstanceStatus sets the cloud specific instance status for this
// machine.
// It returns NotFound if the machine does not exist.
func (st *State) SetInstanceStatus(ctx context.Context, mName machine.Name, newStatus status.StatusInfo) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare the new status to be set.
	instanceStatus := machineInstanceStatus{}

	instanceStatus.Status = fromCoreInstanceStatusValue(newStatus.Status)
	instanceStatus.Message = newStatus.Message
	instanceStatus.Updated = newStatus.Since
	instanceStatusData := transform.MapToSlice(newStatus.Data, func(key string, value interface{}) []machineInstanceStatusData {
		return []machineInstanceStatusData{{Key: key, Data: value.(string)}}
	})

	// Prepare query for machine uuid
	machineNameParam := machineName{Name: mName}
	mUUID := machineUUID{}
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name`
	queryMachineStmt, err := st.Prepare(queryMachine, machineNameParam, mUUID)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for setting the machine cloud instance status
	statusQuery := `
INSERT INTO machine_cloud_instance_status (machine_uuid, status, message, updated_at)
VALUES ($machineUUID.uuid, $machineInstanceStatus.status, $machineInstanceStatus.message, $machineInstanceStatus.updated_at)
  ON CONFLICT (machine_uuid)
  DO UPDATE SET status = excluded.status, message = excluded.message, updated_at = excluded.updated_at
`
	statusQueryStmt, err := st.Prepare(statusQuery, mUUID, instanceStatus)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for setting the machine cloud instance status data
	statusDataQuery := `
INSERT INTO machine_cloud_instance_status_data (machine_uuid, "key", data)
VALUES ($machineUUID.uuid, $machineInstanceStatusData.key, $machineInstanceStatusData.data)
  ON CONFLICT (machine_uuid, "key") DO UPDATE SET data = excluded.data
`
	statusDataQueryStmt, err := st.Prepare(statusDataQuery, mUUID, machineInstanceStatusData{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid
		err := tx.Query(ctx, queryMachineStmt, machineNameParam).Get(&mUUID)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.NotFoundf("machine %q", mName)
			}
			return errors.Annotatef(err, "querying uuid for machine %q", mName)
		}

		// Query for setting the machine cloud instance status
		err = tx.Query(ctx, statusQueryStmt, mUUID, instanceStatus).Run()
		if err != nil {
			return errors.Annotatef(err, "setting machine status for machine %q", mName)
		}

		// Query for setting the machine cloud instance status data if
		// instanceStatusData is not empty.
		if len(instanceStatusData) > 0 {
			err = tx.Query(ctx, statusDataQueryStmt, mUUID, instanceStatusData).Run()
			if err != nil {
				return errors.Annotatef(err, "setting machine status data for machine %q", mName)
			}
		}
		return nil
	})
}

// InitialWatchInstanceStatement returns the table and the initial watch statement
// for the machine cloud instances.
func (s *State) InitialWatchInstanceStatement() (string, string) {
	return "machine_cloud_instance", "SELECT machine_uuid FROM machine_cloud_instance"
}
