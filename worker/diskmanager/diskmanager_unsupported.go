// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !linux

package diskmanager

import (
	"github.com/juju/juju/storage"
	"github.com/juju/juju/version"
)

var blockDeviceInUse = func(storage.BlockDevice) (bool, error) {
	panic("not supported")
}

func listBlockDevices() ([]storage.BlockDevice, error) {
	// Return an empty list each time.
	return nil, nil
}

func init() {
	logger.Infof(
		"block device support has not been implemented for %s",
		version.Current.OS,
	)
	DefaultListBlockDevices = listBlockDevices
}
