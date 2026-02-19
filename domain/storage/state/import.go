// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"

	"github.com/canonical/sqlair"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/internal/errors"
)

// ImportStorageInstances imports storage instances and storage unit
// owners. Storage unit owners are created if the unit name is provided.
func (st *State) ImportStorageInstances(ctx context.Context, args []internal.ImportStorageInstanceArgs) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageInstanceStmt, err := st.Prepare(`
INSERT INTO storage_instance (*) VALUES ($importStorageInstance.*)`, importStorageInstance{})
	if err != nil {
		return errors.Capture(err)
	}

	insertUnitOwnerStmt, err := st.Prepare(`
INSERT INTO storage_unit_owner (*) VALUES ($importStorageUnitOwner.*)`, importStorageUnitOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		storageInstances, storageUnitOwners, err := st.transformStorageInstances(ctx, tx, args)
		if err != nil {
			return err
		}

		err = tx.Query(ctx, insertStorageInstanceStmt, storageInstances).Run()
		if err != nil {
			return errors.Errorf("inserting storage instance rows: %w", err)
		}

		err = tx.Query(ctx, insertUnitOwnerStmt, storageUnitOwners).Run()
		if err != nil {
			return errors.Errorf("inserting storage unit owner rows: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// ImportFilesystemsIAAS imports filesystems from the provided parameters for
// IAAS models.
func (st *State) ImportFilesystemsIAAS(
	ctx context.Context,
	fsArgs []internal.ImportFilesystemIAASArgs,
	attachmentArgs []internal.ImportFilesystemAttachmentIAASArgs,
) error {
	if len(fsArgs)+len(attachmentArgs) == 0 {
		return nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageFilesystemStmt, err := st.Prepare(`
INSERT INTO storage_filesystem (*) VALUES ($importStorageFilesystem.*)`, importStorageFilesystem{})
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageInstanceFilesystemStmt, err := st.Prepare(`
INSERT INTO storage_instance_filesystem (*) VALUES ($importStorageInstanceFilesystem.*)`, importStorageInstanceFilesystem{})
	if err != nil {
		return errors.Capture(err)
	}

	insertFilesystemAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_filesystem_attachment (*) VALUES ($importStorageFilesystemAttachment.*)`, importStorageFilesystemAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	fsDBArgs := make([]importStorageFilesystem, len(fsArgs))
	fsDBInstanceArgs := make([]importStorageInstanceFilesystem, 0, len(fsArgs))
	for i, arg := range fsArgs {
		fsDBArgs[i] = importStorageFilesystem{
			UUID:       arg.UUID,
			ID:         arg.ID,
			LifeID:     int(arg.Life),
			ScopeID:    int(arg.Scope),
			ProviderID: arg.ProviderID,
			SizeInMiB:  arg.SizeInMiB,
		}
		if arg.StorageInstanceUUID != "" {
			fsDBInstanceArgs = append(fsDBInstanceArgs, importStorageInstanceFilesystem{
				StorageInstanceUUID: arg.StorageInstanceUUID,
				FilesystemUUID:      arg.UUID,
			})
		}
	}

	fsAttachmentDBArgs := make([]importStorageFilesystemAttachment, len(attachmentArgs))
	for i, arg := range attachmentArgs {
		fsAttachmentDBArgs[i] = importStorageFilesystemAttachment{
			UUID:           arg.UUID,
			FilesystemUUID: arg.FilesystemUUID,
			NetNodeUUID:    arg.NetNodeUUID,
			ScopeID:        int(arg.Scope),
			LifeID:         int(arg.Life),
			MountPoint:     arg.MountPoint,
			ReadOnly:       arg.ReadOnly,
		}
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if len(fsDBArgs) > 0 {
			err := tx.Query(ctx, insertStorageFilesystemStmt, fsDBArgs).Run()
			if err != nil {
				return errors.Errorf("inserting storage filesystem rows: %w", err)
			}
		}

		if len(fsDBInstanceArgs) > 0 {
			err := tx.Query(ctx, insertStorageInstanceFilesystemStmt, fsDBInstanceArgs).Run()
			if err != nil {
				return errors.Errorf("inserting storage instance filesystem rows: %w", err)
			}
		}

		if len(fsAttachmentDBArgs) > 0 {
			err := tx.Query(ctx, insertFilesystemAttachmentStmt, fsAttachmentDBArgs).Run()
			if err != nil {
				return errors.Errorf("inserting storage filesystem attachment rows: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) transformStorageInstances(
	ctx context.Context,
	tx *sqlair.TX,
	args []internal.ImportStorageInstanceArgs,
) ([]importStorageInstance, []importStorageUnitOwner, error) {
	lookups, err := st.getImportStorageInstanceLookups(ctx, tx)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	storageInstances := make([]importStorageInstance, len(args))
	storageUnitOwners := make([]importStorageUnitOwner, 0)

	for i, arg := range args {
		poolUUID, ok := lookups.StoragePoolUUID[arg.PoolName]
		if !ok {
			return nil, nil, errors.Errorf("pool %q not found for storage instance %q", arg.PoolName, arg.StorageName)
		}
		kind, ok := lookups.Kind[arg.StorageKind]
		if !ok {
			return nil, nil, errors.Errorf("storage kind ID not found for storage instance %q", arg.StorageName)
		}
		storageInstances[i] = importStorageInstance{
			UUID:            arg.UUID,
			LifeID:          arg.Life,
			StorageID:       arg.StorageID,
			StorageKindID:   kind,
			StorageName:     arg.StorageName,
			StoragePoolUUID: poolUUID,
			RequestedSize:   arg.RequestedSizeMiB,
		}

		charmName, unit, err := st.getCharmNameAndUnitUUIDFromUnitName(ctx, tx, arg.UnitName)
		if errors.Is(err, coreerrors.NotFound) {
			// Neither charmName in storage_instance storage_unit_owner rows
			// are required by the DDL.
			continue
		} else if err != nil {
			return nil, nil, errors.Errorf("getting charm and unit uuid from %q: w", err)
		}

		storageInstances[i].CharmName = charmName
		storageUnitOwners = append(storageUnitOwners, importStorageUnitOwner{
			StorageInstanceUUID: arg.UUID,
			UnitUUID:            unit,
		})
	}

	return storageInstances, storageUnitOwners, nil
}

func (st *State) getCharmNameAndUnitUUIDFromUnitName(
	ctx context.Context,
	tx *sqlair.TX,
	unitName string,
) (string, string, error) {
	if unitName == "" {
		return "", "", coreerrors.NotFound
	}
	stmt, err := st.Prepare(`
SELECT (cm.name, u.uuid) AS (&nameAndUUID.*)
FROM   unit AS u
JOIN   charm_metadata AS cm ON u.charm_uuid = cm.charm_uuid
WHERE  u.name = $name.name`, name{}, nameAndUUID{})
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", coreerrors.NotFound
	} else if err != nil {
		return "", "", errors.Capture(err)
	}
	var output nameAndUUID
	err = tx.Query(ctx, stmt, name{Name: unitName}).Get(&output)
	if err != nil {
		return "", "", errors.Errorf("finding charm name and unit uuid for %q: %w", unitName, err)
	}
	return output.Name, output.UUID, nil
}

// GetNetNodeUUIDsByMachineOrUnitName returns net node UUIDs for all machine or
// and unit names provided. If a machine name or unit name is not found then it
// is excluded from the result.
func (st *State) GetNetNodeUUIDsByMachineOrUnitName(ctx context.Context, machines []string, units []string) (map[string]string, map[string]string, error) {
	if len(machines)+len(units) == 0 {
		return nil, nil, nil
	}
	db, err := st.DB(ctx)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	slices.Sort(machines)
	machines = slices.Compact(machines)
	slices.Sort(units)
	units = slices.Compact(units)

	type machineNames []string
	type unitNames []string
	var (
		machineNameInput = machineNames(machines)
		unitNameInput    = unitNames(units)
	)
	stmt, err := st.Prepare(`
SELECT &machineAndUnitNetNodeUUID.*
FROM (
    SELECT name AS machine_name,
           net_node_uuid AS machine_net_node_uuid,
           NULL AS unit_name,
           NULL AS unit_net_node_uuid
    FROM   machine
    WHERE  name IN ($machineNames[:])
    UNION
    SELECT NULL AS machine_name,
           NULL AS machine_net_node_uuid,
           name AS unit_name,
           net_node_uuid AS unit_net_node_uuid
    FROM   unit
    WHERE  name IN ($unitNames[:])
) 
`, machineNameInput, unitNameInput, machineAndUnitNetNodeUUID{})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	var netNodeUUIDs []machineAndUnitNetNodeUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, machineNameInput, unitNameInput).GetAll(&netNodeUUIDs)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	machineMap := make(map[string]string, len(machineNameInput))
	unitMap := make(map[string]string, len(unitNameInput))
	for _, dbVal := range netNodeUUIDs {
		if dbVal.MachineName.Valid {
			machineMap[dbVal.MachineName.String] = dbVal.MachineNetNodeUUID.String
		}
		if dbVal.UnitName.Valid {
			unitMap[dbVal.UnitName.String] = dbVal.UnitNetNodeUUID.String
		}
	}
	return machineMap, unitMap, nil
}

// ImportVolumes creates new volumes and storage instance volumes.
func (st *State) ImportVolumes(ctx context.Context, args []internal.ImportVolumeArgs) error {
	if len(args) == 0 {
		return nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	storageVolumeData, storageInstanceVolumeData := makeInsertVolumeArgs(args)
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.importStorageVolumes(ctx, tx, storageVolumeData); err != nil {
			return errors.Capture(err)
		}
		if err := st.importStorageInstanceVolumes(ctx, tx, storageInstanceVolumeData); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
}

func (st *State) importStorageVolumes(ctx context.Context, tx *sqlair.TX, input []importStorageVolume) error {
	if len(input) == 0 {
		return nil
	}

	insertStmt, err := st.Prepare(`
INSERT INTO storage_volume (*) VALUES ($importStorageVolume.*)
`, importStorageVolume{})
	if err != nil {
		return errors.Errorf("preparing insert volume import statement: %w", err)
	}

	err = tx.Query(ctx, insertStmt, input).Run()
	return err
}

func (st *State) importStorageInstanceVolumes(ctx context.Context, tx *sqlair.TX, input []importStorageInstanceVolume) error {
	if len(input) == 0 {
		return nil
	}

	insertStmt, err := st.Prepare(`
INSERT INTO storage_instance_volume (*) VALUES ($importStorageInstanceVolume.*)
`, importStorageInstanceVolume{})
	if err != nil {
		return errors.Errorf("preparing insert storage instance volume import statement: %w", err)
	}

	err = tx.Query(ctx, insertStmt, input).Run()
	return err
}

func makeInsertVolumeArgs(args []internal.ImportVolumeArgs) ([]importStorageVolume, []importStorageInstanceVolume) {
	out := make([]importStorageVolume, len(args))
	outInstance := make([]importStorageInstanceVolume, len(args))

	for i, arg := range args {
		out[i] = importStorageVolume{
			UUID:             arg.UUID,
			VolumeID:         arg.ID,
			LifeID:           int(arg.LifeID),
			ProvisionScopeID: int(arg.ProvisionScopeID),
			ProviderID:       arg.ProviderID,
			SizeMiB:          arg.SizeMiB,
			HardwareID:       arg.HardwareID,
			WWN:              arg.WWN,
			Persistent:       arg.Persistent,
		}
		outInstance[i] = importStorageInstanceVolume{
			StorageInstanceUUID: arg.StorageInstanceUUID,
			VolumeUUID:          arg.UUID,
		}
	}

	return out, outInstance
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
	ctx context.Context, machineName string,
) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	in := name{
		Name: machineName,
	}
	out := entityLife{}

	stmt, err := st.Prepare(`
SELECT &entityLife.*
FROM   machine
WHERE  name = $name.name`, in, out)
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

	return out.UUID, err
}
