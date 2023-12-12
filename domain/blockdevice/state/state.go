// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"reflect"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/blockdevice"
)

// State represents database interactions dealing with block devices.
type State struct {
	*domain.StateBase
}

// NewState returns a new block device state
// based on the input database factory method.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// BlockDevices returns the BlockDevices for the specified machine.
func (st *State) BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []blockdevice.BlockDevice
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		result, err = loadBlockDevices(ctx, tx, machineId)
		return errors.Trace(err)
	})
	return result, errors.Trace(err)
}

func loadBlockDevices(ctx context.Context, tx *sqlair.TX, machineId string) ([]blockdevice.BlockDevice, error) {
	credQuery := `
SELECT bd.* AS &BlockDevice.*,
       bdl.* AS &DeviceLink.*,
       fs_type.* AS &FilesystemType.*
FROM   block_device bd
       JOIN block_device_machine bdm ON bd.uuid = bdm.block_device_uuid
       JOIN machine ON bdm.machine_uuid = machine.uuid
       LEFT JOIN block_device_link_device bdl ON bd.uuid = bdl.block_device_uuid
       LEFT JOIN filesystem_type fs_type ON bd.filesystem_type_id = fs_type.id
WHERE  machine.machine_id = $M.machine_id
`

	types := []any{
		BlockDevice{},
		FilesystemType{},
		DeviceLink{},
		sqlair.M{},
	}

	stmt, err := sqlair.Prepare(credQuery, types...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		dbRows            BlockDevices
		dbDeviceLinks     []DeviceLink
		dbFilesystemTypes []FilesystemType
	)
	machineParam := sqlair.M{"machine_id": machineId}
	err = tx.Query(ctx, stmt, machineParam).GetAll(&dbRows, &dbDeviceLinks, &dbFilesystemTypes)
	if err != nil {
		return nil, errors.Annotatef(err, "loading block devices for machine %q", machineId)
	}
	return dbRows.toBlockDevices(dbDeviceLinks, dbFilesystemTypes)
}

// GetMachineInfo is used look up the machine UUID and life for a machine.
func (st *State) GetMachineInfo(ctx context.Context, machineId string) (string, domain.Life, error) {
	db, err := st.DB()
	if err != nil {
		return "", 0, errors.Trace(err)
	}

	var (
		machineUUID string
		life        domain.Life
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		machineUUID, life, err = getMachineInfo(ctx, tx, machineId)
		return errors.Trace(err)
	})
	return machineUUID, life, errors.Trace(err)
}

func getMachineInfo(ctx context.Context, tx *sqlair.TX, machineId string) (string, domain.Life, error) {
	q := `
SELECT machine.life_id AS &M.life_id, machine.uuid AS &M.machine_uuid
FROM   machine
WHERE  machine.machine_id = $M.machine_id
`
	stmt, err := sqlair.Prepare(q, sqlair.M{})
	if err != nil {
		return "", 0, errors.Trace(err)
	}

	result := sqlair.M{}
	err = tx.Query(ctx, stmt, sqlair.M{"machine_id": machineId}).Get(result)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return "", 0, errors.Trace(err)
	}
	if len(result) == 0 {
		return "", 0, fmt.Errorf("machine %q %w", machineId, errors.NotFound)
	}
	life, ok := result["life_id"].(int64)
	if !ok {
		return "", 0, errors.Errorf("missing life value for machine %q", machineId)
	}
	machineUUID := result["machine_uuid"].(string)
	return machineUUID, domain.Life(life), nil
}

// SetMachineBlockDevices sets the block devices visible on the machine.
// Previously recorded block devices not in the list will be removed.
func (st *State) SetMachineBlockDevices(ctx context.Context, machineId string, devices ...blockdevice.BlockDevice) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID, life, err := getMachineInfo(ctx, tx, machineId)
		if err != nil {
			return errors.Trace(err)
		}
		if life == domain.Dead {
			return errors.Errorf("cannot update block devices on dead machine %q", machineId)
		}
		existing, err := loadBlockDevices(ctx, tx, machineId)
		if err != nil {
			return errors.Annotatef(err, "loading block devices for machine %q", machineId)
		}
		if !blockDevicesChanged(existing, devices) {
			return nil
		}

		if err := updateBlockDevices(ctx, tx, machineUUID, devices...); err != nil {
			return errors.Annotatef(err, "updating block devices on machine %q (%s)", machineId, machineUUID)
		}
		return nil
	})

	return errors.Trace(err)
}

func updateBlockDevices(ctx context.Context, tx *sqlair.TX, machineUUID string, devices ...blockdevice.BlockDevice) error {
	fsTypeQuery := `SELECT * AS &FilesystemType.* FROM filesystem_type`
	fsTypeStmt, err := sqlair.Prepare(fsTypeQuery, FilesystemType{})
	if err != nil {
		return errors.Trace(err)
	}
	var fsTypes []FilesystemType
	if err := tx.Query(ctx, fsTypeStmt).GetAll(&fsTypes); err != nil {
		return errors.Trace(err)
	}
	fsTypeByName := make(map[string]int)
	for _, fsType := range fsTypes {
		fsTypeByName[fsType.Name] = fsType.ID
	}

	if err := removeMachineBlockDevices(ctx, tx, machineUUID); err != nil {
		return errors.Annotatef(err, "removing existing block devices for machine %q", machineUUID)
	}

	if len(devices) == 0 {
		return nil
	}

	insertQuery := `
INSERT INTO block_device (uuid, name, label, device_uuid, hardware_id, wwn, bus_address, serial_id, mount_point, size, filesystem_type_id, in_use)
VALUES (
    $BlockDevice.uuid,
    $BlockDevice.name,
    $BlockDevice.label,
    $BlockDevice.device_uuid,
    $BlockDevice.hardware_id,
    $BlockDevice.wwn,
    $BlockDevice.bus_address,
    $BlockDevice.serial_id,
    $BlockDevice.mount_point,
    $BlockDevice.size,
    $BlockDevice.filesystem_type_id,
    $BlockDevice.in_use
)
`
	insertStmt, err := sqlair.Prepare(insertQuery, BlockDevice{})
	if err != nil {
		return errors.Trace(err)
	}

	blockDevicesByUUID := make(map[utils.UUID]blockdevice.BlockDevice, len(devices))
	for _, bd := range devices {
		fsTypeID, ok := fsTypeByName[bd.FilesystemType]
		if !ok {
			return errors.NotValidf("filesystem type %q for block device %q", bd.FilesystemType, bd.DeviceName)
		}
		id, err := utils.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		blockDevicesByUUID[id] = bd
		dbBlockDevice := BlockDevice{
			ID:             id.String(),
			DeviceName:     bd.DeviceName,
			Label:          bd.Label,
			DeviceUUID:     bd.UUID,
			HardwareId:     bd.HardwareId,
			WWN:            bd.WWN,
			BusAddress:     bd.BusAddress,
			SerialId:       bd.SerialId,
			MountPoint:     bd.MountPoint,
			Size:           bd.Size,
			FilesystemType: fsTypeID,
			InUse:          bd.InUse,
		}
		if err := tx.Query(ctx, insertStmt, dbBlockDevice).Run(); err != nil {
			return errors.Trace(err)
		}
	}

	insertLinkQuery := `
INSERT INTO block_device_link_device (block_device_uuid, name)
VALUES (
    $DeviceLink.block_device_uuid,
    $DeviceLink.name
)
`
	insertStmt, err = sqlair.Prepare(insertLinkQuery, DeviceLink{})
	if err != nil {
		return errors.Trace(err)
	}

	for uuid, bd := range blockDevicesByUUID {
		for _, link := range bd.DeviceLinks {
			dbDeviceLink := DeviceLink{
				ParentUUID: uuid.String(),
				Name:       link,
			}
			if err := tx.Query(ctx, insertStmt, dbDeviceLink).Run(); err != nil {
				return errors.Trace(err)
			}
		}
	}

	insertJoinQuery := `
INSERT INTO block_device_machine(block_device_uuid, machine_uuid)
VALUES ($M.block_device_uuid, $M.machine_uuid)
`
	insertJoinStmt, err := sqlair.Prepare(insertJoinQuery, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	for uuid := range blockDevicesByUUID {
		if err := tx.Query(ctx, insertJoinStmt, sqlair.M{
			"machine_uuid":      machineUUID,
			"block_device_uuid": uuid.String(),
		}).Run(); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func blockDevicesChanged(oldDevices, newDevices []blockdevice.BlockDevice) bool {
	if len(oldDevices) != len(newDevices) {
		return true
	}
	for _, o := range oldDevices {
		var found bool
		for _, n := range newDevices {
			if reflect.DeepEqual(o, n) {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// RemoveMachineBlockDevices removes all the block devices for the specified machine.
// It is the same as calling SetMachineBlockDevices with an empty list, but does not
// error if the machine life is Dead.
func (st *State) RemoveMachineBlockDevices(ctx context.Context, machineId string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		q := `
SELECT machine.uuid AS &M.machine_uuid
FROM   machine
WHERE  machine.machine_id = $M.machine_id
`
		stmt, err := sqlair.Prepare(q, sqlair.M{})
		if err != nil {
			return errors.Trace(err)
		}

		result := sqlair.M{}
		err = tx.Query(ctx, stmt, sqlair.M{"machine_id": machineId}).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}
		if len(result) == 0 {
			return fmt.Errorf("machine %q %w", machineId, errors.NotFound)
		}
		machineUUID := result["machine_uuid"].(string)
		if err := removeMachineBlockDevices(ctx, tx, machineUUID); err != nil {
			return errors.Annotatef(err, "removing block devices on machine %q (%s)", machineId, machineUUID)
		}
		return nil
	})

	return errors.Trace(err)
}

func removeMachineBlockDevices(ctx context.Context, tx *sqlair.TX, machineUUID string) error {
	machineUUIDParam := sqlair.M{"machine_uuid": machineUUID}

	// TODO(wallyworld) - sqlair doesn't support IN clauses yet.
	// Ideally, we'd first get the block device UUIDs to delete
	// and use IN clauses with the delete queries
	// This avoids the need for a potentially inefficient
	// select distinct over a (maybe) large number of rows when
	// deleting from block_device. In practice, it would be at
	// most no more than up to 1000 rows in the extreme case.

	linkDeleteQuery := fmt.Sprintf(`
DELETE 
FROM  block_device_link_device
WHERE block_device_uuid IN (
    SELECT DISTINCT block_device_uuid
    FROM            block_device_machine bdm
    WHERE           bdm.machine_uuid = $M.machine_uuid
)`)

	deleteStmt, err := sqlair.Prepare(linkDeleteQuery, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	if err := tx.Query(ctx, deleteStmt, machineUUIDParam).Run(); err != nil {
		return errors.Trace(err)
	}

	deviceMachineDeleteQuery := fmt.Sprintf(`
DELETE
FROM  block_device_machine
WHERE machine_uuid = $M.machine_uuid
`)

	deleteStmt, err = sqlair.Prepare(deviceMachineDeleteQuery, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	if err := tx.Query(ctx, deleteStmt, machineUUIDParam).Run(); err != nil {
		return errors.Trace(err)
	}

	deleteQuery := fmt.Sprintf(`
DELETE
FROM  block_device
WHERE uuid NOT IN (
    SELECT DISTINCT block_device_uuid
    FROM            block_device_machine
)`)

	deleteStmt, err = sqlair.Prepare(deleteQuery)
	if err != nil {
		return errors.Trace(err)
	}
	if err := tx.Query(ctx, deleteStmt).Run(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
