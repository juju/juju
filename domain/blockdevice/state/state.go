// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"reflect"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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
// Returns an error satisfying machinerrors.NotFound if the machine does not exist.
func (st *State) BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []blockdevice.BlockDevice
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		result, err = st.loadBlockDevices(ctx, tx, machineId)
		return errors.Capture(err)
	})
	return result, errors.Capture(err)
}

func (st *State) loadBlockDevices(ctx context.Context, tx *sqlair.TX, machineId string) ([]blockdevice.BlockDevice, error) {
	query := `
SELECT bd.* AS &BlockDevice.*,
       bdl.* AS &DeviceLink.*,
       fs_type.* AS &FilesystemType.*
FROM   block_device bd
       JOIN machine ON bd.machine_uuid = machine.uuid
       LEFT JOIN block_device_link_device bdl ON bd.uuid = bdl.block_device_uuid
       LEFT JOIN filesystem_type fs_type ON bd.filesystem_type_id = fs_type.id
WHERE  machine.name = $M.name
`

	types := []any{
		BlockDevice{},
		FilesystemType{},
		DeviceLink{},
		sqlair.M{},
	}

	stmt, err := st.Prepare(query, types...)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows            BlockDevices
		dbDeviceLinks     []DeviceLink
		dbFilesystemTypes []FilesystemType
	)
	machineParam := sqlair.M{"name": machineId}
	err = tx.Query(ctx, stmt, machineParam).GetAll(&dbRows, &dbDeviceLinks, &dbFilesystemTypes)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf("loading block devices for machine %q: %w", machineId, err)
	}
	result, _, err := dbRows.toBlockDevicesAndMachines(dbDeviceLinks, dbFilesystemTypes, nil)
	return result, errors.Capture(err)
}

func (st *State) getMachineInfo(ctx context.Context, tx *sqlair.TX, machineId string) (string, life.Life, error) {
	q := `
SELECT machine.life_id AS &M.life_id, machine.uuid AS &M.machine_uuid
FROM   machine
WHERE  machine.name = $M.name
`
	stmt, err := st.Prepare(q, sqlair.M{})
	if err != nil {
		return "", 0, errors.Capture(err)
	}

	result := sqlair.M{}
	err = tx.Query(ctx, stmt, sqlair.M{"name": machineId}).Get(result)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return "", 0, errors.Errorf("looking up UUID for machine %q: %w", machineId, err)
	}
	if len(result) == 0 {
		return "", 0, errors.Errorf("machine %q not found", machineId).Add(machineerrors.MachineNotFound)
	}
	machineLife, ok := result["life_id"].(int64)
	if !ok {
		return "", 0, errors.Errorf("missing life value for machine %q", machineId)
	}
	machineUUID := result["machine_uuid"].(string)
	return machineUUID, life.Life(machineLife), nil
}

// SetMachineBlockDevices sets the block devices visible on the machine.
// Previously recorded block devices not in the list will be removed.
// Returns an error satisfying machinerrors.NotFound if the machine does not exist.
func (st *State) SetMachineBlockDevices(ctx context.Context, machineId string, devices ...blockdevice.BlockDevice) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID, machineLife, err := st.getMachineInfo(ctx, tx, machineId)
		if err != nil {
			return errors.Capture(err)
		}
		if machineLife == life.Dead {
			return errors.Errorf("cannot update block devices on dead machine %q", machineId)
		}
		existing, err := st.loadBlockDevices(ctx, tx, machineId)
		if err != nil {
			return errors.Errorf("loading block devices for machine %q: %w", machineId, err)
		}
		if !blockDevicesChanged(existing, devices) {
			return nil
		}

		if err := st.updateBlockDevices(ctx, tx, machineUUID, devices...); err != nil {
			return errors.Errorf("updating block devices on machine %q (%s): %w", machineId, machineUUID, err)
		}
		return nil
	})

	return errors.Capture(err)
}

func (st *State) updateBlockDevices(ctx context.Context, tx *sqlair.TX, machineUUID string, devices ...blockdevice.BlockDevice) error {
	if err := RemoveMachineBlockDevices(ctx, tx, machineUUID); err != nil {
		return errors.Errorf("removing existing block devices for machine %q: %w", machineUUID, err)
	}

	if len(devices) == 0 {
		return nil
	}

	fsTypeQuery := `SELECT * AS &FilesystemType.* FROM filesystem_type`
	fsTypeStmt, err := st.Prepare(fsTypeQuery, FilesystemType{})
	if err != nil {
		return errors.Capture(err)
	}
	var fsTypes []FilesystemType
	if err := tx.Query(ctx, fsTypeStmt).GetAll(&fsTypes); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Capture(err)
	}
	fsTypeByName := make(map[string]int)
	for _, fsType := range fsTypes {
		fsTypeByName[fsType.Name] = fsType.ID
	}

	insertQuery := `INSERT INTO block_device (*) VALUES ($BlockDevice.*)`
	insertStmt, err := st.Prepare(insertQuery, BlockDevice{})
	if err != nil {
		return errors.Capture(err)
	}

	insertLinkQuery := `
INSERT INTO block_device_link_device (block_device_uuid, name)
VALUES (
    $DeviceLink.block_device_uuid,
    $DeviceLink.name
)
`
	insertLinkStmt, err := st.Prepare(insertLinkQuery, DeviceLink{})
	if err != nil {
		return errors.Capture(err)
	}

	for _, bd := range devices {
		fsTypeID, ok := fsTypeByName[bd.FilesystemType]
		if !ok {
			return errors.Errorf("filesystem type %q for block device %q %w", bd.FilesystemType, bd.DeviceName, coreerrors.NotValid)
		}
		id, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		dbBlockDevice := BlockDevice{
			ID:             id.String(),
			MachineUUID:    machineUUID,
			DeviceName:     bd.DeviceName,
			Label:          bd.Label,
			DeviceUUID:     bd.UUID,
			HardwareId:     bd.HardwareId,
			WWN:            bd.WWN,
			BusAddress:     bd.BusAddress,
			SerialId:       bd.SerialId,
			MountPoint:     bd.MountPoint,
			SizeMiB:        bd.SizeMiB,
			FilesystemType: fsTypeID,
			InUse:          bd.InUse,
		}
		if err := tx.Query(ctx, insertStmt, dbBlockDevice).Run(); err != nil {
			return errors.Errorf("inserting block devices: %w", err)
		}

		for _, link := range bd.DeviceLinks {
			dbDeviceLink := DeviceLink{
				ParentUUID: id.String(),
				Name:       link,
			}
			if err := tx.Query(ctx, insertLinkStmt, dbDeviceLink).Run(); err != nil {
				return errors.Errorf("inserting block device links: %w", err)
			}
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

// MachineBlockDevices retrieves block devices for all machines.
func (st *State) MachineBlockDevices(ctx context.Context) ([]blockdevice.MachineBlockDevice, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT bd.* AS &BlockDevice.*,
       bdl.* AS &DeviceLink.*,
       fs_type.* AS &FilesystemType.*,
       machine.name AS &BlockDeviceMachine.*
FROM   block_device bd
       JOIN machine ON bd.machine_uuid = machine.uuid
       LEFT JOIN block_device_link_device bdl ON bd.uuid = bdl.block_device_uuid
       LEFT JOIN filesystem_type fs_type ON bd.filesystem_type_id = fs_type.id
`

	types := []any{
		BlockDevice{},
		FilesystemType{},
		DeviceLink{},
		BlockDeviceMachine{},
	}

	stmt, err := st.Prepare(query, types...)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		blockDevices []blockdevice.BlockDevice
		machines     []string
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var (
			dbRows            BlockDevices
			dbDeviceLinks     []DeviceLink
			dbFilesystemTypes []FilesystemType
			dbMachines        []BlockDeviceMachine
		)
		if err := tx.Query(ctx, stmt).GetAll(&dbRows, &dbDeviceLinks, &dbFilesystemTypes, &dbMachines); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("loading block devices: %w", err)
		}
		blockDevices, machines, err = dbRows.toBlockDevicesAndMachines(dbDeviceLinks, dbFilesystemTypes, dbMachines)
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make([]blockdevice.MachineBlockDevice, len(blockDevices))
	for i, bd := range blockDevices {
		result[i] = blockdevice.MachineBlockDevice{
			MachineId:   machines[i],
			BlockDevice: bd,
		}
	}
	return result, nil
}

// RemoveMachineBlockDevices deletes all the block devices belonging to the specified machine.
// Exported so that it can be called from [domain.machine.state.DeleteMachine].
func RemoveMachineBlockDevices(ctx context.Context, tx *sqlair.TX, machineUUID string) error {
	machineUUIDParam := sqlair.M{"machine_uuid": machineUUID}

	// TODO(wallyworld) - sqlair doesn't support IN clauses yet.
	// Ideally, we'd first get the block device UUIDs to delete
	// and use IN clauses with the delete queries
	// This avoids the need for a potentially inefficient
	// select distinct over a (maybe) large number of rows when
	// deleting from block_device. In practice, it would be at
	// most no more than up to 1000 rows in the extreme case.

	linkDeleteQuery := `
DELETE 
FROM  block_device_link_device
WHERE block_device_uuid IN (
    SELECT DISTINCT uuid
    FROM            block_device bd
    WHERE           bd.machine_uuid = $M.machine_uuid
)`

	deleteStmt, err := sqlair.Prepare(linkDeleteQuery, sqlair.M{})
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteStmt, machineUUIDParam).Run(); err != nil {
		return errors.Errorf("deleting block device link devices: %w", err)
	}

	deleteQuery := `
DELETE
FROM  block_device
WHERE machine_uuid = $M.machine_uuid
`

	deleteStmt, err = sqlair.Prepare(deleteQuery, sqlair.M{})
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteStmt, machineUUIDParam).Run(); err != nil {
		return errors.Errorf("deleting block devices: %w", err)
	}
	return nil
}

// WatchBlockDevices returns a new NotifyWatcher watching for
// changes to block devices associated with the specified machine.
func (st *State) WatchBlockDevices(
	ctx context.Context,
	getWatcher func(
		ctx context.Context,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error),
	machineId string,
) (watcher.NotifyWatcher, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		machineUUID string
		machineLife life.Life
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID, machineLife, err = st.getMachineInfo(ctx, tx, machineId)
		return errors.Capture(err)
	})

	if err != nil {
		return nil, errors.Capture(err)
	}
	if machineLife == life.Dead {
		return nil, errors.Errorf("cannot watch block devices on dead machine %q", machineId)
	}

	baseWatcher, err := getWatcher(
		ctx,
		eventsource.PredicateFilter("block_device", changestream.All, eventsource.EqualsPredicate(machineUUID)),
	)
	if err != nil {
		return nil, errors.Errorf("watching machine %q block devices: %w", machineId, err)
	}
	return baseWatcher, nil
}
