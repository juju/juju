// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// StorageState provides access to storage related state methods.
type StorageState interface {
	// SetFilesystemStatus sets the given filesystem status, overwriting any
	// current status data. The following errors can be expected:
	// - [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
	SetFilesystemStatus(
		ctx context.Context,
		filesystemUUID storage.FilesystemUUID,
		sts status.StatusInfo[status.StorageFilesystemStatusType],
	) error

	// GetFilesystemUUIDByID returns the filesystem UUID for the given
	// filesystem unique id. If no filesystem is found, an error satisfying
	// [storageerrors.FilesystemNotFound] is returned.
	GetFilesystemUUIDByID(ctx context.Context, id string) (storage.FilesystemUUID, error)

	// SetVolumeStatus sets the given volume status, overwriting any
	// current status data. The following errors can be expected:
	// - [storageerrors.VolumeNotFound] if the volume doesn't exist.
	SetVolumeStatus(
		ctx context.Context,
		volumeUUID storage.VolumeUUID,
		sts status.StatusInfo[status.StorageVolumeStatusType],
	) error

	// GetVolumeUUIDByID returns the volume UUID for the given
	// volume unique id. If no volume is found, an error satisfying
	// [storageerrors.VolumeNotFound] is returned.
	GetVolumeUUIDByID(ctx context.Context, id string) (storage.VolumeUUID, error)

	// GetStorageInstances returns the specified storage instances.
	GetStorageInstances(
		ctx context.Context, uuids []storage.StorageInstanceUUID,
	) ([]status.StorageInstance, error)

	// GetAllStorageInstances returns all the storage instances for this model.
	GetAllStorageInstances(ctx context.Context) ([]status.StorageInstance, error)

	// GetStorageInstanceAttachments returns the specified storage instance
	// attachments.
	GetStorageInstanceAttachments(
		ctx context.Context, uuids []storage.StorageInstanceUUID,
	) ([]status.StorageAttachment, error)

	// GetAllStorageInstanceAttachments returns all the storage instance
	// attachments for this model.
	GetAllStorageInstanceAttachments(ctx context.Context) ([]status.StorageAttachment, error)

	// GetFilesystems returns the specified filesystems for this model.
	GetFilesystems(
		ctx context.Context, uuids []storageprovisioning.FilesystemUUID,
	) ([]status.Filesystem, error)

	// GetAllFilesystems returns all the filesystems for this model.
	GetAllFilesystems(ctx context.Context) ([]status.Filesystem, error)

	// GetFilesystemAttachments returns the specifeid filesystem attachments.
	GetFilesystemAttachments(
		ctx context.Context, uuids []storageprovisioning.FilesystemUUID,
	) ([]status.FilesystemAttachment, error)

	// GetAllFilesystemAttachments returns all the filesystem attachments for this
	// model.
	GetAllFilesystemAttachments(ctx context.Context) ([]status.FilesystemAttachment, error)

	// GetVolumes returns the specified volumes.
	GetVolumes(
		ctx context.Context, uuids []storageprovisioning.VolumeUUID,
	) ([]status.Volume, error)

	// GetAllVolumes returns all the volumes for this model.
	GetAllVolumes(ctx context.Context) ([]status.Volume, error)

	// GetVolumeAttachments returns the specified volume attachments.
	GetVolumeAttachments(
		ctx context.Context, uuids []storageprovisioning.VolumeUUID,
	) ([]status.VolumeAttachment, error)

	// GetAllVolumeAttachments returns all the volume attachments for this model.
	GetAllVolumeAttachments(ctx context.Context) ([]status.VolumeAttachment, error)

	// GetBlockDevices returns the specified block devices.
	GetBlockDevices(
		ctx context.Context, uuids []blockdevice.BlockDeviceUUID,
	) (map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice, error)

	// GetAllAttachedBlockDevices returns all the block devices that are
	// attached via a volume attachment.
	GetAllAttachedBlockDevices(
		ctx context.Context,
	) (map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice, error)
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

// GetStorageInstanceStatuses returns the specified storage instance statuses.
func (s *Service) GetStorageInstanceStatuses(
	ctx context.Context, uuids []storage.StorageInstanceUUID,
) ([]StorageInstance, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(uuids) == 0 {
		return nil, nil
	}

	storageInstances, err := s.modelState.GetStorageInstances(ctx, uuids)
	if err != nil {
		return nil, errors.Capture(err)
	}
	storageAttachments, err := s.modelState.GetStorageInstanceAttachments(
		ctx, uuids)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var blockDeviceUUIDs []blockdevice.BlockDeviceUUID
	for _, v := range storageAttachments {
		if v.VolumeBlockDevice != nil {
			blockDeviceUUIDs = append(blockDeviceUUIDs, *v.VolumeBlockDevice)
		}
	}
	blockDevices, err := s.modelState.GetBlockDevices(ctx, blockDeviceUUIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.transformStorageInstanceResults(
		storageInstances, storageAttachments, blockDevices)
}

// GetAllStorageInstanceStatuses returns all the storage instance statuses for
// the model.
func (s *Service) GetAllStorageInstanceStatuses(
	ctx context.Context,
) ([]StorageInstance, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	storageInstances, err := s.modelState.GetAllStorageInstances(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	storageAttachments, err := s.modelState.GetAllStorageInstanceAttachments(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	blockDevices, err := s.modelState.GetAllAttachedBlockDevices(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.transformStorageInstanceResults(
		storageInstances, storageAttachments, blockDevices)
}

func (s *Service) transformStorageInstanceResults(
	storageInstances []status.StorageInstance,
	storageAttachments []status.StorageAttachment,
	blockDevices map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
) ([]StorageInstance, error) {
	storageMap := map[storage.StorageInstanceUUID]*StorageInstance{}
	for _, dsi := range storageInstances {
		si := StorageInstance{
			UUID:  dsi.UUID,
			ID:    dsi.ID,
			Kind:  dsi.Kind,
			Owner: dsi.Owner,
		}
		var err error
		si.Life, err = dsi.Life.Value()
		if err != nil {
			return nil, errors.Capture(err)
		}
		switch dsi.Kind {
		case storage.StorageKindBlock:
			si.Status, err = decodeVolumeStatus(dsi.VolumeStatus)
			if err != nil {
				return nil, errors.Capture(err)
			}
		case storage.StorageKindFilesystem:
			si.Status, err = decodeFilesystemStatus(dsi.FilesystemStatus)
			if err != nil {
				return nil, errors.Capture(err)
			}
		default:
			si.Status = corestatus.StatusInfo{
				Status: corestatus.Unknown,
			}
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
			switch si.Kind {
			case storage.StorageKindBlock:
				if dsa.VolumeBlockDevice != nil {
					blockDevice, ok := blockDevices[*dsa.VolumeBlockDevice]
					if ok {
						sa.Location = blockdevice.IDLink(blockDevice.DeviceLinks)
					}
				}
			case storage.StorageKindFilesystem:
				if dsa.FilesystemMountPoint != nil {
					sa.Location = *dsa.FilesystemMountPoint
				}
			}
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

// GetFilesystemStatuses returns the specified filesystem statuses.
func (s *Service) GetFilesystemStatuses(
	ctx context.Context, uuids []storageprovisioning.FilesystemUUID,
) ([]Filesystem, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(uuids) == 0 {
		return nil, nil
	}

	filesystems, err := s.modelState.GetFilesystems(ctx, uuids)
	if err != nil {
		return nil, errors.Capture(err)
	}
	filesystemAttachments, err := s.modelState.GetFilesystemAttachments(
		ctx, uuids)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.transformFilesystemResults(filesystems, filesystemAttachments)
}

// GetAllFilesystemStatuses returns all the filesystem statuses for the model.
func (s *Service) GetAllFilesystemStatuses(ctx context.Context) ([]Filesystem, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	filesystems, err := s.modelState.GetAllFilesystems(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	filesystemAttachments, err := s.modelState.GetAllFilesystemAttachments(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.transformFilesystemResults(filesystems, filesystemAttachments)
}

func (s *Service) transformFilesystemResults(
	filesystems []status.Filesystem,
	filesystemAttachments []status.FilesystemAttachment,
) ([]Filesystem, error) {
	fsMap := map[storageprovisioning.FilesystemUUID]*Filesystem{}
	for _, dfs := range filesystems {
		fs := Filesystem{
			UUID:        dfs.UUID,
			StorageUUID: dfs.StorageUUID,
			ID:          dfs.ID,
			StorageID:   dfs.StorageID,
			VolumeID:    dfs.VolumeID,
			ProviderID:  dfs.ProviderID,
			SizeMiB:     dfs.SizeMiB,
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

// GetVolumeStatuses returns the specified volume statuses.
func (s *Service) GetVolumeStatuses(
	ctx context.Context, uuids []storageprovisioning.VolumeUUID,
) ([]Volume, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	volumes, err := s.modelState.GetVolumes(ctx, uuids)
	if err != nil {
		return nil, errors.Capture(err)
	}
	volumeAttachments, err := s.modelState.GetVolumeAttachments(ctx, uuids)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.transformVolumeResults(volumes, volumeAttachments)
}

// GetAllVolumeStatuses returns all the volume statuses for the model.
func (s *Service) GetAllVolumeStatuses(ctx context.Context) ([]Volume, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	volumes, err := s.modelState.GetAllVolumes(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	volumeAttachments, err := s.modelState.GetAllVolumeAttachments(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.transformVolumeResults(volumes, volumeAttachments)
}

func (s *Service) transformVolumeResults(
	volumes []status.Volume,
	volumeAttachments []status.VolumeAttachment,
) ([]Volume, error) {
	volumeMap := map[storageprovisioning.VolumeUUID]*Volume{}
	for _, dv := range volumes {
		v := Volume{
			UUID:        dv.UUID,
			StorageUUID: dv.StorageUUID,
			ID:          dv.ID,
			StorageID:   dv.StorageID,
			ProviderID:  dv.ProviderID,
			HardwareID:  dv.HardwareID,
			WWN:         dv.WWN,
			SizeMiB:     dv.SizeMiB,
			Persistent:  dv.Persistent,
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
				DeviceType:       dvap.DeviceType,
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
