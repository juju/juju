// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/internal/worker/diskmanager"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&DiskManagerWorkerSuite{})

type DiskManagerWorkerSuite struct {
	coretesting.BaseSuite
}

func (s *DiskManagerWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(diskmanager.BlockDeviceInUse, func(device blockdevice.BlockDevice) (bool, error) {
		return false, nil
	})
}

func (s *DiskManagerWorkerSuite) TestWorker(c *gc.C) {
	done := make(chan struct{})
	var setDevices BlockDeviceSetterFunc = func(devices []blockdevice.BlockDevice) error {
		close(done)
		return nil
	}

	var listDevices diskmanager.ListBlockDevicesFunc = func() ([]blockdevice.BlockDevice, error) {
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

func (s *DiskManagerWorkerSuite) TestBlockDeviceChanges(c *gc.C) {
	var oldDevices []blockdevice.BlockDevice
	var devicesSet [][]blockdevice.BlockDevice
	var setDevices BlockDeviceSetterFunc = func(devices []blockdevice.BlockDevice) error {
		devicesSet = append(devicesSet, append([]blockdevice.BlockDevice{}, devices...))
		return nil
	}

	device := blockdevice.BlockDevice{DeviceName: "sda", DeviceLinks: []string{"a", "b"}}
	var listDevices diskmanager.ListBlockDevicesFunc = func() ([]blockdevice.BlockDevice, error) {
		return []blockdevice.BlockDevice{device}, nil
	}

	err := diskmanager.DoWork(listDevices, setDevices, &oldDevices)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devicesSet, gc.HasLen, 1)

	// diskmanager only calls the BlockDeviceSetter when it sees a
	// change in disks. Order of DeviceLinks should not matter.
	device.DeviceLinks = []string{"b", "a"}
	err = diskmanager.DoWork(listDevices, setDevices, &oldDevices)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devicesSet, gc.HasLen, 1)

	device.DeviceName = "sdb"
	err = diskmanager.DoWork(listDevices, setDevices, &oldDevices)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devicesSet, gc.HasLen, 2)

	c.Assert(devicesSet[0], gc.DeepEquals, []blockdevice.BlockDevice{{
		DeviceName: "sda", DeviceLinks: []string{"a", "b"},
	}})
	c.Assert(devicesSet[1], gc.DeepEquals, []blockdevice.BlockDevice{{
		DeviceName: "sdb", DeviceLinks: []string{"a", "b"},
	}})
}

func (s *DiskManagerWorkerSuite) TestBlockDevicesSorted(c *gc.C) {
	var devicesSet [][]blockdevice.BlockDevice
	var setDevices BlockDeviceSetterFunc = func(devices []blockdevice.BlockDevice) error {
		devicesSet = append(devicesSet, devices)
		return nil
	}

	var listDevices diskmanager.ListBlockDevicesFunc = func() ([]blockdevice.BlockDevice, error) {
		return []blockdevice.BlockDevice{{
			DeviceName: "sdb",
		}, {
			DeviceName: "sda",
		}, {
			DeviceName: "sdc",
		}}, nil
	}
	err := diskmanager.DoWork(listDevices, setDevices, new([]blockdevice.BlockDevice))
	c.Assert(err, jc.ErrorIsNil)

	// The block Devices should be sorted when passed to the block
	// device setter.
	c.Assert(devicesSet, gc.DeepEquals, [][]blockdevice.BlockDevice{{{
		DeviceName: "sda",
	}, {
		DeviceName: "sdb",
	}, {
		DeviceName: "sdc",
	}}})
}

type BlockDeviceSetterFunc func([]blockdevice.BlockDevice) error

func (f BlockDeviceSetterFunc) SetMachineBlockDevices(devices []blockdevice.BlockDevice) error {
	return f(devices)
}
