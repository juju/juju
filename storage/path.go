// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"path"

	"github.com/juju/errors"
)

const (
	diskByID         = "/dev/disk/by-id"
	diskByWWN        = "/dev/disk/by-id/wwn-"
	diskByDeviceName = "/dev"
)

// BlockDevicePath returns the path to a block device, or an error if a path
// cannot be determined. The path is based on the hardware ID, if available;
// the first value in device.DeviceLinks, if non-empty; otherwise the device
// name.
func BlockDevicePath(device BlockDevice) (string, error) {
	if device.WWN != "" {
		return diskByWWN + device.WWN, nil
	}
	if device.HardwareId != "" {
		return path.Join(diskByID, device.HardwareId), nil
	}
	if len(device.DeviceLinks) > 0 {
		// return the first device link in the list
		return device.DeviceLinks[0], nil
	}
	if device.DeviceName != "" {
		return path.Join(diskByDeviceName, device.DeviceName), nil
	}
	return "", errors.Errorf("could not determine path for block device")
}
