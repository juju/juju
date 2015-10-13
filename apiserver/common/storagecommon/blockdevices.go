// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// BlockDeviceFromState translates a state.BlockDeviceInfo to a
// storage.BlockDevice.
func BlockDeviceFromState(in state.BlockDeviceInfo) storage.BlockDevice {
	return storage.BlockDevice{
		in.DeviceName,
		in.DeviceLinks,
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
			continue
		}
		if attachmentInfo.BusAddress != "" {
			if attachmentInfo.BusAddress == dev.BusAddress {
				return &dev, true
			}
			continue
		}
		if attachmentInfo.DeviceLink != "" {
			for _, link := range dev.DeviceLinks {
				if attachmentInfo.DeviceLink == link {
					return &dev, true
				}
			}
			continue
		}
		if attachmentInfo.DeviceName == dev.DeviceName {
			return &dev, true
		}
	}
	return nil, false
}
