// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"path/filepath"

	"github.com/juju/errors"
)

const (
	diskByUUID  = "/dev/disk/by-uuid"
	diskByLabel = "/dev/disk/by-label"
)

// BlockDevicePath returns the path to a block device, or an error if a path
// cannot be determined.
//
// The path is only guaranteed to be persistent until the machine reboots or
// the device is modified (e.g. filesystem destroyed or created).
func BlockDevicePath(device BlockDevice) (string, error) {
	// Labels must be unique, and are short, so prefer them over UUID.
	if device.Label != "" {
		return filepath.Join(diskByLabel, device.Label), nil
	}
	if device.UUID != "" {
		return filepath.Join(diskByUUID, device.UUID), nil
	}
	if device.DeviceName != "" {
		return filepath.Join("/dev", device.DeviceName), nil
	}
	return "", errors.Errorf("could not determine path for block device %q", device.Name)
}
