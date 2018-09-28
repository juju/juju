// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package diskmanager_test

import (
	"errors"
	"os"
	"strings"

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
	testing.PatchExecutable(c, s, "udevadm", `#!/bin/bash --norc`)
}

func (s *ListBlockDevicesSuite) TestListBlockDevices(c *gc.C) {
	s.PatchValue(diskmanager.BlockDeviceInUse, func(dev storage.BlockDevice) (bool, error) {
		return dev.DeviceName == "sdb", nil
	})
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="240057409536" LABEL="" UUID="" TYPE="disk"
KNAME="sda1" SIZE="254803968" LABEL="" UUID="7a62bd85-a350-4c09-8944-5b99bf2080c6" MOUNTPOINT="/tmp" TYPE="part"
KNAME="sda2" SIZE="1024" LABEL="boot" UUID="" TYPE="part"
KNAME="sdb" SIZE="32017047552" LABEL="" UUID="" TYPE="disk"
KNAME="sdb1" SIZE="32015122432" LABEL="media" UUID="2c1c701d-f2ce-43a4-b345-33e2e39f9503" FSTYPE="ext4" TYPE="part"
KNAME="fd0" SIZE="1024" TYPE="disk" MAJ:MIN="2:0"
KNAME="fd1" SIZE="1024" TYPE="disk" MAJ:MIN="2:1"
EOF`)

	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.DeepEquals, []storage.BlockDevice{{
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

func (s *ListBlockDevicesSuite) TestListBlockDevicesWWN(c *gc.C) {
	// If ID_WWN is found, then we should get
	// a WWN value.
	s.testListBlockDevicesExtended(c, `
ID_WWN=foo
`, storage.BlockDevice{WWN: "foo"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesBusAddress(c *gc.C) {
	// If ID_BUS is scsi, then we should get a
	// BusAddress value.
	s.testListBlockDevicesExtended(c, `
DEVPATH=/a/b/c/d/1:2:3:4/block/sda
ID_BUS=scsi
`, storage.BlockDevice{BusAddress: "scsi@1:2.3.4"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesHardwareId(c *gc.C) {
	// If ID_BUS and ID_SERIAL are both present, we
	// should get a HardwareId value.
	s.testListBlockDevicesExtended(c, `
ID_BUS=ata
ID_SERIAL=0980978987987
`, storage.BlockDevice{HardwareId: "ata-0980978987987"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesDeviceLinks(c *gc.C) {
	// Values from DEVLINKS should be split by space, and entered into
	// DeviceLinks verbatim.
	s.testListBlockDevicesExtended(c, `
DEVLINKS=/dev/disk/by-id/abc /dev/disk/by-id/def
`, storage.BlockDevice{
		DeviceLinks: []string{"/dev/disk/by-id/abc", "/dev/disk/by-id/def"},
	})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesAll(c *gc.C) {
	s.testListBlockDevicesExtended(c, `
DEVPATH=/a/b/c/d/1:2:3:4/block/sda
ID_BUS=scsi
ID_SERIAL=0980978987987
`, storage.BlockDevice{BusAddress: "scsi@1:2.3.4", HardwareId: "scsi-0980978987987"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesUnexpectedDevpathFormat(c *gc.C) {
	// If DEVPATH's format doesn't match what we expect, then we should
	// just not get the BusAddress value.
	s.testListBlockDevicesExtended(c, `
DEVPATH=/a/b/c/d/x:y:z:zy/block/sda
ID_BUS=ata
ID_SERIAL=0980978987987
`, storage.BlockDevice{HardwareId: "ata-0980978987987"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesUnexpectedPropertyFormat(c *gc.C) {
	// If udevadm outputs in an unexpected format, we won't error;
	// we only error if some catastrophic error occurs while reading
	// from the udevadm command's stdout.
	s.testListBlockDevicesExtended(c, "nonsense", storage.BlockDevice{})
}

func (s *ListBlockDevicesSuite) testListBlockDevicesExtended(
	c *gc.C,
	udevadmInfo string,
	expect storage.BlockDevice,
) {
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="240057409536" LABEL="" UUID="" TYPE="disk"
EOF`)
	testing.PatchExecutable(c, s, "udevadm", `#!/bin/bash --norc
cat <<EOF
`+strings.TrimSpace(udevadmInfo)+`
EOF`)

	expect.DeviceName = "sda"
	expect.Size = 228936

	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.DeepEquals, []storage.BlockDevice{expect})
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
KNAME="sda" SIZE="240057409536" LABEL="" UUID="" TYPE="disk"
EOF`)

	// If the in-use check errors, the block device will be marked "in use"
	// to prevent it from being used, but no error will be returned.
	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.DeepEquals, []storage.BlockDevice{{
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
KNAME="sda" SIZE="eleventy" LABEL="" UUID="" TYPE="disk"
KNAME="sdb" SIZE="1048576" LABEL="" UUID="" BOB="DOBBS" TYPE="disk"
EOF`)

	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.DeepEquals, []storage.BlockDevice{{
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
KNAME="sda" SIZE="240057409536" LABEL="" UUID="" TYPE="disk"
KNAME="sdb" SIZE="32017047552" LABEL="" UUID="" TYPE="disk"
EOF`)

	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 0)
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesDeviceFiltering(c *gc.C) {
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="240057409536" LABEL="" UUID="" TYPE="disk"
KNAME="sda1" SIZE="254803968" LABEL="" UUID="" TYPE="part"
KNAME="loop0" SIZE="254803968" LABEL="" UUID="" TYPE="loop"
KNAME="sr0" SIZE="254803968" LABEL="" UUID="" TYPE="rom"
KNAME="whatever" SIZE="254803968" LABEL="" UUID="" TYPE="lvm"
EOF`)

	devices, err := diskmanager.ListBlockDevices()
	c.Assert(err, gc.IsNil)
	c.Assert(devices, jc.DeepEquals, []storage.BlockDevice{{
		DeviceName: "sda",
		Size:       228936,
	}, {
		DeviceName: "sda1",
		Size:       243,
	}, {
		DeviceName: "loop0",
		Size:       243,
	}})
}
