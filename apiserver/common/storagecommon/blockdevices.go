// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.apiserver.storagecommon")

// BlockDeviceFromState translates a state.BlockDeviceInfo to a
// storage.BlockDevice.
func BlockDeviceFromState(in state.BlockDeviceInfo) storage.BlockDevice {
	return storage.BlockDevice{
		in.DeviceName,
		in.DeviceLinks,
		in.Label,
		in.UUID,
		in.HardwareId,
		in.WWN,
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
	logger.Tracef("looking for block device for volume %#v", volumeInfo)
	for _, dev := range blockDevices {
		if volumeInfo.WWN != "" {
			if volumeInfo.WWN == dev.WWN {
				return &dev, true
			}
			logger.Tracef("no match for block device WWN: %v", dev.WWN)
			continue
		}
		if volumeInfo.HardwareId != "" {
			if volumeInfo.HardwareId == dev.HardwareId {
				return &dev, true
			}
			logger.Tracef("no match for block device hardware id: %v", dev.HardwareId)
			continue
		}
		if attachmentInfo.BusAddress != "" {
			if attachmentInfo.BusAddress == dev.BusAddress {
				return &dev, true
			}
			logger.Tracef("no match for block device bus address: %v", dev.BusAddress)
			continue
		}
		if attachmentInfo.DeviceLink != "" {
			for _, link := range dev.DeviceLinks {
				if attachmentInfo.DeviceLink == link {
					return &dev, true
				}
			}
			logger.Tracef("no match for block device dev links: %v", dev.DeviceLinks)
			continue
		}
		if attachmentInfo.DeviceName == dev.DeviceName {
			return &dev, true
		}
		logger.Tracef("no match for block device name: %v", dev.DeviceName)
	}
	return nil, false
}
