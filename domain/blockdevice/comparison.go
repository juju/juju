// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice

import (
	"strings"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/blockdevice"
)

const (
	DevLinkByID        = "/dev/disk/by-id/"
	DevLinkByPartUUID  = "/dev/disk/by-partuuid/"
	DevLinkByPartLabel = "/dev/disk/by-partlabel/"
	DevLinkByFSUUID    = "/dev/disk/by-uuid/"
)

// SameDevice returns true if both devices are the same device by using stable
// and strong identifiers first then falling back safely to other identifiers.
// This function is designed to be used against a full set or sets of block
// devices.
func SameDevice(left, right blockdevice.BlockDevice) bool {
	// If one of the provided block devices only has a name, we can compare on
	// just the name.
	if IsNameOnly(left) || IsNameOnly(right) {
		return left.DeviceName == right.DeviceName
	}
	// If the block devices share a strong /dev/ link, this is enough to assert
	// that they are the same.
	leftDevLinks := set.NewStrings(left.DeviceLinks...)
	rightDevLinks := set.NewStrings(right.DeviceLinks...)
	commonDevLinks := leftDevLinks.Intersection(rightDevLinks)
	for link := range commonDevLinks {
		if strings.HasPrefix(link, DevLinkByID) {
			return true
		} else if strings.HasPrefix(link, DevLinkByPartUUID) {
			return true
		} else if strings.HasPrefix(link, DevLinkByFSUUID) {
			return true
		}
	}
	// If either of the devices looks like a partition, they should have matched
	// by this point. Since partitions inherit WWN, SerialID etc from their
	// parent disk, it is not possible to compare them any further on those
	// values.
	if IsPartition(left) || IsPartition(right) {
		return false
	}
	// WWN is the strongest of the identifiers provided for a disk block device.
	// This identifier is likely derived from either `/dev/disk/by-id/wwn-{WWN}`
	// dev link or from lsblk/udevadm from `ID_WWN`.
	if left.WWN != "" && right.WWN != "" {
		return left.WWN == right.WWN
	}
	// HardwareId is both the bus name (i.e. scsi or ata) joined by a hyphen to
	// the serial id for a disk block device. It is derived from lsblk/udevadm
	// `ID_BUS` and `ID_SERIAL`.
	if left.HardwareId != "" && right.HardwareId != "" {
		return left.HardwareId == right.HardwareId
	}
	// SerialId is the serial id of the disk block device derived from lsblk/
	// udevadm `ID_SERIAL`.
	if left.SerialId != "" && right.SerialId != "" {
		return left.SerialId == right.SerialId
	}
	// BusAddress is only set by the iscsi attachment plan, it is in the form
	// `scsi@{H}:{C}.{T}.{L}" where HCTL refer to Host, Channel, Target and LUN.
	if left.BusAddress != "" && right.BusAddress != "" {
		return left.BusAddress == right.BusAddress
	}
	return false
}

// IsPartition returns true if the block device contains any device links that
// indicate that it is a partition.
func IsPartition(dev blockdevice.BlockDevice) bool {
	for _, link := range dev.DeviceLinks {
		if strings.HasPrefix(link, DevLinkByPartLabel) {
			return true
		} else if strings.HasPrefix(link, DevLinkByPartUUID) {
			return true
		} else if strings.HasPrefix(link, DevLinkByFSUUID) {
			return true
		}
	}
	return false
}

// IsNameOnly returns true when the block device has a name but is otherwise
// empty.
func IsNameOnly(dev blockdevice.BlockDevice) bool {
	if dev.DeviceName == "" {
		return false
	}
	dev.DeviceName = ""
	return IsEmpty(dev)
}

// IsEmpty returns true when the block device is an empty value.
func IsEmpty(dev blockdevice.BlockDevice) bool {
	return dev.BusAddress == "" &&
		len(dev.DeviceLinks) == 0 &&
		dev.DeviceName == "" &&
		dev.FilesystemType == "" &&
		dev.HardwareId == "" &&
		dev.InUse == false &&
		dev.FilesystemLabel == "" &&
		dev.MountPoint == "" &&
		dev.SizeMiB == 0 &&
		dev.FilesystemUUID == "" &&
		dev.WWN == ""
}
