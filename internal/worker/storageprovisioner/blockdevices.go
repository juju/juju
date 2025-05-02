// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

// machineBlockDevicesChanged is called when the block devices of the scoped
// machine have been seen to have changed. This triggers a refresh of all
// block devices for attached volumes backing pending filesystems.
func machineBlockDevicesChanged(ctx context.Context, deps *dependencies) error {
	volumeTags := make([]names.VolumeTag, 0, len(deps.incompleteFilesystemParams))
	// We must query volumes for both incomplete filesystems
	// and incomplete filesystem attachments, because even
	// though a filesystem attachment cannot exist without a
	// filesystem, the filesystem may be created and attached
	// in different sessions, and there is no guarantee that
	// the block device will remain attached to the machine
	// in between.
	for _, params := range deps.incompleteFilesystemParams {
		if params.Volume == (names.VolumeTag{}) {
			// Filesystem is not volume-backed.
			continue
		}
		if _, ok := deps.volumeBlockDevices[params.Volume]; ok {
			// Backing-volume's block device is already attached.
			continue
		}
		volumeTags = append(volumeTags, params.Volume)
	}
	for _, params := range deps.incompleteFilesystemAttachmentParams {
		filesystem, ok := deps.filesystems[params.Filesystem]
		if !ok {
			continue
		}
		if filesystem.Volume == (names.VolumeTag{}) {
			// Filesystem is not volume-backed.
			continue
		}
		if _, ok := deps.volumeBlockDevices[filesystem.Volume]; ok {
			// Backing-volume's block device is already attached.
			continue
		}
		var found bool
		for _, tag := range volumeTags {
			if filesystem.Volume == tag {
				found = true
				break
			}
		}
		if !found {
			volumeTags = append(volumeTags, filesystem.Volume)
		}
	}
	// Gather any already attached volume backed filesystem attachments
	// so we can see if the UUID of the attachment has been newly set.
	mountedAttachments := make([]storage.FilesystemAttachmentParams, 0, len(deps.filesystemAttachments))
	for _, attach := range deps.filesystemAttachments {
		filesystem, ok := deps.filesystems[attach.Filesystem]
		if !ok {
			continue
		}
		if filesystem.Volume == (names.VolumeTag{}) {
			// Filesystem is not volume-backed.
			continue
		}
		if _, ok := deps.volumeBlockDevices[filesystem.Volume]; !ok {
			continue
		}
		mountedAttachments = append(mountedAttachments, storage.FilesystemAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				ReadOnly: attach.ReadOnly,
			},
			Filesystem: attach.Filesystem,
			Path:       attach.Path,
		})
		var found bool
		for _, tag := range volumeTags {
			if filesystem.Volume == tag {
				found = true
				break
			}
		}
		if !found {
			volumeTags = append(volumeTags, filesystem.Volume)
		}
	}
	if len(volumeTags) == 0 {
		return nil
	}
	updatedVolumes, err := refreshVolumeBlockDevices(ctx, deps, volumeTags)
	if err != nil {
		return errors.Trace(err)
	}

	// For filesystems backed by volumes (managed filesystems), we re-run the attachment logic
	// to allow for the fact that the mount (and its UUID) may have become available after
	// we noticed that the volume appeared.
	volumes := set.NewStrings()
	for _, v := range updatedVolumes {
		volumes.Add(v.String())
	}
	var toUpdate []storage.FilesystemAttachmentParams
	for _, a := range mountedAttachments {
		filesystem, ok := deps.filesystems[a.Filesystem]
		if !ok {
			continue
		}
		if volumes.Contains(filesystem.Volume.String()) {
			toUpdate = append(toUpdate, a)
		}
	}
	if len(toUpdate) == 0 {
		return nil
	}
	deps.config.Logger.Debugf(ctx, "refreshing mounted filesystems: %#v", toUpdate)
	_, err = deps.managedFilesystemSource.AttachFilesystems(ctx, toUpdate)
	return err
}

// processPendingVolumeBlockDevices is called before waiting for any events,
// to force a block-device query for any volumes for which we have not
// previously observed block devices.
func processPendingVolumeBlockDevices(ctx context.Context, deps *dependencies) error {
	if len(deps.pendingVolumeBlockDevices) == 0 {
		deps.config.Logger.Tracef(ctx, "no pending volume block devices")
		return nil
	}
	volumeTags := make([]names.VolumeTag, len(deps.pendingVolumeBlockDevices))
	for i, tag := range deps.pendingVolumeBlockDevices.SortedValues() {
		volumeTags[i] = tag.(names.VolumeTag)
	}
	// Clear out the pending set, so we don't force-refresh again.
	deps.pendingVolumeBlockDevices = names.NewSet()
	_, err := refreshVolumeBlockDevices(ctx, deps, volumeTags)
	return err
}

// refreshVolumeBlockDevices refreshes the block devices for the specified volumes.
// It returns any volumes which have had the UUID newly set.
func refreshVolumeBlockDevices(ctx context.Context, deps *dependencies, volumeTags []names.VolumeTag) ([]names.VolumeTag, error) {
	machineTag, ok := deps.config.Scope.(names.MachineTag)
	if !ok {
		// This function should only be called by machine-scoped
		// storage provisioners.
		deps.config.Logger.Warningf(ctx, "refresh block devices, expected machine tag, got %v", deps.config.Scope)
		return nil, nil
	}
	ids := make([]params.MachineStorageId, len(volumeTags))
	for i, volumeTag := range volumeTags {
		ids[i] = params.MachineStorageId{
			MachineTag:    machineTag.String(),
			AttachmentTag: volumeTag.String(),
		}
	}
	var volumesWithUpdatedUUID []names.VolumeTag
	results, err := deps.config.Volumes.VolumeBlockDevices(ctx, ids)
	if err != nil {
		return nil, errors.Annotate(err, "refreshing volume block devices")
	}
	for i, result := range results {
		if result.Error == nil {
			existing, ok := deps.volumeBlockDevices[volumeTags[i]]
			if ok && existing.UUID == "" && result.Result.UUID != "" {
				volumesWithUpdatedUUID = append(volumesWithUpdatedUUID, volumeTags[i])
			}
			deps.volumeBlockDevices[volumeTags[i]] = blockDeviceFromParams(result.Result)
			for _, params := range deps.incompleteFilesystemParams {
				if params.Volume == volumeTags[i] {
					updatePendingFilesystem(deps, params)
				}
			}
			for id, params := range deps.incompleteFilesystemAttachmentParams {
				filesystem, ok := deps.filesystems[params.Filesystem]
				if !ok {
					continue
				}
				if filesystem.Volume == volumeTags[i] {
					updatePendingFilesystemAttachment(deps, id, params)
				}
			}
		} else if params.IsCodeNotProvisioned(result.Error) || params.IsCodeNotFound(result.Error) {
			// Either the volume (attachment) isn't provisioned,
			// or the corresponding block device is not yet known.
			//
			// Neither of these errors is fatal; we just wait for
			// the block device watcher to notify us again.
		} else {
			return nil, errors.Annotatef(
				err, "getting block device info for volume attachment %v",
				ids[i],
			)
		}
	}
	return volumesWithUpdatedUUID, nil
}

func blockDeviceFromParams(in params.BlockDevice) blockdevice.BlockDevice {
	return blockdevice.BlockDevice{
		DeviceName:     in.DeviceName,
		DeviceLinks:    in.DeviceLinks,
		Label:          in.Label,
		UUID:           in.UUID,
		HardwareId:     in.HardwareId,
		WWN:            in.WWN,
		BusAddress:     in.BusAddress,
		SizeMiB:        in.Size,
		FilesystemType: in.FilesystemType,
		InUse:          in.InUse,
		MountPoint:     in.MountPoint,
		SerialId:       in.SerialId,
	}
}
