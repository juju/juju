// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !linux

package diskmanager

import (
	"runtime"

	"github.com/juju/juju/core/blockdevice"
)

var blockDeviceInUse = func(blockdevice.BlockDevice) (bool, error) {
	panic("not supported")
}

func listBlockDevices() ([]blockdevice.BlockDevice, error) {
	// Return an empty list each time.
	return nil, nil
}

func init() {
	logger.Infof(ctx,
		"block device support has not been implemented for %s",
		runtime.GOOS,
	)
	DefaultListBlockDevices = listBlockDevices
}
