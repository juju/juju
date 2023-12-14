// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice

// BlockDevice describes information about a block device.
type BlockDevice struct {
	// DeviceName is the block device's OS-specific name (e.g. "sdb").
	DeviceName string

	// DeviceLinks is a collection of symlinks to the block device
	// that the OS maintains (e.g. "/dev/disk/by-id/..."). Storage
	// provisioners can match volume attachments to device links if
	// they know ahead of time how the OS will name them.
	DeviceLinks []string

	// Label is the label for the filesystem on the block device.
	//
	// This will be empty if the block device does not have a filesystem,
	// or if the filesystem is not yet known to Juju.
	Label string

	// UUID is a unique identifier for the filesystem on the block device.
	//
	// This will be empty if the block device does not have a filesystem,
	// or if the filesystem is not yet known to Juju.
	//
	// The UUID format is not necessarily uniform; for example, LVM UUIDs
	// differ in format to the standard v4 UUIDs.
	UUID string

	// HardwareId is the block device's hardware ID, which is composed of
	// a serial number, vendor and model name. Not all block devices have
	// these properties, so HardwareId may be empty. This is used to identify
	// a block device if it is available, in preference to UUID or device
	// name, as the hardware ID is immutable.
	HardwareId string

	// WWN is the block device's World Wide Name (WWN) unique identifier.
	// Not all block devices have one, so WWN may be empty. This is used
	// to identify a block device if it is available, in preference to
	// UUID or device name, as the WWN is immutable.
	WWN string

	// BusAddress is the bus address: where the block device is attached
	// to the machine. This is currently only populated for disks attached
	// to the SCSI bus.
	//
	// The format for this is <bus>@<bus-specific-address> as according to
	// "lshw -businfo". For example, for a SCSI disk with Host=1, Bus=2,
	// Target=3, Lun=4, we populate this field with "scsi@1:2.3.4".
	BusAddress string

	// SizeMiB is the size of the block device, in MiB.
	SizeMiB uint64

	// FilesystemType is the type of the filesystem present on the block
	// device, if any.
	FilesystemType string

	// InUse indicates that the block device is in use (e.g. mounted).
	InUse bool

	// MountPoint is the path at which the block devices is mounted.
	MountPoint string

	// SerialId is the block device's serial id used for matching.
	SerialId string
}
