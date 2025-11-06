// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/blockdevice"
	blockdeviceerrors "github.com/juju/juju/domain/blockdevice/errors"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
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

// GetBlockDevice retrieves the info for the specified block device.
//
// The following errors may be returned:
// - [blockdeviceerrors.BlockDeviceNotFound] when the block device is not found.
func (st *State) GetBlockDevice(
	ctx context.Context, uuid blockdevice.BlockDeviceUUID,
) (coreblockdevice.BlockDevice, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return coreblockdevice.BlockDevice{}, errors.Capture(err)
	}

	input := entityUUID{
		UUID: uuid.String(),
	}
	blockDeviceStmt, err := st.Prepare(`
SELECT &blockDevice.*
FROM   block_device
WHERE  uuid = $entityUUID.uuid
`, input, blockDevice{})
	if err != nil {
		return coreblockdevice.BlockDevice{}, errors.Capture(err)
	}

	blockDeviceLinkStmt, err := st.Prepare(`
SELECT &deviceLink.*
FROM   block_device_link_device
WHERE  block_device_uuid = $entityUUID.uuid
`, input, deviceLink{})
	if err != nil {
		return coreblockdevice.BlockDevice{}, errors.Capture(err)
	}

	var blockDevice blockDevice
	var devLinks []deviceLink
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, blockDeviceStmt, input).Get(&blockDevice)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"block device %q not found", uuid,
			).Add(blockdeviceerrors.BlockDeviceNotFound)
		} else if err != nil {
			return err
		}
		err = tx.Query(ctx, blockDeviceLinkStmt, input).GetAll(&devLinks)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return coreblockdevice.BlockDevice{}, errors.Capture(err)
	}

	retVal := coreblockdevice.BlockDevice{
		DeviceName:      blockDevice.Name.V,
		FilesystemLabel: blockDevice.FilesystemLabel,
		FilesystemUUID:  blockDevice.HostFilesystemUUID,
		HardwareId:      blockDevice.HardwareId,
		WWN:             blockDevice.WWN,
		BusAddress:      blockDevice.BusAddress,
		SizeMiB:         blockDevice.SizeMiB,
		FilesystemType:  blockDevice.FilesystemType,
		InUse:           blockDevice.InUse,
		MountPoint:      blockDevice.MountPoint,
		SerialId:        blockDevice.SerialId,
	}
	for _, v := range devLinks {
		retVal.DeviceLinks = append(retVal.DeviceLinks, v.Name)
	}

	return retVal, nil
}

// GetBlockDevicesForMachine returns the BlockDevices for the specified machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] when the machine is not found.
// - [machineerrors.MachineIsDead] when the machine is dead.
func (st *State) GetBlockDevicesForMachine(
	ctx context.Context, machineUUID machine.UUID,
) (map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkMachineNotDead(ctx, tx, machineUUID)
		if err != nil {
			return err
		}
		result, err = st.loadBlockDevices(ctx, tx, machineUUID)
		return errors.Capture(err)
	})
	return result, errors.Capture(err)
}

func (st *State) loadBlockDevices(
	ctx context.Context, tx *sqlair.TX, machineUUID machine.UUID,
) (map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice, error) {
	input := entityUUID{
		UUID: machineUUID.String(),
	}

	blockDeviceStmt, err := st.Prepare(`
SELECT &blockDevice.*
FROM   block_device
WHERE  machine_uuid = $entityUUID.uuid
`, input, blockDevice{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	blockDeviceLinkStmt, err := st.Prepare(`
SELECT &deviceLink.*
FROM   block_device_link_device
WHERE  machine_uuid = $entityUUID.uuid
`, input, deviceLink{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var blockDevices []blockDevice
	err = tx.Query(ctx, blockDeviceStmt, input).GetAll(&blockDevices)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf(
			"loading block devices for machine %q: %w",
			machineUUID, err,
		)
	}

	var devLinks []deviceLink
	err = tx.Query(ctx, blockDeviceLinkStmt, input).GetAll(&devLinks)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf(
			"loading block device dev links for machine %q: %w",
			machineUUID, err,
		)
	}

	retVal := make(
		map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
		len(blockDevices),
	)
	for _, bd := range blockDevices {
		uuid := blockdevice.BlockDeviceUUID(bd.UUID)
		retVal[uuid] = coreblockdevice.BlockDevice{
			DeviceName:      bd.Name.V,
			FilesystemLabel: bd.FilesystemLabel,
			FilesystemUUID:  bd.HostFilesystemUUID,
			HardwareId:      bd.HardwareId,
			WWN:             bd.WWN,
			BusAddress:      bd.BusAddress,
			SizeMiB:         bd.SizeMiB,
			FilesystemType:  bd.FilesystemType,
			InUse:           bd.InUse,
			MountPoint:      bd.MountPoint,
			SerialId:        bd.SerialId,
		}
	}
	for _, dl := range devLinks {
		uuid := blockdevice.BlockDeviceUUID(dl.BlockDeviceUUID)
		r, ok := retVal[uuid]
		if !ok {
			continue
		}
		r.DeviceLinks = append(r.DeviceLinks, dl.Name)
		retVal[uuid] = r
	}

	return retVal, nil
}

func (st *State) checkMachineNotDead(
	ctx context.Context, tx *sqlair.TX, machineUUID machine.UUID,
) error {
	io := entityLife{
		UUID: machineUUID.String(),
	}

	stmt, err := st.Prepare(`
SELECT &entityLife.*
FROM   machine
WHERE  uuid = $entityLife.uuid`, io)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, io).Get(&io)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf(
			"machine %q not found",
			machineUUID,
		).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return errors.Capture(err)
	}

	if io.Life == life.Dead {
		return errors.Errorf(
			"machine %q is dead",
			machineUUID,
		).Add(machineerrors.MachineIsDead)
	}
	return nil
}

// GetMachineUUIDByName gets the uuid for the named machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] when the machine is not found.
// - [machineerrors.MachineIsDead] when the machine is dead.
func (st *State) GetMachineUUIDByName(
	ctx context.Context, machineName machine.Name,
) (machine.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	in := entityName{
		Name: machineName.String(),
	}
	out := entityLife{}

	stmt, err := st.Prepare(`
SELECT &entityLife.*
FROM   machine
WHERE  name = $entityName.name`, in, out)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, in).Get(&out)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"machine %q not found",
				machineName,
			).Add(machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		if out.Life == life.Dead {
			return errors.Errorf(
				"machine %q is dead",
				machineName,
			).Add(machineerrors.MachineIsDead)
		}
		return nil
	})

	return machine.UUID(out.UUID), nil
}

// UpdateBlockDevicesForMachine updates the block devices for the specified
// machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] when the machine is not found.
// - [machineerrors.MachineIsDead] when the machine is dead.
func (st *State) UpdateBlockDevicesForMachine(
	ctx context.Context, machineUUID machine.UUID,
	added map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
	updated map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
	removeable []blockdevice.BlockDeviceUUID,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkMachineNotDead(ctx, tx, machineUUID)
		if err != nil {
			return errors.Capture(err)
		}

		if len(removeable) > 0 {
			err = st.removeUnreferencedBlockDevices(ctx, tx, removeable)
			if err != nil {
				return errors.Errorf("removing old block devices: %w", err)
			}
		}

		if len(updated) > 0 {
			err = st.updateBlockDevices(ctx, tx, machineUUID, updated)
			if err != nil {
				return errors.Errorf("updating block devices: %w", err)
			}
		}

		if len(added) > 0 {
			err = st.insertBlockDevices(ctx, tx, machineUUID, added)
			if err != nil {
				return errors.Errorf("adding new block devices: %w", err)
			}
		}

		return nil
	})

	return errors.Capture(err)
}

// removeUnreferencedBlockDevices deletes all the block devices specified if
// they are not referenced by a storage volume attachment.`
func (st *State) removeUnreferencedBlockDevices(
	ctx context.Context,
	tx *sqlair.TX,
	deviceUUIDs []blockdevice.BlockDeviceUUID,
) error {
	type blockDeviceUUIDs []string
	input := make(blockDeviceUUIDs, 0, len(deviceUUIDs))
	for _, v := range deviceUUIDs {
		input = append(input, v.String())
	}

	selectBlockDevicesStmt, err := st.Prepare(`
SELECT    bd.uuid AS &entityUUID.uuid
FROM      block_device bd
LEFT JOIN storage_volume_attachment sva ON bd.uuid=sva.block_device_uuid
WHERE     bd.uuid IN ($blockDeviceUUIDs[:]) AND sva.block_device_uuid IS NULL
`, input, entityUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	deleteDevLinksStmt, err := st.Prepare(`
DELETE
FROM block_device_link_device
WHERE block_device_uuid IN ($blockDeviceUUIDs[:])
`, input)
	if err != nil {
		return errors.Capture(err)
	}

	deleteBlockDevicesStmt, err := st.Prepare(`
DELETE
FROM  block_device
WHERE uuid IN ($blockDeviceUUIDs[:])
`, input)
	if err != nil {
		return errors.Capture(err)
	}

	var deleteUUIDs []entityUUID
	err = tx.Query(ctx, selectBlockDevicesStmt, input).GetAll(&deleteUUIDs)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil
	} else if err != nil {
		return errors.Capture(err)
	}
	input = transform.Slice(deleteUUIDs, func(v entityUUID) string {
		return v.UUID
	})

	err = tx.Query(ctx, deleteDevLinksStmt, input).Run()
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, deleteBlockDevicesStmt, input).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) updateBlockDevices(
	ctx context.Context, tx *sqlair.TX, machineUUID machine.UUID,
	devices map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
) error {
	type blockDeviceUUIDs []string

	deleteOldLinkStmt, err := st.Prepare(`
DELETE
FROM   block_device_link_device
WHERE  block_device_uuid IN ($blockDeviceUUIDs[:])
`, blockDeviceUUIDs{})
	if err != nil {
		return errors.Capture(err)
	}

	insertNewLinkStmt, err := st.Prepare(`
INSERT INTO block_device_link_device (block_device_uuid, machine_uuid, name)
VALUES ($deviceLink.*)
`, deviceLink{})
	if err != nil {
		return errors.Capture(err)
	}

	updateBlockDeviceStmt, err := st.Prepare(`
UPDATE block_device
SET    name = $blockDevice.name,
       hardware_id = $blockDevice.hardware_id,
       wwn = $blockDevice.wwn,
       serial_id = $blockDevice.serial_id,
       bus_address = $blockDevice.bus_address,
       size_mib = $blockDevice.size_mib,
       mount_point = $blockDevice.mount_point,
       in_use = $blockDevice.in_use,
       filesystem_label = $blockDevice.filesystem_label,
       host_filesystem_uuid = $blockDevice.host_filesystem_uuid,
       filesystem_type = $blockDevice.filesystem_type
WHERE  uuid = $blockDevice.uuid
`, blockDevice{})
	if err != nil {
		return errors.Capture(err)
	}

	var uuids blockDeviceUUIDs
	for k := range devices {
		uuids = append(uuids, k.String())
	}
	err = tx.Query(ctx, deleteOldLinkStmt, uuids).Run()
	if err != nil {
		return errors.Errorf("deleting block device links: %w", err)
	}

	var devLinks []deviceLink
	for blockDeviceUUID, bd := range devices {
		for _, v := range bd.DeviceLinks {
			devLinks = append(devLinks, deviceLink{
				BlockDeviceUUID: blockDeviceUUID.String(),
				MachineUUID:     machineUUID.String(),
				Name:            v,
			})
		}
	}
	if len(devLinks) > 0 {
		err = tx.Query(ctx, insertNewLinkStmt, devLinks).Run()
		if err != nil {
			return errors.Errorf("inserting block device links: %w", err)
		}
	}

	for blockDeviceUUID, bd := range devices {
		val := blockDevice{
			UUID:               blockDeviceUUID.String(),
			MachineUUID:        machineUUID.String(),
			HardwareId:         bd.HardwareId,
			WWN:                bd.WWN,
			BusAddress:         bd.BusAddress,
			SerialId:           bd.SerialId,
			SizeMiB:            bd.SizeMiB,
			FilesystemLabel:    bd.FilesystemLabel,
			HostFilesystemUUID: bd.FilesystemUUID,
			FilesystemType:     bd.FilesystemType,
			InUse:              bd.InUse,
			MountPoint:         bd.MountPoint,
		}
		if bd.DeviceName != "" {
			val.Name = sql.Null[string]{
				V:     bd.DeviceName,
				Valid: true,
			}
		}
		err = tx.Query(ctx, updateBlockDeviceStmt, val).Run()
		if err != nil {
			return errors.Errorf("updating block devices: %w", err)
		}
	}

	return nil
}

func (st *State) insertBlockDevices(
	ctx context.Context, tx *sqlair.TX, machineUUID machine.UUID,
	devices map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
) error {
	insertQuery := `
INSERT INTO block_device (*)
VALUES ($blockDevice.*)`
	insertStmt, err := st.Prepare(insertQuery, blockDevice{})
	if err != nil {
		return errors.Capture(err)
	}

	insertLinkQuery := `
INSERT INTO block_device_link_device (*)
VALUES ($deviceLink.*)
`
	insertLinkStmt, err := st.Prepare(insertLinkQuery, deviceLink{})
	if err != nil {
		return errors.Capture(err)
	}

	inputBlockDevices := make([]blockDevice, 0, len(devices))
	numLinks := 0
	for uuid, bd := range devices {
		inputBlockDevice := blockDevice{
			UUID:               uuid.String(),
			MachineUUID:        machineUUID.String(),
			FilesystemLabel:    bd.FilesystemLabel,
			HostFilesystemUUID: bd.FilesystemUUID,
			HardwareId:         bd.HardwareId,
			WWN:                bd.WWN,
			BusAddress:         bd.BusAddress,
			SerialId:           bd.SerialId,
			MountPoint:         bd.MountPoint,
			SizeMiB:            bd.SizeMiB,
			FilesystemType:     bd.FilesystemType,
			InUse:              bd.InUse,
		}
		if bd.DeviceName != "" {
			inputBlockDevice.Name = sql.Null[string]{
				V:     bd.DeviceName,
				Valid: true,
			}
		}
		inputBlockDevices = append(inputBlockDevices, inputBlockDevice)
		numLinks += len(bd.DeviceLinks)
	}

	err = tx.Query(ctx, insertStmt, inputBlockDevices).Run()
	if err != nil {
		return errors.Errorf("inserting block devices: %w", err)
	}

	if numLinks == 0 {
		return nil
	}

	inputDevLinks := make([]deviceLink, 0, numLinks)
	for uuid, bd := range devices {
		for _, link := range bd.DeviceLinks {
			inputDevLinks = append(inputDevLinks, deviceLink{
				BlockDeviceUUID: uuid.String(),
				MachineUUID:     machineUUID.String(),
				Name:            link,
			})
		}
	}

	err = tx.Query(ctx, insertLinkStmt, inputDevLinks).Run()
	if err != nil {
		return errors.Errorf("inserting block device links: %w", err)
	}

	return nil
}

// GetBlockDevicesForAllMachines retrieves block devices for all machines.
func (st *State) GetBlockDevicesForAllMachines(
	ctx context.Context,
) (map[machine.Name][]coreblockdevice.BlockDevice, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	blockDeviceStmt, err := st.Prepare(`
SELECT &blockDevice.*
FROM   block_device
`, blockDevice{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	devLinkStmt, err := st.Prepare(`
SELECT &deviceLink.*
FROM   block_device_link_device
`, deviceLink{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	machineNameStmt, err := st.Prepare(`
SELECT &entityName.*
FROM   machine
`, entityName{})

	if err != nil {
		return nil, errors.Capture(err)
	}
	var (
		blockDevices []blockDevice
		devLinks     []deviceLink
		machineNames []entityName
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, blockDeviceStmt).GetAll(&blockDevices)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("loading block devices: %w", err)
		}
		err = tx.Query(ctx, devLinkStmt).GetAll(&devLinks)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("loading block device dev links: %w", err)
		}
		err = tx.Query(ctx, machineNameStmt).GetAll(&machineNames)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("loading machine names: %w", err)
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	devLinkMap := make(map[string][]string, len(blockDevices))
	for _, devLink := range devLinks {
		devLinkMap[devLink.BlockDeviceUUID] = append(
			devLinkMap[devLink.BlockDeviceUUID], devLink.Name)
	}

	machineNameMap := make(map[string]machine.Name, len(machineNames))
	for _, m := range machineNames {
		machineNameMap[m.UUID] = machine.Name(m.Name)
	}

	res := make(map[machine.Name][]coreblockdevice.BlockDevice, len(blockDevices))
	for _, bd := range blockDevices {
		machineName := machineNameMap[bd.MachineUUID]
		res[machineName] = append(res[machineName], coreblockdevice.BlockDevice{
			DeviceName:      bd.Name.V,
			DeviceLinks:     devLinkMap[bd.UUID],
			FilesystemUUID:  bd.HostFilesystemUUID,
			FilesystemLabel: bd.FilesystemLabel,
			FilesystemType:  bd.FilesystemType,
			HardwareId:      bd.HardwareId,
			WWN:             bd.WWN,
			BusAddress:      bd.BusAddress,
			SizeMiB:         bd.SizeMiB,
			InUse:           bd.InUse,
			MountPoint:      bd.MountPoint,
			SerialId:        bd.SerialId,
		})
	}

	return res, nil
}

// NamespaceForWatchBlockDevices returns the change stream namespace for
// watching block devices.
func (st *State) NamespaceForWatchBlockDevices() string {
	return "block_device"
}
