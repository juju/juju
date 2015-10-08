// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"path/filepath"

	"github.com/juju/errors"
)

const (
	diskByID         = "/dev/disk/by-id"
	diskByDeviceName = "/dev"
)

// BlockDevicePath returns the path to a block device, or an error if a path
// cannot be determined. The path is based on the hardware ID, if available;
// the first value in device.DeviceLinks, if non-empty; otherwise the device
// name.
func BlockDevicePath(device BlockDevice) (string, error) {
	if device.HardwareId != "" {
		return filepath.Join(diskByID, device.HardwareId), nil
	}
	if len(device.DeviceLinks) > 0 {
		// return the first device link in the list
		return device.DeviceLinks[0], nil
	}
	if device.DeviceName != "" {
		return filepath.Join(diskByDeviceName, device.DeviceName), nil
	}
	return "", errors.Errorf("could not determine path for block device")
}
