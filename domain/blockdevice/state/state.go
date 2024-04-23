// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"reflect"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	. "github.com/juju/juju/domain/query"
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
		return nil, errors.Trace(err)
	}

	var result []blockdevice.BlockDevice
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		result, err = st.loadBlockDevices(ctx, tx, machineId)
		return errors.Trace(err)
	})
	return result, errors.Trace(err)
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
WHERE  machine.machine_id = $M.machine_id
`

	types := []any{
		BlockDevice{},
		FilesystemType{},
		DeviceLink{},
		sqlair.M{},
	}

	stmt, err := st.Prepare(query, types...)
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
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Annotatef(err, "loading block devices for machine %q", machineId)
	}
	result, _, err := dbRows.toBlockDevicesAndMachines(dbDeviceLinks, dbFilesystemTypes, nil)
	return result, errors.Trace(err)
}

func (st *State) getMachineInfo(ctx context.Context, tx *sqlair.TX, machineId string) (string, life.Life, error) {
	q := `
SELECT machine.life_id AS &M.life_id, machine.uuid AS &M.machine_uuid
FROM   machine
WHERE  machine.machine_id = $M.machine_id
`
	stmt, err := st.Prepare(q, sqlair.M{})
	if err != nil {
		return "", 0, errors.Trace(err)
	}

	result := sqlair.M{}
	err = tx.Query(ctx, stmt, sqlair.M{"machine_id": machineId}).Get(result)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return "", 0, errors.Annotatef(err, "looking up UUID for machine %q", machineId)
	}
	if len(result) == 0 {
		return "", 0, fmt.Errorf("machine %q not found%w", machineId, errors.Hide(machineerrors.NotFound))
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
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID, machineLife, err := st.getMachineInfo(ctx, tx, machineId)
		if err != nil {
			return errors.Trace(err)
		}
		if machineLife == life.Dead {
			return errors.Errorf("cannot update block devices on dead machine %q", machineId)
		}
		existing, err := st.loadBlockDevices(ctx, tx, machineId)
		if err != nil {
			return errors.Annotatef(err, "loading block devices for machine %q", machineId)
		}
		if !blockDevicesChanged(existing, devices) {
			return nil
		}

		if err := st.updateBlockDevices(ctx, tx, machineUUID, devices...); err != nil {
			return errors.Annotatef(err, "updating block devices on machine %q (%s)", machineId, machineUUID)
		}
		return nil
	})

	return errors.Trace(err)
}

func (st *State) updateBlockDevices(ctx context.Context, tx *sqlair.TX, machineUUID string, devices ...blockdevice.BlockDevice) error {
	if err := RemoveMachineBlockDevices(ctx, tx, machineUUID); err != nil {
		return errors.Annotatef(err, "removing existing block devices for machine %q", machineUUID)
	}

	if len(devices) == 0 {
		return nil
	}

	var fsTypes []FilesystemType
	err := st.Query(ctx, tx, "SELECT * AS &FilesystemType.* FROM filesystem_type", OutM(&fsTypes))
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Trace(err)
	}
	fsTypeByName := make(map[string]int)
	for _, fsType := range fsTypes {
		fsTypeByName[fsType.Name] = fsType.ID
	}

	insertDML := "INSERT INTO block_device (*) VALUES ($BlockDevice.*)"
	insertLinkDML := "INSERT INTO block_device_link_device (*) VALUES ($DeviceLink.*)"

	blockDevicesByUUID := make(map[uuid.UUID]blockdevice.BlockDevice, len(devices))
	for _, bd := range devices {
		fsTypeID, ok := fsTypeByName[bd.FilesystemType]
		if !ok {
			return errors.NotValidf("filesystem type %q for block device %q", bd.FilesystemType, bd.DeviceName)
		}
		id, err := uuid.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		blockDevicesByUUID[id] = bd
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
		if _, err := st.Exec(ctx, tx, insertDML, In(dbBlockDevice)); err != nil {
			return errors.Annotate(err, "inserting block devices")
		}

		for _, link := range bd.DeviceLinks {
			dbDeviceLink := DeviceLink{
				ParentUUID: id.String(),
				Name:       link,
			}
			if _, err := st.Exec(ctx, tx, insertLinkDML, In(dbDeviceLink)); err != nil {
				return errors.Annotate(err, "inserting block device links")
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
		return nil, errors.Trace(err)
	}

	query := `
SELECT bd.* AS &BlockDevice.*,
       bdl.* AS &DeviceLink.*,
       fs_type.* AS &FilesystemType.*,
       machine.machine_id AS &BlockDeviceMachine.*
FROM   block_device bd
       JOIN machine ON bd.machine_uuid = machine.uuid
       LEFT JOIN block_device_link_device bdl ON bd.uuid = bdl.block_device_uuid
       LEFT JOIN filesystem_type fs_type ON bd.filesystem_type_id = fs_type.id`

	var (
		dbRows            []BlockDevice
		dbDeviceLinks     []DeviceLink
		dbFilesystemTypes []FilesystemType
		dbMachines        []BlockDeviceMachine
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.Query(ctx, tx, query,
			OutM(&dbRows),
			OutM(&dbDeviceLinks),
			OutM(&dbFilesystemTypes),
			OutM(&dbMachines))
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotate(err, "loading block devices")
			}
		}
		return nil

	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	blockDevices, machines, err := BlockDevices(dbRows).toBlockDevicesAndMachines(dbDeviceLinks, dbFilesystemTypes, dbMachines)
	if err != nil {
		return nil, errors.Trace(err)
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

	linkDeleteQuery := fmt.Sprintf(`
DELETE 
FROM  block_device_link_device
WHERE block_device_uuid IN (
    SELECT DISTINCT uuid
    FROM            block_device bd
    WHERE           bd.machine_uuid = $M.machine_uuid
)`)

	deleteStmt, err := sqlair.Prepare(linkDeleteQuery, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	if err := tx.Query(ctx, deleteStmt, machineUUIDParam).Run(); err != nil {
		return errors.Annotate(err, "deleting block device link devices")
	}

	deleteQuery := fmt.Sprintf(`
DELETE
FROM  block_device
WHERE machine_uuid = $M.machine_uuid
`)

	deleteStmt, err = sqlair.Prepare(deleteQuery, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	if err := tx.Query(ctx, deleteStmt, machineUUIDParam).Run(); err != nil {
		return errors.Annotate(err, "deleting block devices")
	}
	return nil
}

type getWatcherFunc = func(
	namespace, changeValue string,
	changeMask changestream.ChangeType,
	predicate eventsource.Predicate,
) (watcher.NotifyWatcher, error)

// WatchBlockDevices returns a new NotifyWatcher watching for
// changes to block devices associated with the specified machine.
func (st *State) WatchBlockDevices(
	ctx context.Context, getWatcher getWatcherFunc, machineId string,
) (watcher.NotifyWatcher, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		machineUUID string
		machineLife life.Life
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID, machineLife, err = st.getMachineInfo(ctx, tx, machineId)
		return errors.Trace(err)
	})

	if err != nil {
		return nil, errors.Trace(err)
	}
	if machineLife == life.Dead {
		return nil, errors.Errorf("cannot watch block devices on dead machine %q", machineId)
	}

	predicate := func(ctx context.Context, db coredatabase.TxnRunner, changes []changestream.ChangeEvent) (bool, error) {
		for _, ch := range changes {
			if ch.Changed() == machineUUID {
				return true, nil
			}
		}
		return false, nil
	}
	baseWatcher, err := getWatcher("block_device", machineUUID, changestream.All, predicate)
	if err != nil {
		return nil, errors.Annotatef(err, "watching machine %q block devices", machineId)
	}
	return baseWatcher, nil
}
