// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"os"
	"syscall"

	"github.com/juju/juju/storage"
)

// blockDeviceInUse checks if the specified block device
// is in use by attempting to open the device exclusively.
var blockDeviceInUse = func(dev storage.BlockDevice) (bool, error) {
	f, err := os.OpenFile("/dev/"+dev.DeviceName, os.O_EXCL, 0)
	if err != nil {
		if err, ok := err.(*os.PathError); ok {
			// open(2): "In general, the behavior of O_EXCL is undefined if
			// it is used without O_CREAT. There is one exception: on Linux
			// 2.6 and later, O_EXCL can be used without O_CREAT if pathname
			// refers to a block device. If the block device is in use by the
			// system  (e.g., mounted), open() fails with the error EBUSY."
			if errno, ok := err.Err.(syscall.Errno); ok && errno == syscall.EBUSY {
				return true, nil
			}
		}
		return false, err
	}
	f.Close()
	return false, nil
}
