// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/juju/core/blockdevice"
	corewatcher "github.com/juju/juju/core/watcher"
)

type blockDeviceService interface {
	BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error)
	WatchBlockDevices(ctx context.Context, machineId string) (corewatcher.NotifyWatcher, error)
}
