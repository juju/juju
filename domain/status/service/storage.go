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
	GetVolumeUUIDByID(ctx context.Context, id string) (storageprovisioning.VolumeUUID, error)

	// GetStorageInstances returns all the storage instances for this model.
	GetStorageInstances(ctx context.Context) ([]status.StorageInstance, error)

	// GetStorageInstanceAttachments returns all the storage instance
	// attachments for this model.
	GetStorageInstanceAttachments(ctx context.Context) ([]status.StorageAttachment, error)

	// GetFilesystems returns all the filesystems for this model.
	GetFilesystems(ctx context.Context) ([]status.Filesystem, error)

	// GetFilesystemAttachments returns all the filesystem attachments for this
	// model.
	GetFilesystemAttachments(ctx context.Context) ([]status.FilesystemAttachment, error)

	// GetVolumes returns all the volumes for this model.
	GetVolumes(ctx context.Context) ([]status.Volume, error)

	// GetVolumeAttachments returns all the volume attachments for this model.
	GetVolumeAttachments(ctx context.Context) ([]status.VolumeAttachment, error)
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
func (s *Service) GetStorageInstanceStatuses(
	ctx context.Context,
) ([]StorageInstance, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	storageInstances, err := s.modelState.GetStorageInstances(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	storageAttachments, err := s.modelState.GetStorageInstanceAttachments(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	storageMap := map[storage.StorageInstanceUUID]*StorageInstance{}
	for _, dsi := range storageInstances {
		si := StorageInstance{
			ID: dsi.ID,
		}
		var err error
		si.Life, err = dsi.Life.Value()
		if err != nil {
			return nil, errors.Capture(err)
		}
		switch dsi.Kind {
		case storage.StorageKindBlock:
			si.Kind = internalstorage.StorageKindBlock
		case storage.StorageKindFilesystem:
			si.Kind = internalstorage.StorageKindFilesystem
		default:
			si.Kind = internalstorage.StorageKindUnknown
		}
		storageMap[dsi.UUID] = &si
	}
	for _, dsa := range storageAttachments {
		sa := StorageAttachment{
			Unit:    dsa.Unit,
			Machine: dsa.Machine,
		}
		var err error
		sa.Life, err = dsa.Life.Value()
		if err != nil {
			return nil, errors.Capture(err)
		}
		if si, ok := storageMap[dsa.StorageInstanceUUID]; ok {
			if si.Attachments == nil {
				si.Attachments = map[unit.Name]StorageAttachment{}
			}
			si.Attachments[sa.Unit] = sa
		}
	}

	ret := make([]StorageInstance, 0, len(storageMap))
	for _, v := range storageMap {
		ret = append(ret, *v)
	}
	return ret, nil
}

// GetFilesystemStatuses returns all the filesystem statuses for the model.
func (s *Service) GetFilesystemStatuses(ctx context.Context) ([]Filesystem, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	filesystems, err := s.modelState.GetFilesystems(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	filesystemAttachments, err := s.modelState.GetFilesystemAttachments(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	fsMap := map[storageprovisioning.FilesystemUUID]*Filesystem{}
	for _, dfs := range filesystems {
		fs := Filesystem{
			ID:         dfs.ID,
			StorageID:  dfs.StorageID,
			VolumeID:   dfs.VolumeID,
			ProviderID: dfs.ProviderID,
			SizeMiB:    dfs.SizeMiB,
		}
		var err error
		fs.Life, err = dfs.Life.Value()
		if err != nil {
			return nil, errors.Capture(err)
		}
		fs.Status, err = decodeFilesystemStatus(dfs.Status)
		if err != nil {
			return nil, errors.Capture(err)
		}
		fsMap[dfs.UUID] = &fs
	}
	for _, dfsa := range filesystemAttachments {
		fsa := FilesystemAttachment{
			MountPoint: dfsa.MountPoint,
			ReadOnly:   dfsa.ReadOnly,
		}
		var err error
		fsa.Life, err = dfsa.Life.Value()
		if err != nil {
			return nil, errors.Capture(err)
		}
		if fs, ok := fsMap[dfsa.FilesystemUUID]; ok {
			if dfsa.Unit != nil {
				if fs.UnitAttachments == nil {
					fs.UnitAttachments = map[unit.Name]FilesystemAttachment{}
				}
				fs.UnitAttachments[*dfsa.Unit] = fsa
			}
			if dfsa.Machine != nil {
				if fs.MachineAttachments == nil {
					fs.MachineAttachments = map[machine.Name]FilesystemAttachment{}
				}
				fs.MachineAttachments[*dfsa.Machine] = fsa
			}
		}
	}

	ret := make([]Filesystem, 0, len(fsMap))
	for _, v := range fsMap {
		ret = append(ret, *v)
	}
	return ret, nil
}

// GetVolumeStatuses returns all the volume statuses for the model.
func (s *Service) GetVolumeStatuses(ctx context.Context) ([]Volume, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	volumes, err := s.modelState.GetVolumes(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	volumeAttachments, err := s.modelState.GetVolumeAttachments(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	volumeMap := map[storageprovisioning.VolumeUUID]*Volume{}
	for _, dv := range volumes {
		v := Volume{
			ID:         dv.ID,
			StorageID:  dv.StorageID,
			ProviderID: dv.ProviderID,
			HardwareID: dv.HardwareID,
			WWN:        dv.WWN,
			SizeMiB:    dv.SizeMiB,
			Persistent: dv.Persistent,
		}
		var err error
		v.Life, err = dv.Life.Value()
		if err != nil {
			return nil, errors.Capture(err)
		}
		v.Status, err = decodeVolumeStatus(dv.Status)
		if err != nil {
			return nil, errors.Capture(err)
		}
		volumeMap[dv.UUID] = &v
	}
	for _, dva := range volumeAttachments {
		va := VolumeAttachment{
			DeviceName: dva.DeviceName,
			DeviceLink: dva.DeviceLink,
			BusAddress: dva.BusAddress,
			ReadOnly:   dva.ReadOnly,
		}
		var err error
		va.Life, err = dva.Life.Value()
		if err != nil {
			return nil, errors.Capture(err)
		}
		if dvap := dva.VolumeAttachmentPlan; dvap != nil {
			vap := VolumeAttachmentPlan{
				DeviceAttributes: dvap.DeviceAttributes,
			}
			switch dvap.DeviceType {
			case storageprovisioning.PlanDeviceTypeLocal:
				vap.DeviceType = internalstorage.DeviceTypeLocal
			case storageprovisioning.PlanDeviceTypeISCSI:
				vap.DeviceType = internalstorage.DeviceTypeISCSI
			}
			va.VolumeAttachmentPlan = &vap
		}
		if fs, ok := volumeMap[dva.VolumeUUID]; ok {
			if dva.Unit != nil {
				if fs.UnitAttachments == nil {
					fs.UnitAttachments = map[unit.Name]VolumeAttachment{}
				}
				fs.UnitAttachments[*dva.Unit] = va
			}
			if dva.Machine != nil {
				if fs.MachineAttachments == nil {
					fs.MachineAttachments = map[machine.Name]VolumeAttachment{}
				}
				fs.MachineAttachments[*dva.Machine] = va
			}
		}
	}

	ret := make([]Volume, 0, len(volumeMap))
	for _, v := range volumeMap {
		ret = append(ret, *v)
	}
	return ret, nil
}
