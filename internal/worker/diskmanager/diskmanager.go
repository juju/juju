// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/juju/worker/v4"

	"github.com/juju/juju/core/blockdevice"
	internallogger "github.com/juju/juju/internal/logger"
	jworker "github.com/juju/juju/internal/worker"
)

var logger = internallogger.GetLogger("juju.worker.diskmanager")

const (
	// listBlockDevicesPeriod is the time period between block device listings.
	// Unfortunately Linux's inotify does not work with virtual filesystems, so
	// polling it is.
	listBlockDevicesPeriod = time.Second * 30

	// bytesInMiB is the number of bytes in a MiB.
	bytesInMiB = 1024 * 1024
)

// BlockDeviceSetter is an interface that is supplied to
// NewWorker for setting block devices for the local host.
type BlockDeviceSetter interface {
	SetMachineBlockDevices(context.Context, []blockdevice.BlockDevice) error
}

// ListBlockDevicesFunc is the type of a function that is supplied to
// NewWorker for listing block devices available on the local host.
type ListBlockDevicesFunc func() ([]blockdevice.BlockDevice, error)

// DefaultListBlockDevices is the default function for listing block
// devices for the operating system of the local host.
var DefaultListBlockDevices ListBlockDevicesFunc

// NewWorker returns a worker that lists block devices
// attached to the machine, and records them in state.
var NewWorker = func(l ListBlockDevicesFunc, b BlockDeviceSetter) worker.Worker {
	var old []blockdevice.BlockDevice
	f := func(ctx context.Context) error {
		return doWork(ctx, l, b, &old)
	}
	return jworker.NewPeriodicWorker(f, listBlockDevicesPeriod, jworker.NewTimer)
}

func doWork(ctx context.Context, listf ListBlockDevicesFunc, b BlockDeviceSetter, old *[]blockdevice.BlockDevice) error {
	blockDevices, err := listf()
	if err != nil {
		return err
	}
	blockdevice.SortBlockDevices(blockDevices)
	for _, blockDevice := range blockDevices {
		sort.Strings(blockDevice.DeviceLinks)
	}
	if reflect.DeepEqual(blockDevices, *old) {
		logger.Tracef("no changes to block devices detected")
		return nil
	}
	logger.Tracef("block devices changed: %#v", blockDevices)
	if err := b.SetMachineBlockDevices(ctx, blockDevices); err != nil {
		return err
	}
	*old = blockDevices
	return nil
}
