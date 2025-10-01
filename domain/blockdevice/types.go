// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice

// BlockDeviceDetails describes information about a block device.
type BlockDeviceDetails struct {
	UUID             string
	HardwareID       string
	WWN              string
	BlockDeviceName  string
	BlockDeviceLinks []string
}
