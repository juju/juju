// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"encoding/json"

	corestatus "github.com/juju/juju/core/status"
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

// encodeFilesystemStatusType maps a core status to corresponding db filesystem
// status.
func encodeFilesystemStatusType(s corestatus.Status) (StorageFilesystemStatusType, error) {
	switch s {
	case corestatus.Pending:
		return StorageFilesystemStatusTypePending, nil
	case corestatus.Error:
		return StorageFilesystemStatusTypeError, nil
	case corestatus.Attaching:
		return StorageFilesystemStatusTypeAttaching, nil
	case corestatus.Attached:
		return StorageFilesystemStatusTypeAttached, nil
	case corestatus.Detaching:
		return StorageFilesystemStatusTypeDetaching, nil
	case corestatus.Detached:
		return StorageFilesystemStatusTypeDetached, nil
	case corestatus.Destroying:
		return StorageFilesystemStatusTypeDestroying, nil
	default:
		return -1, errors.Errorf("unknown filesystem status %q", s)
	}
}

// decodeFilesystemStatusType maps a domain status to corresponding core status.
func decodeFilesystemStatusType(s StorageFilesystemStatusType) (corestatus.Status, error) {
	switch s {
	case StorageFilesystemStatusTypePending:
		return corestatus.Pending, nil
	case StorageFilesystemStatusTypeError:
		return corestatus.Error, nil
	case StorageFilesystemStatusTypeAttaching:
		return corestatus.Attaching, nil
	case StorageFilesystemStatusTypeAttached:
		return corestatus.Attached, nil
	case StorageFilesystemStatusTypeDetaching:
		return corestatus.Detaching, nil
	case StorageFilesystemStatusTypeDetached:
		return corestatus.Detached, nil
	case StorageFilesystemStatusTypeDestroying:
		return corestatus.Destroying, nil
	default:
		return corestatus.Unknown, nil
	}
}

// EncodeFilesystemStatus converts a core status info to a db status info.
func EncodeFilesystemStatus(s corestatus.StatusInfo) (StatusInfo[StorageFilesystemStatusType], error) {
	encodedStatus, err := encodeFilesystemStatusType(s.Status)
	if err != nil {
		return StatusInfo[StorageFilesystemStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return StatusInfo[StorageFilesystemStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return StatusInfo[StorageFilesystemStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// DecodeFilesystemStatus converts a db status info to a core status info.
func DecodeFilesystemStatus(s StatusInfo[StorageFilesystemStatusType]) (corestatus.StatusInfo, error) {
	decodedStatus, err := decodeFilesystemStatusType(s.Status)
	if err != nil {
		return corestatus.StatusInfo{}, err
	}

	var data map[string]any
	if len(s.Data) > 0 {
		err := json.Unmarshal(s.Data, &data)
		if err != nil {
			return corestatus.StatusInfo{}, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return corestatus.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}

// encodeVolumeStatusType maps a core status to corresponding db volume
// status.
func encodeVolumeStatusType(s corestatus.Status) (StorageVolumeStatusType, error) {
	switch s {
	case corestatus.Pending:
		return StorageVolumeStatusTypePending, nil
	case corestatus.Error:
		return StorageVolumeStatusTypeError, nil
	case corestatus.Attaching:
		return StorageVolumeStatusTypeAttaching, nil
	case corestatus.Attached:
		return StorageVolumeStatusTypeAttached, nil
	case corestatus.Detaching:
		return StorageVolumeStatusTypeDetaching, nil
	case corestatus.Detached:
		return StorageVolumeStatusTypeDetached, nil
	case corestatus.Destroying:
		return StorageVolumeStatusTypeDestroying, nil
	default:
		return -1, errors.Errorf("unknown volume status %q", s)
	}
}

// decodeVolumeStatusType maps a db status to corresponding core status.
func decodeVolumeStatusType(s StorageVolumeStatusType) (corestatus.Status, error) {
	switch s {
	case StorageVolumeStatusTypePending:
		return corestatus.Pending, nil
	case StorageVolumeStatusTypeError:
		return corestatus.Error, nil
	case StorageVolumeStatusTypeAttaching:
		return corestatus.Attaching, nil
	case StorageVolumeStatusTypeAttached:
		return corestatus.Attached, nil
	case StorageVolumeStatusTypeDetaching:
		return corestatus.Detaching, nil
	case StorageVolumeStatusTypeDetached:
		return corestatus.Detached, nil
	case StorageVolumeStatusTypeDestroying:
		return corestatus.Destroying, nil
	default:
		return corestatus.Unknown, nil
	}
}

// EncodeVolumeStatus converts a core status info to a db status info.
func EncodeVolumeStatus(s corestatus.StatusInfo) (StatusInfo[StorageVolumeStatusType], error) {
	encodedStatus, err := encodeVolumeStatusType(s.Status)
	if err != nil {
		return StatusInfo[StorageVolumeStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return StatusInfo[StorageVolumeStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return StatusInfo[StorageVolumeStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// DecodeVolumeStatus converts a db status info to a core status info.
func DecodeVolumeStatus(s StatusInfo[StorageVolumeStatusType]) (corestatus.StatusInfo, error) {
	decodedStatus, err := decodeVolumeStatusType(s.Status)
	if err != nil {
		return corestatus.StatusInfo{}, err
	}

	var data map[string]any
	if len(s.Data) > 0 {
		err := json.Unmarshal(s.Data, &data)
		if err != nil {
			return corestatus.StatusInfo{}, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return corestatus.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}
