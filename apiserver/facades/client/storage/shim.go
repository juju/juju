// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/juju/core/blockdevice"
)

type blockDeviceGetter interface {
	BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error)
}
