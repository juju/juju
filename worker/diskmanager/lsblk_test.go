// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package diskmanager_test

import (
	"errors"
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/diskmanager"
)

var _ = gc.Suite(&ListBlockDevicesSuite{})

type ListBlockDevicesSuite struct {
	coretesting.BaseSuite
}

func (s *ListBlockDevicesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(diskmanager.BlockDeviceInUse, func(storage.BlockDevice) (bool, error) {
		return false, nil
	})
}

func (s *ListBlockDevicesSuite) TestListBlockDevices(c *gc.C) {
	s.PatchValue(diskmanager.BlockDeviceInUse, func(dev storage.BlockDevice) (bool, error) {
		return dev.DeviceName == "sdb", nil
	})
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="240057409536" LABEL="" UUID=""
KNAME="sda1" SIZE="254803968" LABEL="" UUID="7a62bd85-a350-4c09-8944-5b99bf2080c6" MOUNTPOINT="/tmp"
KNAME="sda2" SIZE="1024" LABEL="boot" UUID=""
KNAME="sdb" SIZE="32017047552" LABEL="" UUID=""
KNAME="sdb1" SIZE="32015122432" LABEL="media" UUID="2c1c701d-f2ce-43a4-b345-33e2e39f9503" FSTYPE="ext4"
EOF`)

	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.SameContents, []storage.BlockDevice{{
		DeviceName: "sda",
		Size:       228936,
	}, {
		DeviceName: "sda1",
		Size:       243,
		UUID:       "7a62bd85-a350-4c09-8944-5b99bf2080c6",
		MountPoint: "/tmp",
	}, {
		DeviceName: "sda2",
		Size:       0, // truncated
		Label:      "boot",
	}, {
		DeviceName: "sdb",
		Size:       30533,
		InUse:      true,
	}, {
		DeviceName:     "sdb1",
		Size:           30532,
		Label:          "media",
		UUID:           "2c1c701d-f2ce-43a4-b345-33e2e39f9503",
		FilesystemType: "ext4",
	}})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesLsblkError(c *gc.C) {
	testing.PatchExecutableThrowError(c, s, "lsblk", 123)
	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, gc.ErrorMatches, "cannot list block devices: lsblk failed: exit status 123")
	c.Assert(devices, gc.IsNil)
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesBlockDeviceInUseError(c *gc.C) {
	s.PatchValue(diskmanager.BlockDeviceInUse, func(dev storage.BlockDevice) (bool, error) {
		return false, errors.New("badness")
	})
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="240057409536" LABEL="" UUID=""
EOF`)

	// If the in-use check errors, the block device will be marked "in use"
	// to prevent it from being used, but no error will be returned.
	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.SameContents, []storage.BlockDevice{{
		DeviceName: "sda",
		Size:       228936,
		InUse:      true,
	}})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesLsblkBadOutput(c *gc.C) {
	// Extra key/value pairs should be ignored; invalid sizes should
	// be logged and ignored (Size will be set to zero).
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="eleventy" LABEL="" UUID=""
KNAME="sdb" SIZE="1048576" LABEL="" UUID="" BOB="DOBBS"
EOF`)

	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.SameContents, []storage.BlockDevice{{
		DeviceName: "sda",
		Size:       0,
	}, {
		DeviceName: "sdb",
		Size:       1,
	}})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesDeviceNotExist(c *gc.C) {
	s.PatchValue(diskmanager.BlockDeviceInUse, func(dev storage.BlockDevice) (bool, error) {
		return false, os.ErrNotExist
	})
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="240057409536" LABEL="" UUID=""
KNAME="sdb" SIZE="32017047552" LABEL="" UUID=""
EOF`)

	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 0)
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesDevicePartitions(c *gc.C) {
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="240057409536" LABEL="" UUID="" TYPE="disk"
KNAME="sda1" SIZE="254803968" LABEL="" UUID="" TYPE="part"
EOF`)

	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, gc.IsNil)
	c.Assert(devices, gc.DeepEquals, []storage.BlockDevice{{
		DeviceName: "sda",
		Size:       228936,
	}})
}
