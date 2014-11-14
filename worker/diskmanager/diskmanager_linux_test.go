// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/diskmanager"
)

func (s *DiskManagerWorkerSuite) TestWorker(c *gc.C) {
	done := make(chan struct{})
	var setDevices BlockDeviceSetterFunc = func(devices []storage.BlockDevice) error {
		close(done)
		return nil
	}

	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="0" LABEL="" UUID=""
EOF`,
	)

	w := diskmanager.NewWorker(setDevices)
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

	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="0" LABEL="" UUID=""
EOF`,
	)
	for i := 0; i < 2; i++ {
		err := diskmanager.DoWork(setDevices, &oldDevices)
		c.Assert(err, gc.IsNil)
	}

	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sdb" SIZE="0" LABEL="" UUID=""
EOF`,
	)
	err := diskmanager.DoWork(setDevices, &oldDevices)
	c.Assert(err, gc.IsNil)

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

	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sdb" SIZE="0" LABEL="" UUID=""
KNAME="sda" SIZE="0" LABEL="" UUID=""
KNAME="sdc" SIZE="0" LABEL="" UUID=""
EOF`,
	)
	err := diskmanager.DoWork(setDevices, new([]storage.BlockDevice))
	c.Assert(err, gc.IsNil)

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
