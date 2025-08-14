// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/internal/errors"
)

// StorageFilesystemStatusType represents the status of a filesystem
// as recorded in the storage_filesystem_status_value lookup table.
type StorageFilesystemStatusType int
type StorageFilesystemStatusInfo struct {
	StatusInfo StatusInfo[StorageFilesystemStatusType]
}

const (
	StorageFilesystemStatusTypePending StorageFilesystemStatusType = iota
	StorageFilesystemStatusTypeError
	StorageFilesystemStatusTypeAttaching
	StorageFilesystemStatusTypeAttached
	StorageFilesystemStatusTypeDetaching
	StorageFilesystemStatusTypeDetached
	StorageFilesystemStatusTypeDestroying
)

// EncodeStorageFilesystemStatus encodes a StorageFilesystemStatusType into its
// integer id, as recorded in the storage_filesystem_status_value lookup table.
func EncodeStorageFilesystemStatus(s StorageFilesystemStatusType) (int, error) {
	switch s {
	case StorageFilesystemStatusTypePending:
		return 0, nil
	case StorageFilesystemStatusTypeError:
		return 1, nil
	case StorageFilesystemStatusTypeAttaching:
		return 2, nil
	case StorageFilesystemStatusTypeAttached:
		return 3, nil
	case StorageFilesystemStatusTypeDetaching:
		return 4, nil
	case StorageFilesystemStatusTypeDetached:
		return 5, nil
	case StorageFilesystemStatusTypeDestroying:
		return 6, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// DecodeStorageFilesystemStatus decodes a StorageFilesystemStatusType from its
// integer id, as recorded in the storage_filesystem_status_value lookup table.
func DecodeStorageFilesystemStatus(s int) (StorageFilesystemStatusType, error) {
	switch s {
	case 0:
		return StorageFilesystemStatusTypePending, nil
	case 1:
		return StorageFilesystemStatusTypeError, nil
	case 2:
		return StorageFilesystemStatusTypeAttaching, nil
	case 3:
		return StorageFilesystemStatusTypeAttached, nil
	case 4:
		return StorageFilesystemStatusTypeDetaching, nil
	case 5:
		return StorageFilesystemStatusTypeDetached, nil
	case 6:
		return StorageFilesystemStatusTypeDestroying, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// FilesystemStatusTransitionValid returns the error
// [statuserror.FilesystemStatusTransitionNotValid] if the transition from the
// current status to the new status is not valid.
func FilesystemStatusTransitionValid(
	current StorageFilesystemStatusType,
	isProvisioned bool,
	new StatusInfo[StorageFilesystemStatusType],
) error {
	if current == new.Status {
		return nil
	}
	validTransition := true
	switch new.Status {
	case StorageFilesystemStatusTypePending:
		// If a filesystem is not yet provisioned, we allow its status
		// to be set back to pending (when a retry is to occur).
		validTransition = !isProvisioned
	default:
		// Anything else is ok.
	}
	if !validTransition {
		return errors.Errorf(
			"cannot set status %q when filesystem has status %q: %w",
			new.Status, current, statuserrors.FilesystemStatusTransitionNotValid,
		)
	}
	return nil
}

// StorageVolumeStatusType represents the status of a volume
// as recorded in the storage_volume_status_value lookup table.
type StorageVolumeStatusType int
type StorageVolumeStatusInfo struct {
	StatusInfo StatusInfo[StorageVolumeStatusType]
}

const (
	StorageVolumeStatusTypePending StorageVolumeStatusType = iota
	StorageVolumeStatusTypeError
	StorageVolumeStatusTypeAttaching
	StorageVolumeStatusTypeAttached
	StorageVolumeStatusTypeDetaching
	StorageVolumeStatusTypeDetached
	StorageVolumeStatusTypeDestroying
)

// EncodeStorageVolumeStatus encodes a StorageVolumeStatusType into its
// integer id, as recorded in the storage_volume_status_value lookup table.
func EncodeStorageVolumeStatus(s StorageVolumeStatusType) (int, error) {
	switch s {
	case StorageVolumeStatusTypePending:
		return 0, nil
	case StorageVolumeStatusTypeError:
		return 1, nil
	case StorageVolumeStatusTypeAttaching:
		return 2, nil
	case StorageVolumeStatusTypeAttached:
		return 3, nil
	case StorageVolumeStatusTypeDetaching:
		return 4, nil
	case StorageVolumeStatusTypeDetached:
		return 5, nil
	case StorageVolumeStatusTypeDestroying:
		return 6, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// DecodeStorageVolumeStatus decodes a StorageVolumeStatusType from its
// integer id, as recorded in the storage_volume_status_value lookup table.
func DecodeStorageVolumeStatus(s int) (StorageVolumeStatusType, error) {
	switch s {
	case 0:
		return StorageVolumeStatusTypePending, nil
	case 1:
		return StorageVolumeStatusTypeError, nil
	case 2:
		return StorageVolumeStatusTypeAttaching, nil
	case 3:
		return StorageVolumeStatusTypeAttached, nil
	case 4:
		return StorageVolumeStatusTypeDetaching, nil
	case 5:
		return StorageVolumeStatusTypeDetached, nil
	case 6:
		return StorageVolumeStatusTypeDestroying, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// VolumeStatusTransitionValid returns the error
// [statuserror.VolumeStatusTransitionNotValid] if the transition from the
// current status to the new status is not valid.
func VolumeStatusTransitionValid(
	current StorageVolumeStatusType,
	isProvisioned bool,
	new StatusInfo[StorageVolumeStatusType],
) error {
	if current == new.Status {
		return nil
	}
	validTransition := true
	switch new.Status {
	case StorageVolumeStatusTypePending:
		// If a volume is not yet provisioned, we allow its status
		// to be set back to pending (when a retry is to occur).
		validTransition = !isProvisioned
	default:
		// Anything else is ok.
	}
	if !validTransition {
		return errors.Errorf(
			"cannot set status %q when volume has status %q: %w",
			new.Status, current, statuserrors.VolumeStatusTransitionNotValid,
		)
	}
	return nil
}
