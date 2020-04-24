// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
)

// machineBlockDevicesChanged is called when the block devices of the scoped
// machine have been seen to have changed. This triggers a refresh of all
// block devices for attached volumes backing pending filesystems.
func machineBlockDevicesChanged(ctx *context) error {
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
	}
	if len(volumeTags) == 0 {
		return nil
	}
	return refreshVolumeBlockDevices(ctx, volumeTags)
}

// processPendingVolumeBlockDevices is called before waiting for any events,
// to force a block-device query for any volumes for which we have not
// previously observed block devices.
func processPendingVolumeBlockDevices(ctx *context) error {
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
	return refreshVolumeBlockDevices(ctx, volumeTags)
}

// refreshVolumeBlockDevices refreshes the block devices for the specified
// volumes.
func refreshVolumeBlockDevices(ctx *context, volumeTags []names.VolumeTag) error {
	machineTag, ok := ctx.config.Scope.(names.MachineTag)
	if !ok {
		// This function should only be called by machine-scoped
		// storage provisioners.
		ctx.config.Logger.Warningf("refresh block devices, expected machine tag, got %v", ctx.config.Scope)
		return nil
	}
	ids := make([]params.MachineStorageId, len(volumeTags))
	for i, volumeTag := range volumeTags {
		ids[i] = params.MachineStorageId{
			MachineTag:    machineTag.String(),
			AttachmentTag: volumeTag.String(),
		}
	}
	results, err := ctx.config.Volumes.VolumeBlockDevices(ids)
	if err != nil {
		return errors.Annotate(err, "refreshing volume block devices")
	}
	for i, result := range results {
		if result.Error == nil {
			ctx.volumeBlockDevices[volumeTags[i]] = result.Result
			for _, params := range ctx.incompleteFilesystemParams {
				if params.Volume == volumeTags[i] {
					updatePendingFilesystem(ctx, params)
				}
			}
			for id, params := range ctx.incompleteFilesystemAttachmentParams {
				filesystem, ok := ctx.filesystems[params.Filesystem]
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
			return errors.Annotatef(
				err, "getting block device info for volume attachment %v",
				ids[i],
			)
		}
	}
	return nil
}
