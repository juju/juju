// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain/status"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

// SetFilesystemStatus saves the given filesystem status, overwriting any
// current status data. The following errors can be expected:
// - [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
func (st *State) SetFilesystemStatus(
	ctx context.Context,
	filesystemUUID storage.FilesystemUUID,
	sts status.StatusInfo[status.StorageFilesystemStatusType],
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get current status.
		currentStatus, isProvisioned, err := st.getFilesystemProvisioningStatus(ctx, tx, filesystemUUID)
		if err != nil {
			return errors.Errorf("getting current filesystem status: %w", err)
		}

		// Check we can transition from current status to the new status.
		err = status.FilesystemStatusTransitionValid(currentStatus, isProvisioned, sts)
		if err != nil {
			return errors.Capture(err)
		}

		return st.updateFilesystemStatus(ctx, tx, filesystemUUID, sts)
	})
	if err != nil {
		return errors.Errorf("updating filesystem status for %q: %w", filesystemUUID, err)
	}
	return nil
}

// getFilesystemStatus gets the status of the given filesystem
// and a bool indicating if it is provisioned.
// The following errors can be expected:
// - [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
func (st *State) getFilesystemProvisioningStatus(
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
JOIN      storage_filesystem_status sfs ON sf.uuid = sfs.filesystem_uuid
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
	statusType, err := status.DecodeStorageFilesystemStatus(sts.StatusID)
	if err != nil {
		return -1, false, errors.Capture(err)
	}
	isProvisioned := sts.StorageInstanceUUID != ""
	return statusType, isProvisioned, nil
}

// ImportFilesystemStatus sets the given filesystem status.
// The following errors can be expected:
// - [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
func (st *State) ImportFilesystemStatus(
	ctx context.Context,
	filesystemID string,
	sts status.StatusInfo[status.StorageFilesystemStatusType],
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		filesystemUUID, err := st.getFilesystemUUIDByID(ctx, tx, filesystemID)
		if err != nil {
			return errors.Errorf("getting filesystem UUID: %w", err)
		}
		return st.updateFilesystemStatus(ctx, tx, filesystemUUID, sts)
	})
}

func (st *State) getFilesystemUUIDByID(
	ctx context.Context,
	tx *sqlair.TX,
	id string,
) (storage.FilesystemUUID, error) {
	arg := filesystemID{ID: id}
	stmt, err := st.Prepare(`
SELECT &filesystemID.uuid
FROM   storage_filesystem
WHERE  filesystem_id = $filesystemID.filesystem_id
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Get(&arg)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", storageerrors.FilesystemNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}
	return storage.FilesystemUUID(arg.UUID), nil
}

func (st *State) updateFilesystemStatus(
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
func (st *State) SetVolumeStatus(
	ctx context.Context,
	volumeUUID storage.VolumeUUID,
	sts status.StatusInfo[status.StorageVolumeStatusType],
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get current status.
		currentStatus, isProvisioned, err := st.getVolumeProvisioningStatus(ctx, tx, volumeUUID)
		if err != nil {
			return errors.Errorf("getting current volume status: %w", err)
		}

		// Check we can transition from current status to the new status.
		err = status.VolumeStatusTransitionValid(currentStatus, isProvisioned, sts)
		if err != nil {
			return errors.Capture(err)
		}

		return st.updateVolumeStatus(ctx, tx, volumeUUID, sts)
	})
	if err != nil {
		return errors.Errorf("updating volume status for %q: %w", volumeUUID, err)
	}
	return nil
}

// getVolumeStatus gets the status of the given volume
// and a bool indicating if it is provisioned.
// The following errors can be expected:
// - [storageerrors.VolumeNotFound] if the volume doesn't exist.
func (st *State) getVolumeProvisioningStatus(
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
FROM      storage_volume sf
JOIN      storage_volume_status sfs ON sf.uuid = sfs.volume_uuid
LEFT JOIN storage_instance_volume sif ON sf.uuid = sif.storage_volume_uuid
WHERE     sf.uuid = $volumeUUID.uuid
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
	statusType, err := status.DecodeStorageVolumeStatus(sts.StatusID)
	if err != nil {
		return -1, false, errors.Capture(err)
	}
	isProvisioned := sts.StorageInstanceUUID != ""
	return statusType, isProvisioned, nil
}

// ImportVolumeStatus sets the given volume status.
// The following errors can be expected:
// - [storageerrors.VolumeNotFound] if the volume doesn't exist.
func (st *State) ImportVolumeStatus(
	ctx context.Context,
	volumeID string,
	sts status.StatusInfo[status.StorageVolumeStatusType],
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		volumeUUID, err := st.getVolumeUUIDByID(ctx, tx, volumeID)
		if err != nil {
			return errors.Errorf("getting volume UUID: %w", err)
		}
		return st.updateVolumeStatus(ctx, tx, volumeUUID, sts)
	})
}

func (st *State) getVolumeUUIDByID(
	ctx context.Context,
	tx *sqlair.TX,
	id string,
) (storage.VolumeUUID, error) {
	arg := volumeID{ID: id}
	stmt, err := st.Prepare(`
SELECT &volumeID.uuid
FROM   storage_volume
WHERE  volume_id = $volumeID.volume_id
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Get(&arg)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", storageerrors.VolumeNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}
	return storage.VolumeUUID(arg.UUID), nil
}

func (st *State) updateVolumeStatus(
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
