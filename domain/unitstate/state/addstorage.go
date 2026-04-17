// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	corestorage "github.com/juju/juju/core/storage"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/errors"
)

type storageCount struct {
	StorageName string `db:"storage_name"`
	UnitUUID    string `db:"unit_uuid"`
	Count       uint32 `db:"count"`
}

type insertStorageFilesystem struct {
	FilesystemID     string `db:"filesystem_id"`
	LifeID           int    `db:"life_id"`
	ProvisionScopeID int    `db:"provision_scope_id"`
	UUID             string `db:"uuid"`
}

type insertStorageFilesystemAttachment struct {
	LifeID                int    `db:"life_id"`
	NetNodeUUID           string `db:"net_node_uuid"`
	ProvisionScopeID      int    `db:"provision_scope_id"`
	StorageFilesystemUUID string `db:"storage_filesystem_uuid"`
	UUID                  string `db:"uuid"`
}

type insertStorageFilesystemInstance struct {
	StorageFilesystemUUID string `db:"storage_filesystem_uuid"`
	StorageInstanceUUID   string `db:"storage_instance_uuid"`
}

type insertStorageFilesystemStatus struct {
	FilesystemUUID string    `db:"filesystem_uuid"`
	StatusID       int       `db:"status_id"`
	UpdateAt       time.Time `db:"updated_at"`
}

type insertStorageInstance struct {
	CharmName       string `db:"charm_name"`
	LifeID          int    `db:"life_id"`
	RequestSizeMiB  uint64 `db:"requested_size_mib"`
	StorageID       string `db:"storage_id"`
	StorageKindID   int    `db:"storage_kind_id"`
	StorageName     string `db:"storage_name"`
	StoragePoolUUID string `db:"storage_pool_uuid"`
	UUID            string `db:"uuid"`
}

type insertStorageInstanceAttachment struct {
	LifeID              int    `db:"life_id"`
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	UnitUUID            string `db:"unit_uuid"`
	UUID                string `db:"uuid"`
}

type insertStorageUnitOwner struct {
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	UnitUUID            string `db:"unit_uuid"`
}

type insertStorageVolume struct {
	LifeID           int    `db:"life_id"`
	UUID             string `db:"uuid"`
	VolumeID         string `db:"volume_id"`
	ProvisionScopeID int    `db:"provision_scope_id"`
}

type insertStorageVolumeAttachment struct {
	LifeID            int    `db:"life_id"`
	NetNodeUUID       string `db:"net_node_uuid"`
	ProvisionScopeID  int    `db:"provision_scope_id"`
	StorageVolumeUUID string `db:"storage_volume_uuid"`
	UUID              string `db:"uuid"`
}

type insertStorageVolumeInstance struct {
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	StorageVolumeUUID   string `db:"storage_volume_uuid"`
}

type insertStorageVolumeStatus struct {
	VolumeUUID string    `db:"volume_uuid"`
	StatusID   int       `db:"status_id"`
	UpdateAt   time.Time `db:"updated_at"`
}

type insertVolumeMachineOwner struct {
	MachineUUID string `db:"machine_uuid"`
	VolumeUUID  string `db:"volume_uuid"`
}

type insertFilesystemMachineOwner struct {
	MachineUUID    string `db:"machine_uuid"`
	FilesystemUUID string `db:"filesystem_uuid"`
}

func (st *State) addStorage(ctx context.Context, tx *sqlair.TX, arg internal.CommitHookChangesArg) error {
	for _, add := range arg.AddStorage {
		err := st.addStorageForUnit(ctx, tx, arg, add.StorageName.String(), add.Storage)
		if err != nil {
			return errors.Errorf("storage %q: %w", add.StorageName, err)
		}
	}
	return nil
}

func (st *State) addStorageForUnit(
	ctx context.Context,
	tx *sqlair.TX,
	arg internal.CommitHookChangesArg,
	storageName string,
	storageArg domainstorage.IAASUnitAddStorageArg,
) error {
	unitUUID := arg.UnitUUID

	currentCount, err := st.getUnitStorageCount(ctx, tx, unitUUID, storageName)
	if err != nil {
		return errors.Capture(err)
	}
	if currentCount > storageArg.CountLessThanEqual {
		return storageerrors.MaxStorageCountPreconditionFailed
	}

	_, err = st.insertUnitStorageInstances(ctx, tx, storageArg.StorageInstances)
	if err != nil {
		return errors.Errorf("inserting storage instances: %w", err)
	}

	err = st.insertUnitStorageAttachments(ctx, tx, unitUUID, storageArg.StorageToAttach)
	if err != nil {
		return errors.Errorf("creating storage attachments: %w", err)
	}

	err = st.insertUnitStorageOwnership(ctx, tx, unitUUID, storageArg.StorageToOwn)
	if err != nil {
		return errors.Errorf("inserting storage ownership: %w", err)
	}

	if arg.MachineUUID != nil {
		err = st.insertMachineVolumeOwnership(ctx, tx, *arg.MachineUUID, storageArg.VolumesToOwn)
		if err != nil {
			return errors.Errorf("inserting volume ownership: %w", err)
		}

		err = st.insertMachineFilesystemOwnership(ctx, tx, *arg.MachineUUID, storageArg.FilesystemsToOwn)
		if err != nil {
			return errors.Errorf("inserting filesystem ownership: %w", err)
		}
	}

	return nil
}

func (st *State) getUnitStorageCount(
	ctx context.Context, tx *sqlair.TX, unitUUID string, storageName string,
) (uint32, error) {
	stmt, err := st.Prepare(`
SELECT count(*) AS &storageCount.count
FROM   storage_instance si
JOIN   storage_unit_owner suo ON si.uuid = suo.storage_instance_uuid
WHERE  suo.unit_uuid = $storageCount.unit_uuid
AND    si.storage_name = $storageCount.storage_name
`, storageCount{})
	if err != nil {
		return 0, errors.Capture(err)
	}

	result := storageCount{
		StorageName: storageName,
		UnitUUID:    unitUUID,
	}
	err = tx.Query(ctx, stmt, result).Get(&result)
	if err != nil {
		return 0, errors.Capture(err)
	}
	return result.Count, nil
}

func machineStorageOwnership(
	storageInst []domainstorage.CreateUnitStorageInstanceArg,
) ([]domainstorage.FilesystemUUID, []domainstorage.VolumeUUID, error) {
	var filesystemsToOwn []domainstorage.FilesystemUUID
	var volumesToOwn []domainstorage.VolumeUUID

	for _, inst := range storageInst {
		var comp domainstorageprovisioning.StorageInstanceComposition
		if inst.Filesystem != nil {
			comp.FilesystemRequired = true
			comp.FilesystemProvisionScope = inst.Filesystem.ProvisionScope
		}
		if inst.Volume != nil {
			comp.VolumeRequired = true
			comp.VolumeProvisionScope = inst.Volume.ProvisionScope
		}

		scope, err := domainstorageprovisioning.CalculateStorageInstanceOwnershipScope(comp)
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
		if scope != domainstorageprovisioning.OwnershipScopeMachine {
			continue
		}

		if inst.Filesystem != nil {
			filesystemsToOwn = append(filesystemsToOwn, inst.Filesystem.UUID)
		}
		if inst.Volume != nil {
			volumesToOwn = append(volumesToOwn, inst.Volume.UUID)
		}
	}

	return filesystemsToOwn, volumesToOwn, nil
}

func (st *State) insertUnitStorageAttachments(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
	storageToAttach []domainstorage.CreateUnitStorageAttachmentArg,
) error {
	storageAttachmentArgs := makeInsertUnitStorageAttachmentArgs(unitUUID, storageToAttach)
	fsAttachmentArgs := st.makeInsertUnitFilesystemAttachmentArgs(storageToAttach)
	volAttachmentArgs := st.makeInsertUnitVolumeAttachmentArgs(storageToAttach)

	insertStorageAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_attachment (*) VALUES ($insertStorageInstanceAttachment.*)
`, insertStorageInstanceAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	insertFSAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_filesystem_attachment (*) VALUES ($insertStorageFilesystemAttachment.*)
`, insertStorageFilesystemAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	insertVolAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment (*) VALUES ($insertStorageVolumeAttachment.*)
`, insertStorageVolumeAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	if len(storageAttachmentArgs) != 0 {
		err = tx.Query(ctx, insertStorageAttachmentStmt, storageAttachmentArgs).Run()
		if err != nil {
			return errors.Capture(err)
		}
	}

	if len(fsAttachmentArgs) != 0 {
		err = tx.Query(ctx, insertFSAttachmentStmt, fsAttachmentArgs).Run()
		if err != nil {
			return errors.Capture(err)
		}
	}

	if len(volAttachmentArgs) != 0 {
		err = tx.Query(ctx, insertVolAttachmentStmt, volAttachmentArgs).Run()
		if err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}

func (st *State) insertUnitStorageInstances(
	ctx context.Context, tx *sqlair.TX, storageArgs []domainstorage.CreateUnitStorageInstanceArg,
) ([]string, error) {
	storageInstArgs, err := st.makeInsertUnitStorageInstanceArgs(ctx, tx, storageArgs)
	if err != nil {
		return nil, errors.Errorf("creating storage instance insert args: %w", err)
	}

	fsArgs, fsInstanceArgs, fsStatusArgs, err := st.makeInsertUnitFilesystemArgs(ctx, tx, storageArgs)
	if err != nil {
		return nil, errors.Errorf("creating filesystem insert args: %w", err)
	}

	vArgs, vInstanceArgs, vStatusArgs, err := st.makeInsertUnitVolumeArgs(ctx, tx, storageArgs)
	if err != nil {
		return nil, errors.Errorf("creating volume insert args: %w", err)
	}

	insertStorageInstStmt, err := st.Prepare(`
INSERT INTO storage_instance (*) VALUES ($insertStorageInstance.*)
`, insertStorageInstance{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageFilesystemStmt, err := st.Prepare(`
INSERT INTO storage_filesystem (*) VALUES ($insertStorageFilesystem.*)
`, insertStorageFilesystem{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageFilesystemInstStmt, err := st.Prepare(`
INSERT INTO storage_instance_filesystem (*) VALUES ($insertStorageFilesystemInstance.*)
`, insertStorageFilesystemInstance{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageFilesystemStatusStmt, err := st.Prepare(`
INSERT INTO storage_filesystem_status (*) VALUES ($insertStorageFilesystemStatus.*)
`, insertStorageFilesystemStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageVolumeStmt, err := st.Prepare(`
INSERT INTO storage_volume (*) VALUES ($insertStorageVolume.*)
`, insertStorageVolume{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageVolumeInstStmt, err := st.Prepare(`
INSERT INTO storage_instance_volume (*) VALUES ($insertStorageVolumeInstance.*)
`, insertStorageVolumeInstance{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageVolumeStatusStmt, err := st.Prepare(`
INSERT INTO storage_volume_status (*) VALUES ($insertStorageVolumeStatus.*)
`, insertStorageVolumeStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(storageInstArgs) != 0 {
		err = tx.Query(ctx, insertStorageInstStmt, storageInstArgs).Run()
		if err != nil {
			return nil, errors.Errorf("creating %d storage instance(s): %w", len(storageInstArgs), err)
		}
	}

	if len(fsArgs) != 0 {
		err = tx.Query(ctx, insertStorageFilesystemStmt, fsArgs).Run()
		if err != nil {
			return nil, errors.Errorf("creating %d storage filesystems: %w", len(fsArgs), err)
		}
	}

	if len(fsInstanceArgs) != 0 {
		err = tx.Query(ctx, insertStorageFilesystemInstStmt, fsInstanceArgs).Run()
		if err != nil {
			return nil, errors.Errorf("setting storage filesystem instance relationships: %w", err)
		}
	}

	if len(fsStatusArgs) != 0 {
		err = tx.Query(ctx, insertStorageFilesystemStatusStmt, fsStatusArgs).Run()
		if err != nil {
			return nil, errors.Errorf("setting storage filesystem status: %w", err)
		}
	}

	if len(vArgs) != 0 {
		err = tx.Query(ctx, insertStorageVolumeStmt, vArgs).Run()
		if err != nil {
			return nil, errors.Errorf("creating %d storage volumes: %w", len(vArgs), err)
		}
	}

	if len(vInstanceArgs) != 0 {
		err = tx.Query(ctx, insertStorageVolumeInstStmt, vInstanceArgs).Run()
		if err != nil {
			return nil, errors.Errorf("setting storage volume instance relationships: %w", err)
		}
	}

	if len(vStatusArgs) != 0 {
		err = tx.Query(ctx, insertStorageVolumeStatusStmt, vStatusArgs).Run()
		if err != nil {
			return nil, errors.Errorf("setting storage volume status: %w", err)
		}
	}

	result := make([]string, 0, len(storageInstArgs))
	for _, inst := range storageInstArgs {
		result = append(result, inst.StorageID)
	}
	return result, nil
}

func (st *State) insertUnitStorageOwnership(
	ctx context.Context, tx *sqlair.TX, unitUUID string, storageToOwn []domainstorage.StorageInstanceUUID,
) error {
	args := makeInsertUnitStorageOwnerArgs(ctx, unitUUID, storageToOwn)
	if len(args) == 0 {
		return nil
	}

	stmt, err := st.Prepare(`
INSERT INTO storage_unit_owner (*) VALUES ($insertStorageUnitOwner.*)
`, insertStorageUnitOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, args).Run()
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (st *State) insertMachineVolumeOwnership(
	ctx context.Context, tx *sqlair.TX, machineUUID string, volumesToOwn []domainstorage.VolumeUUID,
) error {
	args := makeInsertMachineVolumeOwnerArgs(machineUUID, volumesToOwn)
	if len(args) == 0 {
		return nil
	}

	stmt, err := st.Prepare(`
INSERT INTO machine_volume (*) VALUES ($insertVolumeMachineOwner.*)
`, insertVolumeMachineOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, args).Run()
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (st *State) insertMachineFilesystemOwnership(
	ctx context.Context, tx *sqlair.TX, machineUUID string, filesystemsToOwn []domainstorage.FilesystemUUID,
) error {
	args := makeInsertMachineFilesystemOwnerArgs(machineUUID, filesystemsToOwn)
	if len(args) == 0 {
		return nil
	}

	stmt, err := st.Prepare(`
INSERT INTO machine_filesystem (*) VALUES ($insertFilesystemMachineOwner.*)
`, insertFilesystemMachineOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, args).Run()
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (st *State) makeInsertUnitFilesystemArgs(
	ctx context.Context, tx *sqlair.TX, args []domainstorage.CreateUnitStorageInstanceArg,
) (
	[]insertStorageFilesystem,
	[]insertStorageFilesystemInstance,
	[]insertStorageFilesystemStatus,
	error,
) {
	argIndexes := make([]int, 0, len(args))
	for i, arg := range args {
		if arg.Filesystem == nil {
			continue
		}
		argIndexes = append(argIndexes, i)
	}
	if len(argIndexes) == 0 {
		return nil, nil, nil, nil
	}

	fsIDs, err := sequencestate.NextNValues(
		ctx, st, tx, uint64(len(argIndexes)),
		domainstorage.FilesystemSequenceNamespace,
	)
	if err != nil {
		return nil, nil, nil, errors.Errorf(
			"generating %d new filesystem ids: %w", len(argIndexes), err,
		)
	}

	fsStatus, err := status.EncodeStorageFilesystemStatus(status.StorageFilesystemStatusTypePending)
	if err != nil {
		return nil, nil, nil, errors.Errorf("encoding filesystem status pending: %w", err)
	}

	fsRows := make([]insertStorageFilesystem, 0, len(argIndexes))
	fsInstanceRows := make([]insertStorageFilesystemInstance, 0, len(argIndexes))
	fsStatusRows := make([]insertStorageFilesystemStatus, 0, len(argIndexes))
	statusTime := time.Now().UTC()
	for i, argIndex := range argIndexes {
		instArg := args[argIndex]
		fsRows = append(fsRows, insertStorageFilesystem{
			FilesystemID:     fmt.Sprintf("%d", fsIDs[i]),
			LifeID:           0, // Alive.
			UUID:             instArg.Filesystem.UUID.String(),
			ProvisionScopeID: int(instArg.Filesystem.ProvisionScope),
		})
		fsInstanceRows = append(fsInstanceRows, insertStorageFilesystemInstance{
			StorageInstanceUUID:   instArg.UUID.String(),
			StorageFilesystemUUID: instArg.Filesystem.UUID.String(),
		})
		fsStatusRows = append(fsStatusRows, insertStorageFilesystemStatus{
			FilesystemUUID: instArg.Filesystem.UUID.String(),
			StatusID:       fsStatus,
			UpdateAt:       statusTime,
		})
	}

	return fsRows, fsInstanceRows, fsStatusRows, nil
}

func (st *State) makeInsertUnitFilesystemAttachmentArgs(
	args []domainstorage.CreateUnitStorageAttachmentArg,
) []insertStorageFilesystemAttachment {
	var result []insertStorageFilesystemAttachment
	for _, arg := range args {
		if arg.FilesystemAttachment == nil {
			continue
		}

		result = append(result, insertStorageFilesystemAttachment{
			LifeID:                0, // Alive.
			NetNodeUUID:           arg.FilesystemAttachment.NetNodeUUID.String(),
			ProvisionScopeID:      int(arg.FilesystemAttachment.ProvisionScope),
			StorageFilesystemUUID: arg.FilesystemAttachment.FilesystemUUID.String(),
			UUID:                  arg.FilesystemAttachment.UUID.String(),
		})
	}

	return result
}

func (st *State) makeInsertUnitStorageInstanceArgs(
	ctx context.Context, tx *sqlair.TX, args []domainstorage.CreateUnitStorageInstanceArg,
) ([]insertStorageInstance, error) {
	result := make([]insertStorageInstance, 0, len(args))

	for _, arg := range args {
		id, err := sequencestate.NextValue(ctx, st, tx, domainstorage.StorageInstanceSequenceNamespace)
		if err != nil {
			return nil, errors.Errorf("creating unique storage instance id: %w", err)
		}
		storageID := corestorage.MakeID(corestorage.Name(arg.Name), id).String()

		result = append(result, insertStorageInstance{
			CharmName:       arg.CharmName,
			LifeID:          0, // Alive.
			RequestSizeMiB:  arg.RequestSizeMiB,
			StorageID:       storageID,
			StorageKindID:   int(arg.Kind),
			StorageName:     arg.Name.String(),
			StoragePoolUUID: arg.StoragePoolUUID.String(),
			UUID:            arg.UUID.String(),
		})
	}

	return result, nil
}

func (st *State) makeInsertUnitVolumeArgs(
	ctx context.Context, tx *sqlair.TX, args []domainstorage.CreateUnitStorageInstanceArg,
) (
	[]insertStorageVolume,
	[]insertStorageVolumeInstance,
	[]insertStorageVolumeStatus,
	error,
) {
	argIndexes := make([]int, 0, len(args))
	for i, arg := range args {
		if arg.Volume == nil {
			continue
		}
		argIndexes = append(argIndexes, i)
	}
	if len(argIndexes) == 0 {
		return nil, nil, nil, nil
	}

	volumeIDs, err := sequencestate.NextNValues(
		ctx, st, tx, uint64(len(argIndexes)),
		domainstorage.VolumeSequenceNamespace,
	)
	if err != nil {
		return nil, nil, nil, errors.Errorf("generating %d new volume ids: %w", len(argIndexes), err)
	}

	volumeStatus, err := status.EncodeStorageVolumeStatus(status.StorageVolumeStatusTypePending)
	if err != nil {
		return nil, nil, nil, errors.Errorf("encoding volume status pending: %w", err)
	}

	volumeRows := make([]insertStorageVolume, 0, len(argIndexes))
	volumeInstanceRows := make([]insertStorageVolumeInstance, 0, len(argIndexes))
	volumeStatusRows := make([]insertStorageVolumeStatus, 0, len(argIndexes))
	statusTime := time.Now().UTC()
	for i, argIndex := range argIndexes {
		instArg := args[argIndex]
		volumeRows = append(volumeRows, insertStorageVolume{
			VolumeID:         fmt.Sprintf("%d", volumeIDs[i]),
			LifeID:           0, // Alive.
			UUID:             instArg.Volume.UUID.String(),
			ProvisionScopeID: int(instArg.Volume.ProvisionScope),
		})
		volumeInstanceRows = append(volumeInstanceRows, insertStorageVolumeInstance{
			StorageInstanceUUID: instArg.UUID.String(),
			StorageVolumeUUID:   instArg.Volume.UUID.String(),
		})
		volumeStatusRows = append(volumeStatusRows, insertStorageVolumeStatus{
			VolumeUUID: instArg.Volume.UUID.String(),
			StatusID:   volumeStatus,
			UpdateAt:   statusTime,
		})
	}

	return volumeRows, volumeInstanceRows, volumeStatusRows, nil
}

func (st *State) makeInsertUnitVolumeAttachmentArgs(
	args []domainstorage.CreateUnitStorageAttachmentArg,
) []insertStorageVolumeAttachment {
	var result []insertStorageVolumeAttachment
	for _, arg := range args {
		if arg.VolumeAttachment == nil {
			continue
		}

		result = append(result, insertStorageVolumeAttachment{
			LifeID:            0, // Alive.
			NetNodeUUID:       arg.VolumeAttachment.NetNodeUUID.String(),
			ProvisionScopeID:  int(arg.VolumeAttachment.ProvisionScope),
			StorageVolumeUUID: arg.VolumeAttachment.VolumeUUID.String(),
			UUID:              arg.VolumeAttachment.UUID.String(),
		})
	}

	return result
}

func makeInsertUnitStorageAttachmentArgs(
	unitUUID string, storageToAttach []domainstorage.CreateUnitStorageAttachmentArg,
) []insertStorageInstanceAttachment {
	return transform.Slice(storageToAttach,
		func(sa domainstorage.CreateUnitStorageAttachmentArg) insertStorageInstanceAttachment {
			return insertStorageInstanceAttachment{
				LifeID:              0, // Alive.
				StorageInstanceUUID: sa.StorageInstanceUUID.String(),
				UnitUUID:            unitUUID,
				UUID:                sa.UUID.String(),
			}
		})
}

func makeInsertUnitStorageOwnerArgs(
	_ context.Context, unitUUID string, storageToOwn []domainstorage.StorageInstanceUUID,
) []insertStorageUnitOwner {
	return transform.Slice(storageToOwn, func(instUUID domainstorage.StorageInstanceUUID) insertStorageUnitOwner {
		return insertStorageUnitOwner{
			StorageInstanceUUID: instUUID.String(),
			UnitUUID:            unitUUID,
		}
	})
}

func makeInsertMachineVolumeOwnerArgs(
	machineUUID string, volumesToOwn []domainstorage.VolumeUUID,
) []insertVolumeMachineOwner {
	return transform.Slice(volumesToOwn, func(uuid domainstorage.VolumeUUID) insertVolumeMachineOwner {
		return insertVolumeMachineOwner{
			MachineUUID: machineUUID,
			VolumeUUID:  uuid.String(),
		}
	})
}

func makeInsertMachineFilesystemOwnerArgs(
	machineUUID string, filesystemsToOwn []domainstorage.FilesystemUUID,
) []insertFilesystemMachineOwner {
	return transform.Slice(filesystemsToOwn, func(uuid domainstorage.FilesystemUUID) insertFilesystemMachineOwner {
		return insertFilesystemMachineOwner{
			MachineUUID:    machineUUID,
			FilesystemUUID: uuid.String(),
		}
	})
}
