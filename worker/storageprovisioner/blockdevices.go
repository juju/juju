// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/params"
)

// machineBlockDevicesChanged is called when the block devices of the scoped
// machine have been seen to have changed. This triggers a refresh of all
// block devices for attached volumes backing pending filesystems.
func machineBlockDevicesChanged(ctx *context) error {
	if len(ctx.pendingFilesystems) == 0 {
		return nil
	}
	volumeTags := make([]names.VolumeTag, 0, len(ctx.pendingFilesystems))
	for _, params := range ctx.pendingFilesystems {
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
		logger.Tracef("no pending volume block devices")
		return nil
	}
	volumeTags := make([]names.VolumeTag, len(ctx.pendingVolumeBlockDevices))
	for i, tag := range ctx.pendingVolumeBlockDevices.SortedValues() {
		volumeTags[i] = tag.(names.VolumeTag)
	}
	// Clear out the pending set, so we don't force-refresh again.
	ctx.pendingVolumeBlockDevices = set.NewTags()
	return refreshVolumeBlockDevices(ctx, volumeTags)
}

// refreshVolumeBlockDevices refreshes the block devices for the specified
// volumes.
func refreshVolumeBlockDevices(ctx *context, volumeTags []names.VolumeTag) error {
	machineTag, ok := ctx.scope.(names.MachineTag)
	if !ok {
		// This function should only be called by machine-scoped
		// storage provisioners.
		panic(errors.New("expected machine tag"))
	}
	ids := make([]params.MachineStorageId, len(volumeTags))
	for i, volumeTag := range volumeTags {
		ids[i] = params.MachineStorageId{
			MachineTag:    machineTag.String(),
			AttachmentTag: volumeTag.String(),
		}
	}
	results, err := ctx.volumeAccessor.VolumeBlockDevices(ids)
	if err != nil {
		return errors.Annotate(err, "refreshing volume block devices")
	}
	for i, result := range results {
		if result.Error == nil {
			ctx.volumeBlockDevices[volumeTags[i]] = result.Result
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
