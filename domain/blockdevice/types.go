// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice

// BlockDevice describes information about a block device.
type BlockDevice struct {
	DeviceName     string
	DeviceLinks    []string
	Label          string
	UUID           string
	HardwareId     string
	WWN            string
	BusAddress     string
	Size           uint64
	FilesystemType string
	InUse          bool
	MountPoint     string
	SerialId       string
}
