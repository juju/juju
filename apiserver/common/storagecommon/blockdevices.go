// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"context"
	"strings"

	"github.com/juju/juju/core/blockdevice"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.apiserver.storagecommon")

// MatchingVolumeBlockDevice finds the block device that matches the
// provided volume info and volume attachment info.
func MatchingVolumeBlockDevice(
	ctx context.Context,
	blockDevices []blockdevice.BlockDevice,
	volumeInfo state.VolumeInfo,
	attachmentInfo state.VolumeAttachmentInfo,
	planBlockInfo blockdevice.BlockDevice,
) (*blockdevice.BlockDevice, bool) {
	return matchingBlockDevice(ctx, blockDevices, volumeInfo, attachmentInfo, planBlockInfo, false)
}

// MatchingFilesystemBlockDevice finds the block device that matches the
// provided volume info and volume attachment info, preferring a matching
// device of type partition.
func MatchingFilesystemBlockDevice(
	ctx context.Context,
	blockDevices []blockdevice.BlockDevice,
	volumeInfo state.VolumeInfo,
	attachmentInfo state.VolumeAttachmentInfo,
	planBlockInfo blockdevice.BlockDevice,
) (*blockdevice.BlockDevice, bool) {
	return matchingBlockDevice(ctx, blockDevices, volumeInfo, attachmentInfo, planBlockInfo, true)
}

func matchingBlockDevice(
	ctx context.Context,
	blockDevices []blockdevice.BlockDevice,
	volumeInfo state.VolumeInfo,
	attachmentInfo state.VolumeAttachmentInfo,
	planBlockInfo blockdevice.BlockDevice,
	allowPartitions bool,
) (*blockdevice.BlockDevice, bool) {
	logger.Tracef(ctx, "looking for block device to match one of planBlockInfo %#v volumeInfo %#v attachmentInfo %#v",
		planBlockInfo, volumeInfo, attachmentInfo)

	if planBlockInfo.HardwareId != "" {
		for _, dev := range blockDevices {
			if planBlockInfo.HardwareId == dev.HardwareId {
				logger.Tracef(ctx, "plan hwid match on %v", planBlockInfo.HardwareId)
				return &dev, true
			}
		}
		logger.Tracef(ctx, "no match for block device hardware id: %v", planBlockInfo.HardwareId)
	}

	if planBlockInfo.WWN != "" {
		for _, dev := range blockDevices {
			if planBlockInfo.WWN == dev.WWN {
				logger.Tracef(ctx, "plan wwn match on %v", planBlockInfo.WWN)
				return &dev, true
			}
		}
		logger.Tracef(ctx, "no match for block device wwn: %v", planBlockInfo.WWN)
	}

	if planBlockInfo.DeviceName != "" {
		for _, dev := range blockDevices {
			if planBlockInfo.DeviceName == dev.DeviceName {
				logger.Tracef(ctx, "plan device name match on %v", planBlockInfo.DeviceName)
				return &dev, true
			}
		}
		logger.Tracef(ctx, "no match for block device name: %v", planBlockInfo.DeviceName)
	}

	if volumeInfo.WWN != "" {
		for _, dev := range blockDevices {
			if volumeInfo.WWN == dev.WWN {
				logger.Tracef(ctx, "wwn match on %v", volumeInfo.WWN)
				return &dev, true
			}
		}
		logger.Tracef(ctx, "no match for block device wwn: %v", volumeInfo.WWN)
	}

	if volumeInfo.HardwareId != "" {
		for _, dev := range blockDevices {
			if volumeInfo.HardwareId == dev.HardwareId {
				logger.Tracef(ctx, "hwid match on %v", volumeInfo.HardwareId)
				return &dev, true
			}
		}
		logger.Tracef(ctx, "no match for block device hardware id: %v", volumeInfo.HardwareId)
	}

	if volumeInfo.VolumeId != "" {
		for _, dev := range blockDevices {
			if dev.SerialId != "" && strings.HasPrefix(volumeInfo.VolumeId, dev.SerialId) {
				logger.Tracef(ctx, "serial id %v match on volume id %v", dev.SerialId, volumeInfo.VolumeId)
				return &dev, true
			}
		}
		logger.Tracef(ctx, "no match for block device volume id: %v", volumeInfo.VolumeId)
	}

	if attachmentInfo.BusAddress != "" {
		for _, dev := range blockDevices {
			if attachmentInfo.BusAddress == dev.BusAddress {
				logger.Tracef(ctx, "bus address match on %v", attachmentInfo.BusAddress)
				return &dev, true
			}
		}
		logger.Tracef(ctx, "no match for block device bus address: %v", attachmentInfo.BusAddress)
	}

	if attachmentInfo.DeviceLink != "" {
		// We'll prefer to use a mounted partition if available.
		// This will be the case for filesystem storage; it will be part1.
		var devWithUUID, parentDev *blockdevice.BlockDevice
	devMatch:
		for _, dev := range blockDevices {
			for _, link := range dev.DeviceLinks {
				if attachmentInfo.DeviceLink == link || (allowPartitions && attachmentInfo.DeviceLink+"-part1" == link) {
					devCopy := dev
					if dev.UUID != "" && allowPartitions {
						devWithUUID = &devCopy
					} else {
						parentDev = &devCopy
					}
				}
				if devWithUUID != nil {
					break devMatch
				}
			}
		}
		if devWithUUID != nil {
			logger.Tracef(ctx, "device link with UUID match on %v", attachmentInfo.DeviceLink)
			return devWithUUID, true
		}
		if parentDev != nil {
			logger.Tracef(ctx, "device link without UUID match on %v", attachmentInfo.DeviceLink)
			return parentDev, true
		}
		logger.Tracef(ctx, "no match for block device dev link: %v", attachmentInfo.DeviceLink)
	}

	if attachmentInfo.DeviceName != "" {
		for _, dev := range blockDevices {
			if attachmentInfo.DeviceName == dev.DeviceName {
				logger.Tracef(ctx, "device name match on %v", attachmentInfo.DeviceName)
				return &dev, true
			}
		}
		logger.Tracef(ctx, "no match for block device name: %v", attachmentInfo.DeviceName)
	}
	return nil, false
}
