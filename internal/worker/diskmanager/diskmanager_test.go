// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	"context"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/blockdevice"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/diskmanager"
)

var _ = tc.Suite(&DiskManagerWorkerSuite{})

type DiskManagerWorkerSuite struct {
	coretesting.BaseSuite
}

func (s *DiskManagerWorkerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(diskmanager.BlockDeviceInUse, func(device blockdevice.BlockDevice) (bool, error) {
		return false, nil
	})
}

func (s *DiskManagerWorkerSuite) TestWorker(c *tc.C) {
	done := make(chan struct{})
	var setDevices BlockDeviceSetterFunc = func(_ context.Context, devices []blockdevice.BlockDevice) error {
		close(done)
		return nil
	}

	var listDevices diskmanager.ListBlockDevicesFunc = func(context.Context) ([]blockdevice.BlockDevice, error) {
		return []blockdevice.BlockDevice{{DeviceName: "whatever"}}, nil
	}

	w := diskmanager.NewWorker(listDevices, setDevices)
	defer w.Wait()
	defer w.Kill()

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for diskmanager to update")
	}
}

func (s *DiskManagerWorkerSuite) TestBlockDeviceChanges(c *tc.C) {
	var oldDevices []blockdevice.BlockDevice
	var devicesSet [][]blockdevice.BlockDevice
	var setDevices BlockDeviceSetterFunc = func(_ context.Context, devices []blockdevice.BlockDevice) error {
		devicesSet = append(devicesSet, append([]blockdevice.BlockDevice{}, devices...))
		return nil
	}

	device := blockdevice.BlockDevice{DeviceName: "sda", DeviceLinks: []string{"a", "b"}}
	var listDevices diskmanager.ListBlockDevicesFunc = func(context.Context) ([]blockdevice.BlockDevice, error) {
		return []blockdevice.BlockDevice{device}, nil
	}

	err := diskmanager.DoWork(c.Context(), listDevices, setDevices, &oldDevices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(devicesSet, tc.HasLen, 1)

	// diskmanager only calls the BlockDeviceSetter when it sees a
	// change in disks. Order of DeviceLinks should not matter.
	device.DeviceLinks = []string{"b", "a"}
	err = diskmanager.DoWork(c.Context(), listDevices, setDevices, &oldDevices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(devicesSet, tc.HasLen, 1)

	device.DeviceName = "sdb"
	err = diskmanager.DoWork(c.Context(), listDevices, setDevices, &oldDevices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(devicesSet, tc.HasLen, 2)

	c.Assert(devicesSet[0], tc.DeepEquals, []blockdevice.BlockDevice{{
		DeviceName: "sda", DeviceLinks: []string{"a", "b"},
	}})
	c.Assert(devicesSet[1], tc.DeepEquals, []blockdevice.BlockDevice{{
		DeviceName: "sdb", DeviceLinks: []string{"a", "b"},
	}})
}

func (s *DiskManagerWorkerSuite) TestBlockDevicesSorted(c *tc.C) {
	var devicesSet [][]blockdevice.BlockDevice
	var setDevices BlockDeviceSetterFunc = func(_ context.Context, devices []blockdevice.BlockDevice) error {
		devicesSet = append(devicesSet, devices)
		return nil
	}

	var listDevices diskmanager.ListBlockDevicesFunc = func(context.Context) ([]blockdevice.BlockDevice, error) {
		return []blockdevice.BlockDevice{{
			DeviceName: "sdb",
		}, {
			DeviceName: "sda",
		}, {
			DeviceName: "sdc",
		}}, nil
	}
	err := diskmanager.DoWork(c.Context(), listDevices, setDevices, new([]blockdevice.BlockDevice))
	c.Assert(err, tc.ErrorIsNil)

	// The block Devices should be sorted when passed to the block
	// device setter.
	c.Assert(devicesSet, tc.DeepEquals, [][]blockdevice.BlockDevice{{{
		DeviceName: "sda",
	}, {
		DeviceName: "sdb",
	}, {
		DeviceName: "sdc",
	}}})
}

type BlockDeviceSetterFunc func(context.Context, []blockdevice.BlockDevice) error

func (f BlockDeviceSetterFunc) SetMachineBlockDevices(ctx context.Context, devices []blockdevice.BlockDevice) error {
	return f(ctx, devices)
}
