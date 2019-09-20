// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"strings"

	"github.com/juju/loggo"

	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.apiserver.storagecommon")

// BlockDeviceFromState translates a state.BlockDeviceInfo to a
// storage.BlockDevice.
func BlockDeviceFromState(in state.BlockDeviceInfo) storage.BlockDevice {
	return storage.BlockDevice{
		DeviceName:     in.DeviceName,
		DeviceLinks:    in.DeviceLinks,
		Label:          in.Label,
		UUID:           in.UUID,
		HardwareId:     in.HardwareId,
		WWN:            in.WWN,
		BusAddress:     in.BusAddress,
		Size:           in.Size,
		FilesystemType: in.FilesystemType,
		InUse:          in.InUse,
		MountPoint:     in.MountPoint,
		SerialId:       in.SerialId,
	}
}

// MatchingBlockDevice finds the block device that matches the
// provided volume info and volume attachment info.
func MatchingBlockDevice(
	blockDevices []state.BlockDeviceInfo,
	volumeInfo state.VolumeInfo,
	attachmentInfo state.VolumeAttachmentInfo,
	planBlockInfo state.BlockDeviceInfo,
) (*state.BlockDeviceInfo, bool) {
	logger.Tracef("looking for block device for volume %#v", volumeInfo)
	for _, dev := range blockDevices {
		if planBlockInfo.HardwareId != "" {
			if planBlockInfo.HardwareId == dev.HardwareId {
				logger.Tracef("plan hwid match on %v", volumeInfo.HardwareId)
				return &dev, true
			}
		}
		if planBlockInfo.WWN != "" {
			if planBlockInfo.WWN == dev.WWN {
				logger.Tracef("plan wwn match on %v", volumeInfo.WWN)
				return &dev, true
			}
			continue
		}
		if planBlockInfo.DeviceName != "" {
			if planBlockInfo.DeviceName == dev.DeviceName {
				logger.Tracef("plan device name match on %v", attachmentInfo.DeviceName)
				return &dev, true
			}
			continue
		}
		if volumeInfo.WWN != "" {
			if volumeInfo.WWN == dev.WWN {
				logger.Tracef("wwn match on %v", volumeInfo.WWN)
				return &dev, true
			}
			logger.Tracef("no match for block device WWN: %v", dev.WWN)
			continue
		}
		if volumeInfo.HardwareId != "" {
			if volumeInfo.HardwareId == dev.HardwareId {
				logger.Tracef("hwid match on %v", volumeInfo.HardwareId)
				return &dev, true
			}
			logger.Tracef("no match for block device hardware id: %v", dev.HardwareId)
			continue
		}
		if volumeInfo.VolumeId != "" && dev.SerialId != "" {
			if strings.HasPrefix(volumeInfo.VolumeId, dev.SerialId) {
				logger.Tracef("serial id %v match on volume id %v", dev.SerialId, volumeInfo.VolumeId)
				return &dev, true
			}
			logger.Tracef("no match for block device serial id: %v", dev.SerialId)
			continue
		}
		if attachmentInfo.BusAddress != "" {
			if attachmentInfo.BusAddress == dev.BusAddress {
				logger.Tracef("bus address match on %v", attachmentInfo.BusAddress)
				return &dev, true
			}
			logger.Tracef("no match for block device bus address: %v", dev.BusAddress)
			continue
		}
		// Only match on block device link if the block device is published
		// with device link information.
		if attachmentInfo.DeviceLink != "" && len(dev.DeviceLinks) > 0 {
			for _, link := range dev.DeviceLinks {
				if attachmentInfo.DeviceLink == link {
					logger.Tracef("device link match on %v", attachmentInfo.DeviceLink)
					return &dev, true
				}
			}
			logger.Tracef("no match for block device dev links: %v", dev.DeviceLinks)
			continue
		}
		if attachmentInfo.DeviceName == dev.DeviceName {
			logger.Tracef("device name match on %v", attachmentInfo.DeviceName)
			return &dev, true
		}
		logger.Tracef("no match for block device name: %v", dev.DeviceName)
	}
	return nil, false
}
