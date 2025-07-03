// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	stdcontext "context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

// machineBlockDevicesChanged is called when the block devices of the scoped
// machine have been seen to have changed. This triggers a refresh of all
// block devices for attached volumes backing pending filesystems.
func machineBlockDevicesChanged(ctx *context) error {
	ctx.config.Logger.Debugf("alvin machineBlockDevicesChanged incompleteFilesystemParams: %+v", ctx.incompleteFilesystemParams)
	volumeTags := make([]names.VolumeTag, 0, len(ctx.incompleteFilesystemParams))
	// We must query volumes for both incomplete filesystems
	// and incomplete filesystem attachments, because even
	// though a filesystem attachment cannot exist without a
	// filesystem, the filesystem may be created and attached
	// in different sessions, and there is no guarantee that
	// the block device will remain attached to the machine
	// in between.
	for _, params := range ctx.incompleteFilesystemParams {
		if params.Volume == (names.VolumeTag{}) {
			// Filesystem is not volume-backed.
			continue
		}
		if _, ok := ctx.volumeBlockDevices[params.Volume]; ok {
			// Backing-volume's block device is already attached.
			continue
		}
		volumeTags = append(volumeTags, params.Volume)
	}
	for _, params := range ctx.incompleteFilesystemAttachmentParams {
		filesystem, ok := ctx.filesystems[params.Filesystem]
		ctx.config.Logger.Debugf("alvin machineBlockDevicesChanged params: %+v | filesystem: %+v", params, filesystem)

		if !ok {
			continue
		}
		if filesystem.Volume == (names.VolumeTag{}) {
			// Filesystem is not volume-backed.
			continue
		}
		if _, ok := ctx.volumeBlockDevices[filesystem.Volume]; ok {
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
		ctx.config.Logger.Debugf("alvin machineBlockDevicesChanged found: %+v | filesystem.Volume: %+v", found, filesystem.Volume)

	}
	// Gather any already attached volume backed filesystem attachments
	// so we can see if the UUID of the attachment has been newly set.
	mountedAttachments := make([]storage.FilesystemAttachmentParams, 0, len(ctx.filesystemAttachments))
	for _, attach := range ctx.filesystemAttachments {
		filesystem, ok := ctx.filesystems[attach.Filesystem]
		if !ok {
			continue
		}
		if filesystem.Volume == (names.VolumeTag{}) {
			// Filesystem is not volume-backed.
			continue
		}
		if _, ok := ctx.volumeBlockDevices[filesystem.Volume]; !ok {
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
	updatedVolumes, err := refreshVolumeBlockDevices(ctx, volumeTags)
	if err != nil {
		return errors.Trace(err)
	}
	ctx.config.Logger.Debugf("alvin updatedVolumes: %+v ", updatedVolumes)

	// For filesystems backed by volumes (managed filesystems), we re-run the attachment logic
	// to allow for the fact that the mount (and its UUID) may have become available after
	// we noticed that the volume appeared.
	volumes := set.NewStrings()
	for _, v := range updatedVolumes {
		volumes.Add(v.String())
	}
	var toUpdate []storage.FilesystemAttachmentParams
	for _, a := range mountedAttachments {
		filesystem, ok := ctx.filesystems[a.Filesystem]
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
	ctx.config.Logger.Debugf("refreshing mounted filesystems: %#v", toUpdate)
	_, err = ctx.managedFilesystemSource.AttachFilesystems(ctx.config.CloudCallContextFunc(stdcontext.Background()), toUpdate)
	return err
}

// processPendingVolumeBlockDevices is called before waiting for any events,
// to force a block-device query for any volumes for which we have not
// previously observed block devices.
func processPendingVolumeBlockDevices(ctx *context) error {
	ctx.config.Logger.Debugf("alvin processPendingVolumeBlockDevices called")
	if len(ctx.pendingVolumeBlockDevices) == 0 {
		ctx.config.Logger.Tracef("no pending volume block devices")
		return nil
	}
	volumeTags := make([]names.VolumeTag, len(ctx.pendingVolumeBlockDevices))
	for i, tag := range ctx.pendingVolumeBlockDevices.SortedValues() {
		volumeTags[i] = tag.(names.VolumeTag)
	}
	// Clear out the pending set, so we don't force-refresh again.
	ctx.pendingVolumeBlockDevices = names.NewSet()
	_, err := refreshVolumeBlockDevices(ctx, volumeTags)
	return err
}

// refreshVolumeBlockDevices refreshes the block devices for the specified volumes.
// It returns any volumes which have had the UUID newly set.
func refreshVolumeBlockDevices(ctx *context, volumeTags []names.VolumeTag) ([]names.VolumeTag, error) {
	ctx.config.Logger.Debugf("alvin refreshVolumeBlockDevices called")
	machineTag, ok := ctx.config.Scope.(names.MachineTag)
	if !ok {
		// This function should only be called by machine-scoped
		// storage provisioners.
		ctx.config.Logger.Warningf("refresh block devices, expected machine tag, got %v", ctx.config.Scope)
		return nil, nil
	}
	ids := make([]params.MachineStorageId, len(volumeTags))
	for i, volumeTag := range volumeTags {
		ids[i] = params.MachineStorageId{
			MachineTag:    machineTag.String(),
			AttachmentTag: volumeTag.String(),
		}
	}
	ctx.config.Logger.Debugf("alvin refreshVolumeBlockDevices ids: %+v", ids)
	var volumesWithUpdatedUUID []names.VolumeTag
	results, err := ctx.config.Volumes.VolumeBlockDevices(ids)
	if err != nil {
		ctx.config.Logger.Debugf("alvin refreshVolumeBlockDevices VolumeBlockDevices err: %+v", err)
		return nil, errors.Annotate(err, "refreshing volume block devices")
	}
	ctx.config.Logger.Debugf("alvin refreshVolumeBlockDevices VolumeBlockDevices results: %+v", results)

	for i, result := range results {
		ctx.config.Logger.Debugf("alvin refreshVolumeBlockDevices VolumeBlockDevices result: %+v", result)
		if result.Error == nil {
			existing, ok := ctx.volumeBlockDevices[volumeTags[i]]
			if ok && existing.UUID == "" && result.Result.UUID != "" {
				volumesWithUpdatedUUID = append(volumesWithUpdatedUUID, volumeTags[i])
			}
			ctx.volumeBlockDevices[volumeTags[i]] = result.Result
			for _, params := range ctx.incompleteFilesystemParams {
				ctx.config.Logger.Debugf("alvin refreshVolumeBlockDevices incompleteFilesystemParams volume: %+v", params.Volume)
				ctx.config.Logger.Debugf("alvin refreshVolumeBlockDevices incompleteFilesystemParams volumeTag: %+v", volumeTags[i])
				if params.Volume == volumeTags[i] {
					updatePendingFilesystem(ctx, params)
				}
			}
			for id, params := range ctx.incompleteFilesystemAttachmentParams {
				filesystem, ok := ctx.filesystems[params.Filesystem]
				ctx.config.Logger.Debugf("alvin refreshVolumeBlockDevices incompleteFilesystemAttachmentParams params: %+v", params)
				ctx.config.Logger.Debugf("alvin refreshVolumeBlockDevices incompleteFilesystemAttachmentParams filesystem: %+v", filesystem)
				if !ok {
					continue
				}
				if filesystem.Volume == volumeTags[i] {
					updatePendingFilesystemAttachment(ctx, id, params)
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
