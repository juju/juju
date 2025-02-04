// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/database"
	internalerrors "github.com/juju/juju/internal/errors"
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
	query := `
SELECT    &instanceDataResult.*
FROM      v_hardware_characteristics AS v
WHERE     v.machine_uuid = $instanceDataResult.machine_uuid`
	machineUUIDQuery := instanceDataResult{
		MachineUUID: machineUUID,
	}
	stmt, err := st.Prepare(query, machineUUIDQuery)
	if err != nil {
		return nil, errors.Annotate(err, "preparing retrieve hardware characteristics statement")
	}

	var row instanceDataResult
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, machineUUIDQuery).Get(&row)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Annotatef(machineerrors.NotProvisioned, "machine: %q", machineUUID)
		}
		return errors.Annotatef(err, "querying machine cloud instance for machine %q", machineUUID)
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return row.toHardwareCharacteristics(), nil
}

// AvailabilityZone returns the availability zone for the specified machine.
// If no hardware characteristics are set for the machine, it returns
// [machineerrors.AvailabilityZoneNotFound].
func (st *State) AvailabilityZone(
	ctx context.Context,
	machineUUID string,
) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	query := `
SELECT    &instanceDataResult.availability_zone_name
FROM      v_hardware_characteristics AS v
WHERE     v.machine_uuid = $instanceDataResult.machine_uuid`
	machineUUIDQuery := instanceDataResult{
		MachineUUID: machineUUID,
	}
	stmt, err := st.Prepare(query, machineUUIDQuery)
	if err != nil {
		return "", errors.Annotate(err, "preparing retrieve hardware characteristics statement")
	}

	var row instanceDataResult
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, machineUUIDQuery).Get(&row)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Annotatef(machineerrors.AvailabilityZoneNotFound, "machine cloud instance for machine %q", machineUUID)
		}
		return errors.Annotatef(err, "querying machine cloud instance for machine %q", machineUUID)
	}); err != nil {
		return "", errors.Trace(err)
	}
	if row.AvailabilityZone == nil {
		return "", nil
	}
	return *row.AvailabilityZone, nil
}

// SetMachineCloudInstance sets an entry in the machine cloud instance table
// along with the instance tags and the link to a lxd profile if any.
func (st *State) SetMachineCloudInstance(
	ctx context.Context,
	machineUUID string,
	instanceID instance.Id,
	displayName string,
	hardwareCharacteristics *instance.HardwareCharacteristics,
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

	azName := availabilityZoneName{}
	if hardwareCharacteristics != nil && hardwareCharacteristics.AvailabilityZone != nil {
		az := *hardwareCharacteristics.AvailabilityZone
		azName = availabilityZoneName{Name: az}
	}
	retrieveAZUUID := `
SELECT &availabilityZoneName.uuid
FROM   availability_zone
WHERE  availability_zone.name = $availabilityZoneName.name
`
	retrieveAZUUIDStmt, err := st.Prepare(retrieveAZUUID, azName)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		instanceData := instanceData{
			MachineUUID: machineUUID,
			InstanceID:  instanceID.String(),
			DisplayName: displayName,
		}
		if hardwareCharacteristics != nil {
			instanceData.Arch = hardwareCharacteristics.Arch
			instanceData.Mem = hardwareCharacteristics.Mem
			instanceData.RootDisk = hardwareCharacteristics.RootDisk
			instanceData.RootDiskSource = hardwareCharacteristics.RootDiskSource
			instanceData.CPUCores = hardwareCharacteristics.CpuCores
			instanceData.CPUPower = hardwareCharacteristics.CpuPower
			instanceData.VirtType = hardwareCharacteristics.VirtType
			if hardwareCharacteristics.AvailabilityZone != nil && *hardwareCharacteristics.AvailabilityZone != "" {
				azUUID := availabilityZoneName{}
				if err := tx.Query(ctx, retrieveAZUUIDStmt, azName).Get(&azUUID); err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return internalerrors.Errorf("%w %q for machine %q", networkerrors.AvailabilityZoneNotFound, *hardwareCharacteristics.AvailabilityZone, machineUUID)
					}
					return internalerrors.Errorf("cannot retrieve availability zone %q for machine uuid %q: %w", *hardwareCharacteristics.AvailabilityZone, machineUUID, err)
				}
				instanceData.AvailabilityZoneUUID = &azUUID.UUID
			}
		}
		if err := tx.Query(ctx, setInstanceDataStmt, instanceData).Run(); err != nil {
			if database.IsErrConstraintPrimaryKey(err) {
				return internalerrors.Errorf("%w for machine %q", machineerrors.MachineCloudInstanceAlreadyExists, machineUUID)
			}
			return errors.Annotatef(err, "inserting machine cloud instance for machine %q", machineUUID)
		}
		if instanceTags := tagsFromHardwareCharacteristics(machineUUID, hardwareCharacteristics); len(instanceTags) > 0 {
			if err := tx.Query(ctx, setInstanceTagStmt, instanceTags).Run(); err != nil {
				return errors.Annotatef(err, "inserting instance tags for machine %q", machineUUID)
			}
		}
		return nil
	})
}

// DeleteMachineCloudInstance removes an entry in the machine cloud instance
// table along with the instance tags and the link to a lxd profile if any, as
// well as any associated status data.
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

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Delete the machine cloud instance status. No need to return error if
		// no status is set for the instance while deleting.
		if err := tx.Query(ctx, deleteInstanceStatusStmt, machineUUIDParam).Run(); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(domain.CoerceError(err), "deleting machine cloud instance status for machine %q", mUUID)
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

// InstanceID returns the cloud specific instance id for this machine.
// If the machine is not provisioned, it returns a
// [machineerrors.NotProvisionedError].
func (st *State) InstanceID(ctx context.Context, mUUID string) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	mUUIDParam := machineUUID{UUID: mUUID}
	query := `
SELECT &instanceID.instance_id
FROM   machine_cloud_instance
WHERE  machine_uuid = $machineUUID.uuid;`
	queryStmt, err := st.Prepare(query, mUUIDParam, instanceID{})
	if err != nil {
		return "", errors.Trace(err)
	}

	var instanceId string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result instanceID
		err := tx.Query(ctx, queryStmt, mUUIDParam).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(machineerrors.NotProvisioned, "machine: %q", mUUID)
			}
			return errors.Annotatef(err, "querying instance for machine %q", mUUID)
		}

		instanceId = result.ID
		return nil
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return instanceId, nil
}

// InstanceIDAndName returns the cloud specific instance ID and display name for
// this machine.
// If the machine is not provisioned, it returns a
// [machineerrors.NotProvisionedError].
func (st *State) InstanceIDAndName(ctx context.Context, mUUID string) (string, string, error) {
	db, err := st.DB()
	if err != nil {
		return "", "", errors.Trace(err)
	}

	mUUIDParam := machineUUID{UUID: mUUID}
	query := `
SELECT &instanceIDAndDisplayName.*
FROM   machine_cloud_instance
WHERE  machine_uuid = $machineUUID.uuid;`
	queryStmt, err := st.Prepare(query, mUUIDParam, instanceIDAndDisplayName{})
	if err != nil {
		return "", "", errors.Trace(err)
	}

	var (
		instanceID, instanceName string
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result instanceIDAndDisplayName
		err := tx.Query(ctx, queryStmt, mUUIDParam).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(machineerrors.NotProvisioned, "machine: %q", mUUID)
			}
			return errors.Annotatef(err, "querying display name for machine %q", mUUID)
		}

		instanceID = result.ID
		instanceName = result.Name
		return nil
	})
	if err != nil {
		return "", "", errors.Trace(err)
	}
	return instanceID, instanceName, nil
}

// GetInstanceStatus returns the cloud specific instance status for the given
// machine.
// It returns NotFound if the machine does not exist.
// It returns a StatusNotSet if the instance status is not set.
// Idempotent.
func (st *State) GetInstanceStatus(ctx context.Context, mName machine.Name) (domainmachine.StatusInfo, error) {
	db, err := st.DB()
	if err != nil {
		return domainmachine.StatusInfo{}, errors.Trace(err)
	}

	nameIdent := machineName{Name: mName}

	var uuid machineUUID
	uuidQuery := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name`
	uuidQueryStmt, err := st.Prepare(uuidQuery, nameIdent, uuid)
	if err != nil {
		return domainmachine.StatusInfo{}, errors.Trace(err)
	}

	var status machineStatus
	statusCombinedQuery := `
SELECT &machineStatus.*
FROM v_machine_cloud_instance_status AS st
WHERE st.machine_uuid = $machineUUID.uuid`
	statusCombinedQueryStmt, err := st.Prepare(statusCombinedQuery, uuid, status)
	if err != nil {
		return domainmachine.StatusInfo{}, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid
		err := tx.Query(ctx, uuidQueryStmt, nameIdent).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		} else if err != nil {
			return errors.Annotatef(err, "querying uuid for machine %q", mName)
		}

		// Query for the machine cloud instance status and status data combined
		err = tx.Query(ctx, statusCombinedQueryStmt, uuid).Get(&status)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(machineerrors.StatusNotSet, "machine: %q", mName)
		} else if err != nil {
			return errors.Annotatef(err, "querying cloud instance status and status data for machine %q", mName)
		}

		return nil
	})
	if err != nil {
		return domainmachine.StatusInfo{}, errors.Trace(err)
	}

	// Convert the internal status id from the
	// (machine_cloud_instance_status_value table) into the core status.Status
	// type.
	machineStatus, err := decodeCloudInstanceStatus(status.Status)
	if err != nil {
		return domainmachine.StatusInfo{}, errors.Annotatef(err, "decoding cloud instance status for machine %q", mName)
	}

	var since time.Time
	if status.Updated.Valid {
		since = status.Updated.Time
	} else {
		since = st.clock.Now()
	}

	return domainmachine.StatusInfo{
		Status:  machineStatus,
		Message: status.Message,
		Since:   &since,
		Data:    status.Data,
	}, nil
}

// SetInstanceStatus sets the cloud specific instance status for this
// machine.
// It returns [machineerrors.MachineNotFound] if the machine does not exist.
func (st *State) SetInstanceStatus(ctx context.Context, mName machine.Name, newStatus domainmachine.StatusInfo) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	statusID, err := encodeCloudInstanceStatus(newStatus.Status)
	if err != nil {
		return errors.Trace(err)
	}

	status := setMachineStatus{
		StatusID: statusID,
		Message:  newStatus.Message,
		Data:     newStatus.Data,
		Updated:  newStatus.Since,
	}

	// Prepare query for machine uuid
	nameIdent := machineName{Name: mName}

	var mUUID machineUUID
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name;`
	queryMachineStmt, err := st.Prepare(queryMachine, nameIdent, mUUID)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for setting the machine cloud instance status
	statusQuery := `
INSERT INTO machine_cloud_instance_status (*)
VALUES ($setMachineStatus.*)
  ON CONFLICT (machine_uuid)
  DO UPDATE SET 
  	status_id = excluded.status_id, 
	message = excluded.message, 
	updated_at = excluded.updated_at,
	data = excluded.data;
`
	statusQueryStmt, err := st.Prepare(statusQuery, status)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid
		err := tx.Query(ctx, queryMachineStmt, nameIdent).Get(&mUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		} else if err != nil {
			return errors.Annotatef(err, "querying uuid for machine %q", mName)
		}

		status.MachineUUID = mUUID.UUID

		// Query for setting the machine cloud instance status
		err = tx.Query(ctx, statusQueryStmt, status).Run()
		if err != nil {
			return errors.Annotatef(err, "setting machine status for machine %q", mName)
		}
		return nil
	})
}
