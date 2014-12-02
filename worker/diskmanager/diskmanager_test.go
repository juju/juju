// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/diskmanager"
)

var _ = gc.Suite(&DiskManagerWorkerSuite{})

type DiskManagerWorkerSuite struct {
	coretesting.BaseSuite
}

func (s *DiskManagerWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(diskmanager.BlockDeviceInUse, func(storage.BlockDevice) (bool, error) {
		return false, nil
	})
}

func (s *DiskManagerWorkerSuite) TestWorker(c *gc.C) {
	done := make(chan struct{})
	var setDevices BlockDeviceSetterFunc = func(devices []storage.BlockDevice) error {
		close(done)
		return nil
	}

	var listDevices diskmanager.ListBlockDevicesFunc = func() ([]storage.BlockDevice, error) {
		return []storage.BlockDevice{{DeviceName: "whatever"}}, nil
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
	var oldDevices []storage.BlockDevice
	var devicesSet [][]storage.BlockDevice
	var setDevices BlockDeviceSetterFunc = func(devices []storage.BlockDevice) error {
		devicesSet = append(devicesSet, devices)
		return nil
	}

	var listDevices diskmanager.ListBlockDevicesFunc = func() ([]storage.BlockDevice, error) {
		return []storage.BlockDevice{{DeviceName: "sda"}}, nil
	}
	for i := 0; i < 2; i++ {
		err := diskmanager.DoWork(listDevices, setDevices, &oldDevices)
		c.Assert(err, jc.ErrorIsNil)
	}

	listDevices = func() ([]storage.BlockDevice, error) {
		return []storage.BlockDevice{{DeviceName: "sdb"}}, nil
	}
	err := diskmanager.DoWork(listDevices, setDevices, &oldDevices)
	c.Assert(err, jc.ErrorIsNil)

	// diskmanager only calls the BlockDeviceSetter when it sees
	// a change in disks.
	c.Assert(devicesSet, gc.HasLen, 2)
	c.Assert(devicesSet[0], gc.DeepEquals, []storage.BlockDevice{{
		DeviceName: "sda",
	}})
	c.Assert(devicesSet[1], gc.DeepEquals, []storage.BlockDevice{{
		DeviceName: "sdb",
	}})
}

func (s *DiskManagerWorkerSuite) TestBlockDevicesSorted(c *gc.C) {
	var devicesSet [][]storage.BlockDevice
	var setDevices BlockDeviceSetterFunc = func(devices []storage.BlockDevice) error {
		devicesSet = append(devicesSet, devices)
		return nil
	}

	var listDevices diskmanager.ListBlockDevicesFunc = func() ([]storage.BlockDevice, error) {
		return []storage.BlockDevice{{
			DeviceName: "sdb",
		}, {
			DeviceName: "sda",
		}, {
			DeviceName: "sdc",
		}}, nil
	}
	err := diskmanager.DoWork(listDevices, setDevices, new([]storage.BlockDevice))
	c.Assert(err, jc.ErrorIsNil)

	// The block Devices should be sorted when passed to the block
	// device setter.
	c.Assert(devicesSet, gc.DeepEquals, [][]storage.BlockDevice{{{
		DeviceName: "sda",
	}, {
		DeviceName: "sdb",
	}, {
		DeviceName: "sdc",
	}}})
}

type BlockDeviceSetterFunc func([]storage.BlockDevice) error

func (f BlockDeviceSetterFunc) SetMachineBlockDevices(devices []storage.BlockDevice) error {
	return f(devices)
}
