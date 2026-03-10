// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/internal/errors"
)

// ImportStorageInstances imports storage instances, storage attachments and
// storage unit owners if a unit uuid is provided.
func (st *State) ImportStorageInstances(
	ctx context.Context,
	instanceArgs []internal.ImportStorageInstanceArgs,
	attachmentArgs []internal.ImportStorageInstanceAttachmentArgs,
) error {
	if len(instanceArgs) == 0 {
		return nil
	}

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

	storageAttachments := transform.Slice(attachmentArgs, func(arg internal.ImportStorageInstanceAttachmentArgs) importStorageAttachment {
		return importStorageAttachment{
			UUID:                arg.UUID,
			StorageInstanceUUID: arg.StorageInstanceUUID,
			UnitUUID:            arg.UnitUUID,
			LifeID:              int(arg.Life),
		}
	})

	insertStorageAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_attachment (*) VALUES ($importStorageAttachment.*)`, importStorageAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		storageInstances, storageUnitOwners, err := st.transformStorageInstances(ctx, tx, instanceArgs)
		if err != nil {
			return err
		}

		if len(storageInstances) > 0 {
			err := tx.Query(ctx, insertStorageInstanceStmt, storageInstances).Run()
			if err != nil {
				return errors.Errorf("inserting storage instance rows: %w", err)
			}
		}

		if len(storageUnitOwners) > 0 {
			err := tx.Query(ctx, insertUnitOwnerStmt, storageUnitOwners).Run()
			if err != nil {
				return errors.Errorf("inserting storage unit owner rows: %w", err)
			}
		}

		if len(storageAttachments) > 0 {
			err := tx.Query(ctx, insertStorageAttachmentStmt, storageAttachments).Run()
			if err != nil {
				return errors.Errorf("inserting storage attachment rows: %w", err)
			}
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
	if len(args) == 0 {
		return nil, nil, nil
	}

	lookups, err := st.getImportStorageInstanceLookups(ctx, tx)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	storageInstances := make([]importStorageInstance, len(args))
	storageUnitOwners := make([]importStorageUnitOwner, 0)

	stmt, err := st.Prepare(`
SELECT cm.name AS &name.name
FROM   unit AS u
JOIN   charm_metadata AS cm ON u.charm_uuid = cm.charm_uuid
WHERE  u.uuid = $entityUUID.uuid`, name{}, entityUUID{})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

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
			LifeID:          int(arg.Life),
			StorageID:       arg.StorageInstanceID,
			StorageKindID:   kind,
			StorageName:     arg.StorageName,
			StoragePoolUUID: poolUUID,
			RequestedSize:   arg.RequestedSizeMiB,
		}

		if arg.UnitUUID == "" {
			continue
		}

		var charmName name
		err := tx.Query(ctx, stmt, entityUUID{UUID: arg.UnitUUID}).Get(&charmName)
		if errors.Is(err, sql.ErrNoRows) {
			// Neither charmName in storage_instance storage_unit_owner rows
			// are required by the DDL.
			continue
		} else if err != nil {
			return nil, nil, errors.Errorf("getting charm name from unit %q: %w", arg.UnitUUID, err)
		}

		storageInstances[i].CharmName = charmName.Name
		storageUnitOwners = append(storageUnitOwners, importStorageUnitOwner{
			StorageInstanceUUID: arg.UUID,
			UnitUUID:            arg.UnitUUID,
		})
	}

	return storageInstances, storageUnitOwners, nil
}

// GetNetNodeUUIDsByMachineOrUnitName returns net node UUIDs for all machine or
// and unit names provided. If a machine name or unit name is not found then it
// is excluded from the result.
func (st *State) GetNetNodeUUIDsByMachineOrUnitName(
	ctx context.Context,
	machines []machine.Name,
	units []unit.Name,
) (map[machine.Name]network.NetNodeUUID, map[unit.Name]network.NetNodeUUID, error) {
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
		machineNameInput = machineNames(transform.Slice(machines, func(in machine.Name) string { return string(in) }))
		unitNameInput    = unitNames(transform.Slice(units, func(in unit.Name) string { return string(in) }))
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
	machineMap := make(map[machine.Name]network.NetNodeUUID, len(machineNameInput))
	unitMap := make(map[unit.Name]network.NetNodeUUID, len(unitNameInput))
	for _, dbVal := range netNodeUUIDs {
		if dbVal.MachineName.Valid {
			machineMap[machine.Name(dbVal.MachineName.String)] = network.NetNodeUUID(dbVal.MachineNetNodeUUID.String)
		}
		if dbVal.UnitName.Valid {
			unitMap[unit.Name(dbVal.UnitName.String)] = network.NetNodeUUID(dbVal.UnitNetNodeUUID.String)
		}
	}
	return machineMap, unitMap, nil
}

// GetUnitUUIDsByNames returns a map of unit names to unit UUIDs for the provided
// unit names.
func (st *State) GetUnitUUIDsByNames(ctx context.Context, units []string) (map[string]string, error) {
	if len(units) == 0 {
		return map[string]string{}, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type unitNames []string
	unitNameInput := unitNames(units)

	stmt, err := st.Prepare(`
SELECT &nameAndUUID.*
FROM unit
WHERE name IN ($unitNames[:])
	`, unitNameInput, nameAndUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var nameAndUUIDs []nameAndUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitNameInput).GetAll(&nameAndUUIDs)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string]string, len(nameAndUUIDs))
	for _, nu := range nameAndUUIDs {
		result[nu.Name] = nu.UUID
	}
	return result, nil
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

	insertData := makeInsertVolumeArgs(args)

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.importStorageVolumes(ctx, tx, insertData.volumes); err != nil {
			return errors.Errorf("importing volume: %w", err)
		}
		if err := st.importStorageInstanceVolumes(ctx, tx, insertData.instances); err != nil {
			return errors.Errorf("importing storage instance volumes: %w", err)
		}
		if err := st.importStorageVolumeAttachments(ctx, tx, insertData.attachments); err != nil {
			return errors.Errorf("importing volume attachments: %w", err)
		}
		if err := st.importVolumeAttachmentPlans(ctx, tx, insertData.plans); err != nil {
			return errors.Errorf("importing volume attachment plans: %w", err)
		}
		// Volume attachment plan attributes require the plan to be inserted.
		if err := st.importVolumeAttachmentPlanAttributes(ctx, tx, insertData.planAttributes); err != nil {
			return errors.Errorf("importing volume attachment plan attributes: %w", err)
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
		return err
	}

	return tx.Query(ctx, insertStmt, input).Run()
}

func (st *State) importStorageInstanceVolumes(ctx context.Context, tx *sqlair.TX, input []importStorageInstanceVolume) error {
	if len(input) == 0 {
		return nil
	}

	insertStmt, err := st.Prepare(`
INSERT INTO storage_instance_volume (*) VALUES ($importStorageInstanceVolume.*)
`, importStorageInstanceVolume{})
	if err != nil {
		return err
	}

	return tx.Query(ctx, insertStmt, input).Run()
}

func (st *State) importStorageVolumeAttachments(ctx context.Context, tx *sqlair.TX, input []importStorageVolumeAttachment) error {
	if len(input) == 0 {
		return nil
	}

	insertStmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment (*) VALUES ($importStorageVolumeAttachment.*)
`, importStorageVolumeAttachment{})
	if err != nil {
		return err
	}

	return tx.Query(ctx, insertStmt, input).Run()
}

func (st *State) importVolumeAttachmentPlans(ctx context.Context, tx *sqlair.TX, input []importStorageVolumeAttachmentPlan) error {
	if len(input) == 0 {
		return nil
	}

	insertStmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment_plan (*) VALUES ($importStorageVolumeAttachmentPlan.*)
`, importStorageVolumeAttachmentPlan{})
	if err != nil {
		return err
	}

	return tx.Query(ctx, insertStmt, input).Run()
}

func (st *State) importVolumeAttachmentPlanAttributes(ctx context.Context, tx *sqlair.TX, input []importStorageVolumePlanAttribute) error {
	if len(input) == 0 {
		return nil
	}

	insertStmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment_plan_attr (*) VALUES ($importStorageVolumePlanAttribute.*)
`, importStorageVolumePlanAttribute{})
	if err != nil {
		return err
	}

	return tx.Query(ctx, insertStmt, input).Run()
}

type insertVolumeData struct {
	volumes        []importStorageVolume
	instances      []importStorageInstanceVolume
	attachments    []importStorageVolumeAttachment
	plans          []importStorageVolumeAttachmentPlan
	planAttributes []importStorageVolumePlanAttribute
}

func makeInsertVolumeArgs(args []internal.ImportVolumeArgs) insertVolumeData {
	out := insertVolumeData{
		volumes:     make([]importStorageVolume, len(args)),
		instances:   make([]importStorageInstanceVolume, len(args)),
		attachments: make([]importStorageVolumeAttachment, 0),
		plans:       make([]importStorageVolumeAttachmentPlan, 0),
	}

	for i, arg := range args {
		out.volumes[i] = importStorageVolume{
			UUID:             arg.UUID.String(),
			VolumeID:         arg.ID,
			LifeID:           int(arg.LifeID),
			ProvisionScopeID: int(arg.ProvisionScopeID),
			ProviderID:       arg.ProviderID,
			SizeMiB:          arg.SizeMiB,
			HardwareID:       arg.HardwareID,
			WWN:              arg.WWN,
			Persistent:       arg.Persistent,
		}
		out.instances[i] = importStorageInstanceVolume{
			StorageInstanceUUID: arg.StorageInstanceUUID.String(),
			VolumeUUID:          arg.UUID.String(),
		}

		for _, attach := range arg.Attachments {
			out.attachments = append(out.attachments, importStorageVolumeAttachment{
				UUID:              attach.UUID.String(),
				BlockDeviceUUID:   attach.BlockDeviceUUID.String(),
				LifeID:            int(attach.LifeID),
				NetNodeUUID:       attach.NetNodeUUID.String(),
				ProvisionScopeID:  int(arg.ProvisionScopeID),
				ProviderID:        arg.ProviderID,
				ReadOnly:          attach.ReadOnly,
				StorageVolumeUUID: arg.UUID.String(),
			})
		}
		for _, plan := range arg.AttachmentPlans {
			out.plans = append(out.plans, importStorageVolumeAttachmentPlan{
				UUID: plan.UUID.String(),
				DeviceTypeID: sql.NullInt64{
					Int64: int64(dereferenceOrEmpty(plan.DeviceTypeID)),
					Valid: isNotNil(plan.DeviceTypeID),
				},
				LifeID:            int(plan.LifeID),
				NetNodeUUID:       plan.NetNodeUUID.String(),
				ProvisionScopeID:  int(plan.ProvisionScopeID),
				StorageVolumeUUID: arg.UUID.String(),
			})
			for key, value := range plan.DeviceAttributes {
				out.planAttributes = append(out.planAttributes, importStorageVolumePlanAttribute{
					PlanUUID: plan.UUID.String(),
					Key:      key,
					Value:    value,
				})
			}
		}
	}

	return out
}

// dereferenceOrEmpty is handy for assigning values to the sql.Null* types.
func dereferenceOrEmpty[T any](val *T) T {
	if val == nil {
		var empty T
		return empty
	}
	return *val
}

// isNotNil is handy for assigning validity to the sql.Null* types.
func isNotNil[T any](val *T) bool {
	return val != nil
}

// GetBlockDevicesForMachineByNetNodeUUID returns the BlockDevices for the
// specified machines.
func (st *State) GetBlockDevicesForMachinesByNetNodeUUIDs(
	ctx context.Context, netNodeUUIDs []network.NetNodeUUID,
) (map[network.NetNodeUUID][]internal.BlockDevice, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	blockDeviceStmt, err := st.Prepare(`
SELECT (bd.uuid, bd.name, bd.hardware_id, bd.wwn, bd.bus_address,
       bd.serial_id, bd.size_mib, bd.filesystem_label, bd.host_filesystem_uuid,
       bd.filesystem_type, bd.in_use, bd.mount_point, m.net_node_uuid) AS (&blockDevice.*)
FROM   block_device AS bd
JOIN   machine AS m ON bd.machine_uuid = m.uuid
WHERE  m.net_node_uuid IN ($uuids[:])
`, uuids{}, blockDevice{})
	if err != nil {
		return nil, errors.Errorf("preparing block device query: %w", err)
	}

	blockDeviceLinkStmt, err := st.Prepare(`
SELECT (bd.block_device_uuid, bd.name, m.net_node_uuid) AS (&deviceLink.*)
FROM   block_device_link_device AS bd
JOIN   machine AS m ON bd.machine_uuid = m.uuid
WHERE  m.net_node_uuid IN ($uuids[:])
`, uuids{}, deviceLink{})
	if err != nil {
		return nil, errors.Errorf("preparing block device link device query: %w", err)
	}

	nodeUUIDs := transform.Slice(netNodeUUIDs, func(in network.NetNodeUUID) string { return string(in) })
	var (
		blockDevices []blockDevice
		devLinks     []deviceLink
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		qErr := tx.Query(ctx, blockDeviceStmt, uuids(nodeUUIDs)).GetAll(&blockDevices)
		if errors.Is(qErr, sqlair.ErrNoRows) {
			return nil
		} else if qErr != nil {
			return errors.Errorf(
				"querying block devices: %w", qErr,
			)
		}
		qErr = tx.Query(ctx, blockDeviceLinkStmt, uuids(nodeUUIDs)).GetAll(&devLinks)
		if qErr != nil && !errors.Is(qErr, sqlair.ErrNoRows) {
			return errors.Errorf(
				"querying block device dev links: %w", qErr,
			)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Arrange the block devices into a map of net node UUIDs to a map of
	// block device UUIDs to block devices. This will facilitate adding the
	// device links to the block devices.
	interim := make(
		map[network.NetNodeUUID]map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
		len(netNodeUUIDs),
	)
	for _, bd := range blockDevices {
		bdUUID := blockdevice.BlockDeviceUUID(bd.UUID)
		netNodeUUID := network.NetNodeUUID(bd.NetNodeUUID)
		devices, ok := interim[netNodeUUID]
		if !ok {
			devices = make(map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice, 0)
		}
		devices[bdUUID] = coreblockdevice.BlockDevice{
			DeviceName:      bd.Name,
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
		interim[netNodeUUID] = devices
	}

	// Add devices links to their block devices.
	for _, dl := range devLinks {
		bdUUID := blockdevice.BlockDeviceUUID(dl.BlockDeviceUUID)
		netNodeUUID := network.NetNodeUUID(dl.NetNodeUUID)
		bds, ok := interim[netNodeUUID]
		if !ok {
			continue
		}
		bd, ok := bds[bdUUID]
		if !ok {
			continue
		}
		bd.DeviceLinks = append(bd.DeviceLinks, dl.Name)
		interim[netNodeUUID][bdUUID] = bd
	}

	// Arrange in final format.
	result := make(map[network.NetNodeUUID][]internal.BlockDevice, len(interim))
	for netNodeUUID, devices := range interim {
		bdList := make([]internal.BlockDevice, 0, len(devices))
		for bdUUID, bd := range devices {
			bdList = append(bdList, internal.BlockDevice{
				UUID:        bdUUID,
				BlockDevice: bd,
			})
		}
		result[netNodeUUID] = bdList
	}

	return result, nil
}
