// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

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
