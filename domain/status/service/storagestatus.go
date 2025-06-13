// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"encoding/json"

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// encodeFilesystemStatusType maps a core status to corresponding db filesystem
// status.
func encodeFilesystemStatusType(s corestatus.Status) (status.StorageFilesystemStatusType, error) {
	switch s {
	case corestatus.Pending:
		return status.StorageFilesystemStatusTypePending, nil
	case corestatus.Error:
		return status.StorageFilesystemStatusTypeError, nil
	case corestatus.Attaching:
		return status.StorageFilesystemStatusTypeAttaching, nil
	case corestatus.Attached:
		return status.StorageFilesystemStatusTypeAttached, nil
	case corestatus.Detaching:
		return status.StorageFilesystemStatusTypeDetaching, nil
	case corestatus.Detached:
		return status.StorageFilesystemStatusTypeDetached, nil
	case corestatus.Destroying:
		return status.StorageFilesystemStatusTypeDestroying, nil
	default:
		return -1, errors.Errorf("unknown filesystem status %q", s)
	}
}

// encodeFilesystemStatus converts a core status info to a db status info.
func encodeFilesystemStatus(s corestatus.StatusInfo) (status.StatusInfo[status.StorageFilesystemStatusType], error) {
	encodedStatus, err := encodeFilesystemStatusType(s.Status)
	if err != nil {
		return status.StatusInfo[status.StorageFilesystemStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return status.StatusInfo[status.StorageFilesystemStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// encodeVolumeStatusType maps a core status to corresponding db volume
// status.
func encodeVolumeStatusType(s corestatus.Status) (status.StorageVolumeStatusType, error) {
	switch s {
	case corestatus.Pending:
		return status.StorageVolumeStatusTypePending, nil
	case corestatus.Error:
		return status.StorageVolumeStatusTypeError, nil
	case corestatus.Attaching:
		return status.StorageVolumeStatusTypeAttaching, nil
	case corestatus.Attached:
		return status.StorageVolumeStatusTypeAttached, nil
	case corestatus.Detaching:
		return status.StorageVolumeStatusTypeDetaching, nil
	case corestatus.Detached:
		return status.StorageVolumeStatusTypeDetached, nil
	case corestatus.Destroying:
		return status.StorageVolumeStatusTypeDestroying, nil
	default:
		return -1, errors.Errorf("unknown volume status %q", s)
	}
}

// encodeVolumeStatus converts a core status info to a db status info.
func encodeVolumeStatus(s corestatus.StatusInfo) (status.StatusInfo[status.StorageVolumeStatusType], error) {
	encodedStatus, err := encodeVolumeStatusType(s.Status)
	if err != nil {
		return status.StatusInfo[status.StorageVolumeStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return status.StatusInfo[status.StorageVolumeStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}
