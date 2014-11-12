// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !linux

package diskmanager

import "github.com/juju/juju/storage"

func blockDeviceInUse(storage.BlockDevice) (bool, error) {
	panic("not supported")
}
