// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

const (
	statusNotFound = errors.ConstError("status not found")
)

// SetFilesystemStatus saves the given filesystem status, overwriting any
// current status data. The following errors can be expected:
// - [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
func (st *ModelState) SetFilesystemStatus(
	ctx context.Context,
	filesystemUUID storage.FilesystemUUID,
	sts status.StatusInfo[status.StorageFilesystemStatusType],
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get current status.
		currentStatus, isProvisioned, err := st.getFilesystemProvisioningStatus(ctx, tx, filesystemUUID)
		if err != nil && !errors.Is(err, statusNotFound) {
			return errors.Errorf("getting current filesystem status: %w", err)
		}
		if err == nil {
			// Check we can transition from current status to the new status.
			err = status.FilesystemStatusTransitionValid(currentStatus, isProvisioned, sts)
			if err != nil {
				return errors.Capture(err)
			}
		}

		return st.updateFilesystemStatus(ctx, tx, filesystemUUID, sts)
	})
	if err != nil {
		return errors.Errorf("updating filesystem status for %q: %w", filesystemUUID, err)
	}
	return nil
}

func (st *ModelState) GetAllAttachedBlockDeviceLinks(
	ctx context.Context,
) (map[blockdevice.BlockDeviceUUID][]string, error) {
	// TODO(tlm): implement
	//
	return map[blockdevice.BlockDeviceUUID][]string{}, nil
}

// getFilesystemstatus gets the status of the given filesystem
// and a bool indicating if it is provisioned.
// The following errors can be expected:
// - [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
func (st *ModelState) getFilesystemProvisioningStatus(
	ctx context.Context,
	tx *sqlair.TX,
	uuid storage.FilesystemUUID,
) (status.StorageFilesystemStatusType, bool, error) {
	id := filesystemUUID{
		FilesystemUUID: uuid.String(),
	}
	var sts storageProvisioningStatusInfo

	stmt, err := st.Prepare(`
SELECT    &storageProvisioningStatusInfo.*
FROM      storage_filesystem sf
LEFT JOIN storage_filesystem_status sfs ON sf.uuid = sfs.filesystem_uuid
LEFT JOIN storage_instance_filesystem sif ON sf.uuid = sif.storage_filesystem_uuid
WHERE     sf.uuid = $filesystemUUID.uuid
`, id, sts)
	if err != nil {
		return -1, false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, id).Get(&sts)
	if errors.Is(err, sqlair.ErrNoRows) {
		return -1, false, storageerrors.FilesystemNotFound
	} else if err != nil {
		return -1, false, errors.Capture(err)
	}
	if !sts.StatusID.Valid {
		return -1, false, statusNotFound
	}
	statusType, err := status.DecodeStorageFilesystemStatus(int(sts.StatusID.Int16))
	if err != nil {
		return -1, false, errors.Capture(err)
	}
	isProvisioned := sts.StorageInstanceUUID.Valid
	return statusType, isProvisioned, nil
}

// ImportFilesystemStatus sets the given filesystem status.
// The following errors can be expected:
// - [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
func (st *ModelState) ImportFilesystemStatus(
	ctx context.Context,
	filesystemUUID storage.FilesystemUUID,
	sts status.StatusInfo[status.StorageFilesystemStatusType],
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.updateFilesystemStatus(ctx, tx, filesystemUUID, sts)
	})
}

// GetFilesystemUUIDByID returns the UUID for the given filesystem ID.
// It can return the following errors:
//   - [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
func (st *ModelState) GetFilesystemUUIDByID(
	ctx context.Context,
	id string,
) (storage.FilesystemUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	arg := filesystemUUIDID{ID: id}
	stmt, err := st.Prepare(`
SELECT &filesystemUUIDID.uuid
FROM   storage_filesystem
WHERE  filesystem_id = $filesystemUUIDID.filesystem_id
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, arg).Get(&arg)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("filesystem %q not found", id).Add(storageerrors.FilesystemNotFound)
		} else if err != nil {
			return errors.Errorf("getting filesystem UUID for %q: %w", id, err)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return storage.FilesystemUUID(arg.UUID), nil
}

func (st *ModelState) updateFilesystemStatus(
	ctx context.Context,
	tx *sqlair.TX,
	filesystemUUID storage.FilesystemUUID,
	sts status.StatusInfo[status.StorageFilesystemStatusType],
) error {
	statusID, err := status.EncodeStorageFilesystemStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := filesystemStatusInfo{
		FilesystemUUID: filesystemUUID.String(),
		StatusID:       statusID,
		Message:        sts.Message,
		UpdatedAt:      sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO storage_filesystem_status (*) VALUES ($filesystemStatusInfo.*)
ON CONFLICT(filesystem_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, statusInfo).Run()
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// SetVolumeStatus saves the given volume status, overwriting any
// current status data. The following errors can be expected:
// - [storageerrors.VolumeNotFound] if the volume doesn't exist.
func (st *ModelState) SetVolumeStatus(
	ctx context.Context,
	volumeUUID storage.VolumeUUID,
	sts status.StatusInfo[status.StorageVolumeStatusType],
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get current status.
		currentStatus, isProvisioned, err := st.getVolumeProvisioningStatus(ctx, tx, volumeUUID)
		if err != nil && !errors.Is(err, statusNotFound) {
			return errors.Errorf("getting current volume status: %w", err)
		}
		if err == nil {
			// Check we can transition from current status to the new status.
			err = status.VolumeStatusTransitionValid(currentStatus, isProvisioned, sts)
			if err != nil {
				return errors.Capture(err)
			}
		}

		return st.updateVolumeStatus(ctx, tx, volumeUUID, sts)
	})
	if err != nil {
		return errors.Errorf("updating volume status for %q: %w", volumeUUID, err)
	}
	return nil
}

// getVolumeProvisioningStatus gets the status of the given volume
// and a bool indicating if it is provisioned.
// The following errors can be expected:
// - [storageerrors.VolumeNotFound] if the volume doesn't exist.
func (st *ModelState) getVolumeProvisioningStatus(
	ctx context.Context,
	tx *sqlair.TX,
	uuid storage.VolumeUUID,
) (status.StorageVolumeStatusType, bool, error) {
	id := volumeUUID{
		VolumeUUID: uuid.String(),
	}
	var sts storageProvisioningStatusInfo

	stmt, err := st.Prepare(`
SELECT    &storageProvisioningStatusInfo.*
FROM      storage_volume sv
LEFT JOIN storage_volume_status svs ON sv.uuid = svs.volume_uuid
LEFT JOIN storage_instance_volume siv ON sv.uuid = siv.storage_volume_uuid
WHERE     sv.uuid = $volumeUUID.uuid
`, id, sts)
	if err != nil {
		return -1, false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, id).Get(&sts)
	if errors.Is(err, sqlair.ErrNoRows) {
		return -1, false, storageerrors.VolumeNotFound
	} else if err != nil {
		return -1, false, errors.Capture(err)
	}
	if !sts.StatusID.Valid {
		return -1, false, statusNotFound
	}
	statusType, err := status.DecodeStorageVolumeStatus(int(sts.StatusID.Int16))
	if err != nil {
		return -1, false, errors.Capture(err)
	}
	isProvisioned := sts.StorageInstanceUUID.Valid
	return statusType, isProvisioned, nil
}

// ImportVolumeStatus sets the given volume status.
// The following errors can be expected:
// - [storageerrors.VolumeNotFound] if the volume doesn't exist.
func (st *ModelState) ImportVolumeStatus(
	ctx context.Context,
	volumeUUID storage.VolumeUUID,
	sts status.StatusInfo[status.StorageVolumeStatusType],
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.updateVolumeStatus(ctx, tx, volumeUUID, sts)
	})
}

// GetVolumeUUIDByID returns the UUID for the given volume ID.
// It can return the following errors:
//   - [storageerrors.VolumeNotFound] if the volume doesn't exist.
func (st *ModelState) GetVolumeUUIDByID(
	ctx context.Context,
	id string,
) (storage.VolumeUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	arg := volumeUUIDID{ID: id}
	stmt, err := st.Prepare(`
SELECT &volumeUUIDID.uuid
FROM   storage_volume
WHERE  volume_id = $volumeUUIDID.volume_id
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, arg).Get(&arg)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("volume %q not found", id).Add(storageerrors.VolumeNotFound)
		} else if err != nil {
			return errors.Errorf("getting volume UUID for %q: %w", id, err)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return storage.VolumeUUID(arg.UUID), nil
}

func (st *ModelState) updateVolumeStatus(
	ctx context.Context,
	tx *sqlair.TX,
	volumeUUID storage.VolumeUUID,
	sts status.StatusInfo[status.StorageVolumeStatusType],
) error {
	statusID, err := status.EncodeStorageVolumeStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := volumeStatusInfo{
		VolumeUUID: volumeUUID.String(),
		StatusID:   statusID,
		Message:    sts.Message,
		UpdatedAt:  sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO storage_volume_status (*) VALUES ($volumeStatusInfo.*)
ON CONFLICT(volume_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, statusInfo).Run()
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// GetStorageInstances returns the specified storage instances if they exist.
func (st *ModelState) GetStorageInstances(
	ctx context.Context, uuids []storage.StorageInstanceUUID,
) ([]status.StorageInstance, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ids := IDs[entityUUIDs](uuids)

	stmt, err := st.Prepare(`
SELECT &storageInstanceStatusDetails.* FROM (
    SELECT    si.uuid,
              si.storage_id,
              si.life_id,
              si.storage_kind_id,
              si.storage_name,
              u.name AS owner_unit_name,
              svs.status_id AS volume_status_id,
              svs.message AS volume_status_message,
              svs.updated_at AS volume_status_updated_at,
              sfs.status_id AS filesystem_status_id,
              sfs.message AS filesystem_status_message,
              sfs.updated_at AS filesystem_status_updated_at
    FROM      storage_instance si
    LEFT JOIN storage_unit_owner suo ON si.uuid=suo.storage_instance_uuid
    LEFT JOIN unit u ON suo.unit_uuid=u.uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid=siv.storage_instance_uuid
    LEFT JOIN storage_volume_status svs ON siv.storage_volume_uuid=svs.volume_uuid
    LEFT JOIN storage_instance_filesystem sif ON si.uuid=sif.storage_instance_uuid
    LEFT JOIN storage_filesystem_status sfs ON sif.storage_filesystem_uuid=sfs.filesystem_uuid
    WHERE     si.uuid IN ($entityUUIDs[:])
)
`, storageInstanceStatusDetails{}, ids)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []storageInstanceStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ids).GetAll(&out)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No rows just means that no StorageInstances exist in the model.
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceOrErr(out, func(v storageInstanceStatusDetails) (status.StorageInstance, error) {
		var owner *unit.Name
		if v.OwnerUnitName.Valid {
			n := unit.Name(v.OwnerUnitName.String)
			owner = &n
		}
		var fsStatus status.StatusInfo[status.StorageFilesystemStatusType]
		if v.FilesystemStatusID.Valid {
			statusValue, err := status.DecodeStorageFilesystemStatus(
				v.FilesystemStatusID.V)
			if err != nil {
				return status.StorageInstance{}, errors.Capture(err)
			}
			fsStatus = status.StatusInfo[status.StorageFilesystemStatusType]{
				Status:  statusValue,
				Message: v.FilesystemStatusMessage,
				Since:   v.FilesystemStatusUpdatedAt,
			}
		}
		var volStatus status.StatusInfo[status.StorageVolumeStatusType]
		if v.FilesystemStatusID.Valid {
			statusValue, err := status.DecodeStorageVolumeStatus(
				v.VolumeStatusID.V)
			if err != nil {
				return status.StorageInstance{}, errors.Capture(err)
			}
			volStatus = status.StatusInfo[status.StorageVolumeStatusType]{
				Status:  statusValue,
				Message: v.VolumeStatusMessage,
				Since:   v.VolumeStatusUpdatedAt,
			}
		}
		return status.StorageInstance{
			UUID:             storage.StorageInstanceUUID(v.UUID),
			ID:               v.ID,
			Kind:             storage.StorageKind(v.KindID),
			Name:             v.StorageName,
			Life:             life.Life(v.LifeID),
			Owner:            owner,
			FilesystemStatus: fsStatus,
			VolumeStatus:     volStatus,
		}, nil
	})
}

// GetAllStorageInstances returns all the storage instances for this model.
func (st *ModelState) GetAllStorageInstances(
	ctx context.Context,
) ([]status.StorageInstance, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &storageInstanceStatusDetails.* FROM (
    SELECT    si.uuid,
    		  si.storage_id,
              si.life_id,
              si.storage_kind_id,
              si.storage_name,
              u.name AS owner_unit_name,
              svs.status_id AS volume_status_id,
              svs.message AS volume_status_message,
              svs.updated_at AS volume_status_updated_at,
              sfs.status_id AS filesystem_status_id,
              sfs.message AS filesystem_status_message,
              sfs.updated_at AS filesystem_status_updated_at
    FROM      storage_instance si
    LEFT JOIN storage_unit_owner suo ON si.uuid=suo.storage_instance_uuid
    LEFT JOIN unit u ON suo.unit_uuid=u.uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid=siv.storage_instance_uuid
    LEFT JOIN storage_volume_status svs ON siv.storage_volume_uuid=svs.volume_uuid
    LEFT JOIN storage_instance_filesystem sif ON si.uuid=sif.storage_instance_uuid
    LEFT JOIN storage_filesystem_status sfs ON sif.storage_filesystem_uuid=sfs.filesystem_uuid
)
`, storageInstanceStatusDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []storageInstanceStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&out)
		if errors.Is(err, sql.ErrNoRows) {
			// No rows just means that no StorageInstances exist in the model.
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceOrErr(out, func(v storageInstanceStatusDetails) (status.StorageInstance, error) {
		var owner *unit.Name
		if v.OwnerUnitName.Valid {
			n := unit.Name(v.OwnerUnitName.String)
			owner = &n
		}
		var fsStatus status.StatusInfo[status.StorageFilesystemStatusType]
		if v.FilesystemStatusID.Valid {
			statusValue, err := status.DecodeStorageFilesystemStatus(
				v.FilesystemStatusID.V)
			if err != nil {
				return status.StorageInstance{}, errors.Capture(err)
			}
			fsStatus = status.StatusInfo[status.StorageFilesystemStatusType]{
				Status:  statusValue,
				Message: v.FilesystemStatusMessage,
				Since:   v.FilesystemStatusUpdatedAt,
			}
		}
		var volStatus status.StatusInfo[status.StorageVolumeStatusType]
		if v.VolumeStatusID.Valid {
			statusValue, err := status.DecodeStorageVolumeStatus(
				v.VolumeStatusID.V)
			if err != nil {
				return status.StorageInstance{}, errors.Capture(err)
			}
			volStatus = status.StatusInfo[status.StorageVolumeStatusType]{
				Status:  statusValue,
				Message: v.VolumeStatusMessage,
				Since:   v.VolumeStatusUpdatedAt,
			}
		}
		return status.StorageInstance{
			UUID:             storage.StorageInstanceUUID(v.UUID),
			ID:               v.ID,
			Kind:             storage.StorageKind(v.KindID),
			Name:             v.StorageName,
			Life:             life.Life(v.LifeID),
			Owner:            owner,
			FilesystemStatus: fsStatus,
			VolumeStatus:     volStatus,
		}, nil
	})
}

// GetStorageInstanceAttachments returns the specified storage instance
// attachments if they exist.
func (st *ModelState) GetStorageInstanceAttachments(
	ctx context.Context, uuids []storage.StorageInstanceUUID,
) ([]status.StorageAttachment, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ids := IDs[entityUUIDs](uuids)

	stmt, err := st.Prepare(`
SELECT &storageAttachmentStatusDetails.* FROM (
    SELECT    sa.storage_instance_uuid, sa.life_id,
              u.name AS unit_name,
              m.name AS machine_name,
              sfa.mount_point AS filesystem_mount_point,
              sva.block_device_uuid AS volume_block_device_uuid
    FROM      storage_attachment sa
    LEFT JOIN unit u ON sa.unit_uuid=u.uuid
    LEFT JOIN machine m ON u.net_node_uuid=m.net_node_uuid
    LEFT JOIN storage_instance_volume siv ON sa.storage_instance_uuid=siv.storage_instance_uuid
    LEFT JOIN storage_volume_attachment sva ON siv.storage_volume_uuid=sva.storage_volume_uuid AND
                                               u.net_node_uuid=sva.net_node_uuid
    LEFT JOIN storage_instance_filesystem sif ON sa.storage_instance_uuid=sif.storage_instance_uuid
    LEFT JOIN storage_filesystem_attachment sfa ON sif.storage_filesystem_uuid=sfa.storage_filesystem_uuid AND
                                                   u.net_node_uuid=sfa.net_node_uuid
    WHERE     sa.storage_instance_uuid IN ($entityUUIDs[:])
)
`, storageAttachmentStatusDetails{}, ids)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []storageAttachmentStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ids).GetAll(&out)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(out, func(v storageAttachmentStatusDetails) status.StorageAttachment {
		var machineName *machine.Name
		if v.MachineName.Valid {
			m := machine.Name(v.MachineName.String)
			machineName = &m
		}
		var filesystemMountPoint *string
		if v.FilesystemMountPoint.Valid {
			filesystemMountPoint = &v.FilesystemMountPoint.String
		}
		var volumeBlockDevice *blockdevice.BlockDeviceUUID
		if v.VolumeBlockDeviceUUID.Valid {
			blockDeviceUUID := blockdevice.BlockDeviceUUID(
				v.VolumeBlockDeviceUUID.String)
			volumeBlockDevice = &blockDeviceUUID
		}
		return status.StorageAttachment{
			StorageInstanceUUID:  storage.StorageInstanceUUID(v.StorageInstanceUUID),
			Life:                 life.Life(v.LifeID),
			Unit:                 unit.Name(v.UnitName),
			Machine:              machineName,
			FilesystemMountPoint: filesystemMountPoint,
			VolumeBlockDevice:    volumeBlockDevice,
		}
	}), nil
}

// GetAllStorageInstanceAttachments returns all the storage instance
// attachments for this model.
func (st *ModelState) GetAllStorageInstanceAttachments(
	ctx context.Context,
) ([]status.StorageAttachment, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &storageAttachmentStatusDetails.* FROM (
    SELECT    sa.storage_instance_uuid, sa.life_id,
              u.name AS unit_name,
              m.name AS machine_name,
              sfa.mount_point AS filesystem_mount_point,
              sva.block_device_uuid AS volume_block_device_uuid
    FROM      storage_attachment sa
    LEFT JOIN unit u ON sa.unit_uuid=u.uuid
    LEFT JOIN machine m ON u.net_node_uuid=m.net_node_uuid
    LEFT JOIN storage_instance_volume siv ON sa.storage_instance_uuid=siv.storage_instance_uuid
    LEFT JOIN storage_volume_attachment sva ON siv.storage_volume_uuid=sva.storage_volume_uuid AND
                                               u.net_node_uuid=sva.net_node_uuid
    LEFT JOIN storage_instance_filesystem sif ON sa.storage_instance_uuid=sif.storage_instance_uuid
    LEFT JOIN storage_filesystem_attachment sfa ON sif.storage_filesystem_uuid=sfa.storage_filesystem_uuid AND
                                                   u.net_node_uuid=sfa.net_node_uuid
)
`, storageAttachmentStatusDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []storageAttachmentStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&out)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(out, func(v storageAttachmentStatusDetails) status.StorageAttachment {
		var machineName *machine.Name
		if v.MachineName.Valid {
			m := machine.Name(v.MachineName.String)
			machineName = &m
		}
		var filesystemMountPoint *string
		if v.FilesystemMountPoint.Valid {
			filesystemMountPoint = &v.FilesystemMountPoint.String
		}
		var volumeBlockDevice *blockdevice.BlockDeviceUUID
		if v.VolumeBlockDeviceUUID.Valid {
			blockDeviceUUID := blockdevice.BlockDeviceUUID(
				v.VolumeBlockDeviceUUID.String)
			volumeBlockDevice = &blockDeviceUUID
		}
		return status.StorageAttachment{
			StorageInstanceUUID:  storage.StorageInstanceUUID(v.StorageInstanceUUID),
			Life:                 life.Life(v.LifeID),
			Unit:                 unit.Name(v.UnitName),
			Machine:              machineName,
			FilesystemMountPoint: filesystemMountPoint,
			VolumeBlockDevice:    volumeBlockDevice,
		}
	}), nil
}

// GetFilesystems returns the specified filesystems if they exist.
func (st *ModelState) GetFilesystems(
	ctx context.Context, uuids []storage.FilesystemUUID,
) ([]status.Filesystem, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ids := IDs[entityUUIDs](uuids)

	stmt, err := st.Prepare(`
SELECT    (sf.uuid, sf.filesystem_id, sf.life_id, sf.provider_id, sf.size_mib) AS (&filesystemStatusDetails.*),
          (sfs.status_id, sfs.message, sfs.updated_at) AS (&filesystemStatusDetails.*),
          (si.storage_id, sv.volume_id) AS (&filesystemStatusDetails.*),
          si.uuid AS &filesystemStatusDetails.storage_instance_uuid
FROM      storage_filesystem sf
LEFT JOIN storage_filesystem_status sfs ON sfs.filesystem_uuid=sf.uuid
LEFT JOIN storage_instance_filesystem sif ON sf.uuid=sif.storage_filesystem_uuid
LEFT JOIN storage_instance si ON si.uuid=sif.storage_instance_uuid
LEFT JOIN storage_instance_volume siv ON siv.storage_instance_uuid=si.uuid
LEFT JOIN storage_volume sv ON sv.uuid=siv.storage_volume_uuid
WHERE     sf.uuid IN ($entityUUIDs[:])
`, filesystemStatusDetails{}, ids)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []filesystemStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ids).GetAll(&out)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceOrErr(out, func(v filesystemStatusDetails) (status.Filesystem, error) {
		statusValue, err := status.DecodeStorageFilesystemStatus(v.StatusID)
		if err != nil {
			return status.Filesystem{}, errors.Capture(err)
		}
		var volumeID *string
		if v.VolumeID.Valid {
			volumeID = &v.VolumeID.String
		}
		var storageInstanceUUID *storage.StorageInstanceUUID
		if v.StorageInstanceUUID.Valid {
			siUUID := storage.StorageInstanceUUID(v.StorageInstanceUUID.String)
			storageInstanceUUID = &siUUID
		}
		return status.Filesystem{
			UUID: storage.FilesystemUUID(v.UUID),
			ID:   v.ID,
			Life: life.Life(v.LifeID),
			Status: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status:  statusValue,
				Message: v.Message,
				Since:   v.UpdatedAt,
			},
			StorageUUID: storageInstanceUUID,
			StorageID:   v.StorageID,
			VolumeID:    volumeID,
			ProviderID:  v.ProviderID,
			SizeMiB:     v.SizeMiB,
		}, nil
	})
}

// GetAllFilesystems returns all the filesystems for this model.
func (st *ModelState) GetAllFilesystems(
	ctx context.Context,
) ([]status.Filesystem, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT    (sf.uuid, sf.filesystem_id, sf.life_id, sf.provider_id, sf.size_mib) AS (&filesystemStatusDetails.*),
          (sfs.status_id, sfs.message, sfs.updated_at) AS (&filesystemStatusDetails.*),
          (si.storage_id, sv.volume_id) AS (&filesystemStatusDetails.*),
          si.uuid AS &filesystemStatusDetails.storage_instance_uuid
FROM      storage_filesystem sf
LEFT JOIN storage_filesystem_status sfs ON sfs.filesystem_uuid=sf.uuid
LEFT JOIN storage_instance_filesystem sif ON sf.uuid=sif.storage_filesystem_uuid
LEFT JOIN storage_instance si ON si.uuid=sif.storage_instance_uuid
LEFT JOIN storage_instance_volume siv ON siv.storage_instance_uuid=si.uuid
LEFT JOIN storage_volume sv ON sv.uuid=siv.storage_volume_uuid
`, filesystemStatusDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []filesystemStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&out)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceOrErr(out, func(v filesystemStatusDetails) (status.Filesystem, error) {
		statusValue, err := status.DecodeStorageFilesystemStatus(v.StatusID)
		if err != nil {
			return status.Filesystem{}, errors.Capture(err)
		}
		var volumeID *string
		if v.VolumeID.Valid {
			volumeID = &v.VolumeID.String
		}
		var storageInstanceUUID *storage.StorageInstanceUUID
		if v.StorageInstanceUUID.Valid {
			siUUID := storage.StorageInstanceUUID(v.StorageInstanceUUID.String)
			storageInstanceUUID = &siUUID
		}
		return status.Filesystem{
			UUID: storage.FilesystemUUID(v.UUID),
			ID:   v.ID,
			Life: life.Life(v.LifeID),
			Status: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status:  statusValue,
				Message: v.Message,
				Since:   v.UpdatedAt,
			},
			StorageUUID: storageInstanceUUID,
			StorageID:   v.StorageID,
			VolumeID:    volumeID,
			ProviderID:  v.ProviderID,
			SizeMiB:     v.SizeMiB,
		}, nil
	})
}

// GetFilesystemAttachments returns the specified filesystem attachments if they
// exist.
func (st *ModelState) GetFilesystemAttachments(
	ctx context.Context, uuids []storage.FilesystemUUID,
) ([]status.FilesystemAttachment, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// To satisfy the unit name column of this query a filesystem attachment
	// must be for a net node uuid that is on a unit where that unit does not
	// share a net node with a machine.
	// If units are for machines they share a net node.
	q := `
SELECT DISTINCT &filesystemAttachmentStatusDetails.* FROM (
    SELECT    sfa.storage_filesystem_uuid,
              sfa.life_id,
              sfa.mount_point,
              sfa.read_only,
              u.name As unit_name,
              m.name AS machine_name
    FROM      storage_filesystem_attachment sfa
    LEFT JOIN machine m ON sfa.net_node_uuid=m.net_node_uuid
    -- Only join units when there is no machine.
    LEFT JOIN unit u
        ON sfa.net_node_uuid = u.net_node_uuid
        AND m.net_node_uuid IS NULL
    LEFT JOIN storage_instance_filesystem sif ON sif.storage_filesystem_uuid=sfa.storage_filesystem_uuid
    LEFT JOIN storage_attachment sa ON sa.storage_instance_uuid=sif.storage_instance_uuid
    WHERE     sfa.storage_filesystem_uuid IN ($entityUUIDs[:])
)
`

	ids := IDs[entityUUIDs](uuids)
	stmt, err := st.Prepare(q, filesystemAttachmentStatusDetails{}, ids)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []filesystemAttachmentStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ids).GetAll(&out)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(out, func(v filesystemAttachmentStatusDetails) status.FilesystemAttachment {
		var machineName *machine.Name
		if v.MachineName.Valid {
			m := machine.Name(v.MachineName.String)
			machineName = &m
		}
		var unitName *unit.Name
		if v.UnitName.Valid {
			u := unit.Name(v.UnitName.String)
			unitName = &u
		}
		return status.FilesystemAttachment{
			FilesystemUUID: storage.FilesystemUUID(v.FilesystemUUID),
			Life:           life.Life(v.LifeID),
			Unit:           unitName,
			Machine:        machineName,
			MountPoint:     v.MountPoint,
			ReadOnly:       v.ReadOnly,
		}
	}), nil
}

// GetAllFilesystemAttachments returns all the filesystem attachments for this
// model.
func (st *ModelState) GetAllFilesystemAttachments(
	ctx context.Context,
) ([]status.FilesystemAttachment, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// To satisfy the unit name column of this query a filesystem attachment
	// must be for a net node uuid that is on a unit where that unit does not
	// share a net node with a machine.
	// If units are for machines they share a net node.
	q := `
SELECT DISTINCT &filesystemAttachmentStatusDetails.* FROM (
    SELECT    sfa.storage_filesystem_uuid,
              sfa.life_id,
              sfa.mount_point,
              sfa.read_only,
              u.name As unit_name,
              m.name AS machine_name
    FROM      storage_filesystem_attachment sfa
    LEFT JOIN machine m ON sfa.net_node_uuid=m.net_node_uuid
    -- Only join units when there is no machine.
    LEFT JOIN unit u
        ON sfa.net_node_uuid = u.net_node_uuid
        AND m.net_node_uuid IS NULL
    LEFT JOIN storage_instance_filesystem sif ON sif.storage_filesystem_uuid=sfa.storage_filesystem_uuid
    LEFT JOIN storage_attachment sa ON sa.storage_instance_uuid=sif.storage_instance_uuid
)
`

	stmt, err := st.Prepare(q, filesystemAttachmentStatusDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []filesystemAttachmentStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&out)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(out, func(v filesystemAttachmentStatusDetails) status.FilesystemAttachment {
		var machineName *machine.Name
		if v.MachineName.Valid {
			m := machine.Name(v.MachineName.String)
			machineName = &m
		}
		var unitName *unit.Name
		if v.UnitName.Valid {
			u := unit.Name(v.UnitName.String)
			unitName = &u
		}
		return status.FilesystemAttachment{
			FilesystemUUID: storage.FilesystemUUID(v.FilesystemUUID),
			Life:           life.Life(v.LifeID),
			Unit:           unitName,
			Machine:        machineName,
			MountPoint:     v.MountPoint,
			ReadOnly:       v.ReadOnly,
		}
	}), nil
}

// GetVolumes returns the specified volumes if they exist.
func (st *ModelState) GetVolumes(
	ctx context.Context, uuids []storage.VolumeUUID,
) ([]status.Volume, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ids := IDs[entityUUIDs](uuids)

	stmt, err := st.Prepare(`
SELECT    (sv.uuid, sv.volume_id, sv.life_id, sv.provider_id, sv.hardware_id, sv.wwn, sv.size_mib, sv.persistent) AS (&volumeStatusDetails.*),
          (svs.status_id, svs.message, svs.updated_at) AS (&volumeStatusDetails.*),
          (si.storage_id) AS (&volumeStatusDetails.*),
          si.uuid AS &volumeStatusDetails.storage_instance_uuid
FROM      storage_volume sv
LEFT JOIN storage_volume_status svs ON svs.volume_uuid=sv.uuid
LEFT JOIN storage_instance_volume siv ON siv.storage_volume_uuid=sv.uuid
LEFT JOIN storage_instance si ON si.uuid=siv.storage_instance_uuid
WHERE     sv.uuid IN ($entityUUIDs[:])
`, volumeStatusDetails{}, ids)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []volumeStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ids).GetAll(&out)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceOrErr(out, func(v volumeStatusDetails) (status.Volume, error) {
		statusValue, err := status.DecodeStorageVolumeStatus(v.StatusID)
		if err != nil {
			return status.Volume{}, errors.Capture(err)
		}
		var storageInstanceUUID *storage.StorageInstanceUUID
		if v.StorageInstanceUUID.Valid {
			siUUID := storage.StorageInstanceUUID(v.StorageInstanceUUID.String)
			storageInstanceUUID = &siUUID
		}
		return status.Volume{
			UUID: storage.VolumeUUID(v.UUID),
			ID:   v.ID,
			Life: life.Life(v.LifeID),
			Status: status.StatusInfo[status.StorageVolumeStatusType]{
				Status:  statusValue,
				Message: v.Message,
				Since:   v.UpdatedAt,
			},
			StorageUUID: storageInstanceUUID,
			StorageID:   v.StorageID,
			ProviderID:  v.ProviderID,
			HardwareID:  v.HardwareID,
			WWN:         v.WWN,
			Persistent:  v.Persistent,
			SizeMiB:     v.SizeMiB,
		}, nil
	})
}

// GetAllVolumes returns all the volumes for this model.
func (st *ModelState) GetAllVolumes(
	ctx context.Context,
) ([]status.Volume, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT    (sv.uuid, sv.volume_id, sv.life_id, sv.provider_id, sv.hardware_id, sv.wwn, sv.size_mib, sv.persistent) AS (&volumeStatusDetails.*),
          (svs.status_id, svs.message, svs.updated_at) AS (&volumeStatusDetails.*),
          (si.storage_id) AS (&volumeStatusDetails.*),
          si.uuid AS &volumeStatusDetails.storage_instance_uuid
FROM      storage_volume sv
LEFT JOIN storage_volume_status svs ON svs.volume_uuid=sv.uuid
LEFT JOIN storage_instance_volume siv ON siv.storage_volume_uuid=sv.uuid
LEFT JOIN storage_instance si ON si.uuid=siv.storage_instance_uuid
`, volumeStatusDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []volumeStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&out)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceOrErr(out, func(v volumeStatusDetails) (status.Volume, error) {
		statusValue, err := status.DecodeStorageVolumeStatus(v.StatusID)
		if err != nil {
			return status.Volume{}, errors.Capture(err)
		}
		var storageInstanceUUID *storage.StorageInstanceUUID
		if v.StorageInstanceUUID.Valid {
			siUUID := storage.StorageInstanceUUID(v.StorageInstanceUUID.String)
			storageInstanceUUID = &siUUID
		}
		return status.Volume{
			UUID: storage.VolumeUUID(v.UUID),
			ID:   v.ID,
			Life: life.Life(v.LifeID),
			Status: status.StatusInfo[status.StorageVolumeStatusType]{
				Status:  statusValue,
				Message: v.Message,
				Since:   v.UpdatedAt,
			},
			StorageUUID: storageInstanceUUID,
			StorageID:   v.StorageID,
			ProviderID:  v.ProviderID,
			HardwareID:  v.HardwareID,
			WWN:         v.WWN,
			Persistent:  v.Persistent,
			SizeMiB:     v.SizeMiB,
		}, nil
	})
}

// GetVolumeAttachments returns the specified volume attachments if they exist.
func (st *ModelState) GetVolumeAttachments(
	ctx context.Context, uuids []storage.VolumeUUID,
) ([]status.VolumeAttachment, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// To satisfy the unit name column of this query a volume attachment
	// must be for a net node uuid that is on a unit where that unit does not
	// share a net node with a machine.
	// If units are for machines they share a net node.
	q := `
SELECT DISTINCT &volumeAttachmentStatusDetails.* FROM (
    SELECT    sva.storage_volume_uuid,
              sva.life_id,
              sva.read_only,
              bd.name AS device_name,
              bd.bus_address AS bus_address,
              first_value(bdld.name) OVER bdld_first AS device_link,
              u.name AS unit_name,
              m.name AS machine_name
    FROM      storage_volume_attachment sva
    LEFT JOIN block_device bd ON bd.uuid=sva.block_device_uuid
    LEFT JOIN block_device_link_device bdld ON bdld.block_device_uuid=bd.uuid
    LEFT JOIN machine m ON sva.net_node_uuid=m.net_node_uuid
    LEFT JOIN unit u
        ON sva.net_node_uuid=u.net_node_uuid
        AND m.net_node_uuid IS NULL
    WINDOW    bdld_first AS (PARTITION BY bdld.block_device_uuid ORDER BY bdld.name)
    WHERE sva.storage_volume_uuid IN ($entityUUIDs[:])
)
`

	ids := IDs[entityUUIDs](uuids)
	stmt, err := st.Prepare(q, volumeAttachmentStatusDetails{}, ids)
	if err != nil {
		return nil, errors.Capture(err)
	}

	volPlanStmt, err := st.Prepare(`
SELECT    &volumeAttachmentPlanStatusDetails.*
FROM      storage_volume_attachment_plan svap
LEFT JOIN storage_volume_attachment_plan_attr svapa ON svapa.attachment_plan_uuid=svap.uuid

`, volumeAttachmentPlanStatusDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []volumeAttachmentStatusDetails
	var vapOut []volumeAttachmentPlanStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ids).GetAll(&out)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		err = tx.Query(ctx, volPlanStmt).GetAll(&vapOut)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	vaps := map[string]*status.VolumeAttachmentPlan{}
	for _, v := range vapOut {
		vap := vaps[v.VolumeUUID]
		if vap == nil {
			vap = &status.VolumeAttachmentPlan{
				DeviceType: storage.VolumeDeviceType(v.DeviceTypeID),
			}
			vaps[v.VolumeUUID] = vap
		}
		if v.DeviceAttributeKey.Valid && v.DeviceAttributeValue.Valid {
			key := v.DeviceAttributeKey.String
			value := v.DeviceAttributeValue.String
			if vap.DeviceAttributes == nil {
				vap.DeviceAttributes = map[string]string{}
			}
			vap.DeviceAttributes[key] = value
		}
	}

	return transform.Slice(out, func(v volumeAttachmentStatusDetails) status.VolumeAttachment {
		var machineName *machine.Name
		if v.MachineName.Valid {
			m := machine.Name(v.MachineName.String)
			machineName = &m
		}
		var unitName *unit.Name
		if v.UnitName.Valid {
			u := unit.Name(v.UnitName.String)
			unitName = &u
		}
		return status.VolumeAttachment{
			VolumeUUID:           storage.VolumeUUID(v.VolumeUUID),
			Life:                 life.Life(v.LifeID),
			Unit:                 unitName,
			Machine:              machineName,
			DeviceName:           v.DeviceName,
			DeviceLink:           v.DeviceLink,
			BusAddress:           v.BusAddress,
			ReadOnly:             v.ReadOnly,
			VolumeAttachmentPlan: vaps[v.VolumeUUID],
		}
	}), nil
}

// GetAllVolumeAttachments returns all the volume attachments for this model.
func (st *ModelState) GetAllVolumeAttachments(
	ctx context.Context,
) ([]status.VolumeAttachment, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// To satisfy the unit name column of this query a volume attachment
	// must be for a net node uuid that is on a unit where that unit does not
	// share a net node with a machine.
	// If units are for machines they share a net node.
	q := `
SELECT DISTINCT &volumeAttachmentStatusDetails.* FROM (
    SELECT    sva.storage_volume_uuid,
              sva.life_id,
              sva.read_only,
              bd.name AS device_name,
              bd.bus_address AS bus_address,
              first_value(bdld.name) OVER bdld_first AS device_link,
              u.name AS unit_name,
              m.name AS machine_name
    FROM      storage_volume_attachment sva
    LEFT JOIN block_device bd ON bd.uuid=sva.block_device_uuid
    LEFT JOIN block_device_link_device bdld ON bdld.block_device_uuid=bd.uuid
    LEFT JOIN machine m ON sva.net_node_uuid=m.net_node_uuid
    LEFT JOIN unit u
        ON sva.net_node_uuid=u.net_node_uuid
        AND m.net_node_uuid IS NULL
    WINDOW    bdld_first AS (PARTITION BY bdld.block_device_uuid ORDER BY bdld.name)
)
`

	stmt, err := st.Prepare(q, volumeAttachmentStatusDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	volPlanStmt, err := st.Prepare(`
SELECT    &volumeAttachmentPlanStatusDetails.*
FROM      storage_volume_attachment_plan svap
LEFT JOIN storage_volume_attachment_plan_attr svapa ON svapa.attachment_plan_uuid=svap.uuid
`, volumeAttachmentPlanStatusDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var out []volumeAttachmentStatusDetails
	var vapOut []volumeAttachmentPlanStatusDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&out)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		err = tx.Query(ctx, volPlanStmt).GetAll(&vapOut)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	vaps := map[string]*status.VolumeAttachmentPlan{}
	for _, v := range vapOut {
		vap := vaps[v.VolumeUUID]
		if vap == nil {
			vap = &status.VolumeAttachmentPlan{
				DeviceType: storage.VolumeDeviceType(v.DeviceTypeID),
			}
			vaps[v.VolumeUUID] = vap
		}
		if v.DeviceAttributeKey.Valid && v.DeviceAttributeValue.Valid {
			key := v.DeviceAttributeKey.String
			value := v.DeviceAttributeValue.String
			if vap.DeviceAttributes == nil {
				vap.DeviceAttributes = map[string]string{}
			}
			vap.DeviceAttributes[key] = value
		}
	}

	return transform.Slice(out, func(v volumeAttachmentStatusDetails) status.VolumeAttachment {
		var machineName *machine.Name
		if v.MachineName.Valid {
			m := machine.Name(v.MachineName.String)
			machineName = &m
		}
		var unitName *unit.Name
		if v.UnitName.Valid {
			u := unit.Name(v.UnitName.String)
			unitName = &u
		}
		return status.VolumeAttachment{
			VolumeUUID:           storage.VolumeUUID(v.VolumeUUID),
			Life:                 life.Life(v.LifeID),
			Unit:                 unitName,
			Machine:              machineName,
			DeviceName:           v.DeviceName,
			DeviceLink:           v.DeviceLink,
			BusAddress:           v.BusAddress,
			ReadOnly:             v.ReadOnly,
			VolumeAttachmentPlan: vaps[v.VolumeUUID],
		}
	}), nil
}

func IDs[RS ~[]R, R ~string, TS ~[]T, T ~string](input TS) RS {
	r := make(RS, len(input))
	for i, v := range input {
		r[i] = R(string(v))
	}
	return r
}
