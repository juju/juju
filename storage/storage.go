// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import "sort"

// BlockDevice describes a block device (disk, logical volume, etc.)
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

	// Size is the size of the block device, in MiB.
	Size uint64 `yaml:"size"`

	// InUse indicates that the block device is in use (e.g. mounted).
	InUse bool `yaml:"inuse"`
}

// SortBlockDevices sorts block devices by device name.
func SortBlockDevices(devices []BlockDevice) {
	sort.Sort(byDeviceName(devices))
}

type byDeviceName []BlockDevice

func (b byDeviceName) Len() int {
	return len(b)
}

func (b byDeviceName) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byDeviceName) Less(i, j int) bool {
	return b[i].DeviceName < b[j].DeviceName
}
