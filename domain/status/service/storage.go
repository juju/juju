// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// StorageState provides access to storage related state methods.
type StorageState interface {
	// SetFilesystemStatus sets the given filesystem status, overwriting any
	// current status data. The following errors can be expected:
	// - [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
	SetFilesystemStatus(
		ctx context.Context,
		filesystemUUID storageprovisioning.FilesystemUUID,
		sts status.StatusInfo[status.StorageFilesystemStatusType],
	) error

	// GetFilesystemUUIDByID returns the filesystem UUID for the given
	// filesystem unique id. If no filesystem is found, an error satisfying
	// [storageerrors.FilesystemNotFound] is returned.
	GetFilesystemUUIDByID(ctx context.Context, id string) (storageprovisioning.FilesystemUUID, error)

	// SetVolumeStatus sets the given volume status, overwriting any
	// current status data. The following errors can be expected:
	// - [storageerrors.VolumeNotFound] if the volume doesn't exist.
	SetVolumeStatus(
		ctx context.Context,
		volumeUUID storageprovisioning.VolumeUUID,
		sts status.StatusInfo[status.StorageVolumeStatusType],
	) error

	// GetVolumeUUIDByID returns the volume UUID for the given
	// volume unique id. If no volume is found, an error satisfying
	// [storageerrors.VolumeNotFound] is returned.
	GetVolumeUUIDByID(ctx context.Context, id string) (storage.VolumeUUID, error)
}

// SetFilesystemStatus validates and sets the given filesystem status, overwriting any
// current status data. If returns an error satisfying
// [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
func (s *Service) SetFilesystemStatus(
	ctx context.Context,
	filesystemID string,
	statusInfo corestatus.StatusInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if statusInfo.Status == corestatus.Error && statusInfo.Message == "" {
		return errors.Errorf("cannot set status %q without message", statusInfo.Status)
	}

	// This will also verify that the status is valid.
	encodedStatus, err := encodeFilesystemStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding filesystem status: %w", err)
	}

	uuid, err := s.modelState.GetFilesystemUUIDByID(ctx, filesystemID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := s.modelState.SetFilesystemStatus(ctx, uuid, encodedStatus); err != nil {
		return errors.Capture(err)
	}

	if err := s.statusHistory.RecordStatus(ctx, status.FilesystemNamespace.WithID(uuid.String()), statusInfo); err != nil {
		s.logger.Warningf(ctx, "recording filesystem status history: %v", err)
	}

	return nil
}

// SetVolumeStatus validates and sets the given volume status, overwriting any
// current status data. If returns an error satisfying
// [storageerrors.VolumeNotFound] if the volume doesn't exist.
func (s *Service) SetVolumeStatus(
	ctx context.Context,
	volumeID string,
	statusInfo corestatus.StatusInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if statusInfo.Status == corestatus.Error && statusInfo.Message == "" {
		return errors.Errorf("cannot set status %q without message", statusInfo.Status)
	}

	// This will also verify that the status is valid.
	encodedStatus, err := encodeVolumeStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding volume status: %w", err)
	}

	uuid, err := s.modelState.GetVolumeUUIDByID(ctx, volumeID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := s.modelState.SetVolumeStatus(ctx, uuid, encodedStatus); err != nil {
		return errors.Capture(err)
	}

	if err := s.statusHistory.RecordStatus(ctx, status.VolumeNamespace.WithID(uuid.String()), statusInfo); err != nil {
		s.logger.Warningf(ctx, "recording volume status history: %v", err)
	}

	return nil
}

// GetStorageInstanceStatuses returns all the storage instance statuses for
// the model.
func (s *Service) GetStorageInstanceStatuses(ctx context.Context) ([]StorageInstance, error) {
	return nil, nil
}

// GetFilesystemStatuses returns all the filesystem statuses for the model.
func (s *Service) GetFilesystemStatuses(ctx context.Context) ([]Filesystem, error) {
	return nil, nil
}

// GetVolumeStatuses returns all the volume statuses for the model.
func (s *Service) GetVolumeStatuses(ctx context.Context) ([]Volume, error) {
	return nil, nil
}
