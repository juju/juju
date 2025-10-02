// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice

import (
	"github.com/juju/juju/core/blockdevice"
)

// BlockDeviceDetails describes information about a block device for the service layer to use.
type BlockDeviceDetails struct {
	UUID BlockDeviceUUID
	blockdevice.BlockDevice
}

// BlockDeviceData describes information about a block device for the state layer to use.
type BlockDeviceData struct {
	UUID string
	blockdevice.BlockDevice
}
