// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"

	"github.com/juju/juju/core/blockdevice"
)

type mockBlockDeviceGetter struct {
	blockDevices func(string) ([]blockdevice.BlockDevice, error)
}

func (b *mockBlockDeviceGetter) BlockDevices(_ context.Context, machineId string) ([]blockdevice.BlockDevice, error) {
	if b.blockDevices != nil {
		return b.blockDevices(machineId)
	}
	return []blockdevice.BlockDevice{}, nil
}
