// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// BlockDeviceFromState translates a state.BlockDeviceInfo to a
// storage.BlockDevice.
func BlockDeviceFromState(in state.BlockDeviceInfo) storage.BlockDevice {
	return storage.BlockDevice{
		in.DeviceName,
		in.Label,
		in.UUID,
		in.HardwareId,
		in.BusAddress,
		in.Size,
		in.FilesystemType,
		in.InUse,
		in.MountPoint,
	}
}

// MatchingBlockDevice finds the block device that matches the
// provided volume info and volume attachment info.
func MatchingBlockDevice(
	blockDevices []state.BlockDeviceInfo,
	volumeInfo state.VolumeInfo,
	attachmentInfo state.VolumeAttachmentInfo,
) (*state.BlockDeviceInfo, bool) {
	for _, dev := range blockDevices {
		if volumeInfo.HardwareId != "" {
			if volumeInfo.HardwareId == dev.HardwareId {
				return &dev, true
			}
		} else if attachmentInfo.BusAddress != "" {
			if attachmentInfo.BusAddress == dev.BusAddress {
				return &dev, true
			}
		} else if attachmentInfo.DeviceName == dev.DeviceName {
			return &dev, true
		}
	}
	return nil, false
}
