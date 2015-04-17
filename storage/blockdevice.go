// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// BlockDevice describes a block device discovered on a machine.
type BlockDevice struct {
	// DeviceName is the block device's OS-specific name (e.g. "sdb").
	DeviceName string `yaml:"devicename,omitempty"`

	// Label is the label for the filesystem on the block device.
	//
	// This will be empty if the block device does not have a filesystem,
	// or if the filesystem is not yet known to Juju.
	Label string `yaml:"label,omitempty"`

	// UUID is a unique identifier for the filesystem on the block device.
	//
	// This will be empty if the block device does not have a filesystem,
	// or if the filesystem is not yet known to Juju.
	//
	// The UUID format is not necessarily uniform; for example, LVM UUIDs
	// differ in format to the standard v4 UUIDs.
	UUID string `yaml:"uuid,omitempty"`

	// Serial is the block device's serial number. Not all block devices
	// have a serial number. This is used to identify a block device if
	// it is available, in preference to UUID or device name, as the serial
	// is immutable.
	Serial string `yaml:"serial,omitempty"`

	// Size is the size of the block device, in MiB.
	Size uint64 `yaml:"size"`

	// FilesystemType is the type of the filesystem present on the block
	// device, if any.
	FilesystemType string `yaml:"fstype,omitempty"`

	// InUse indicates that the block device is in use (e.g. mounted).
	InUse bool `yaml:"inuse"`

	// MountPoint is the path at which the block devices is mounted.
	MountPoint string `yaml:"mountpoint,omitempty"`
}
