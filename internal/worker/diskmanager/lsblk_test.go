// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package diskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/blockdevice"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/diskmanager"
)

var _ = tc.Suite(&ListBlockDevicesSuite{})

type ListBlockDevicesSuite struct {
	coretesting.BaseSuite
}

func (s *ListBlockDevicesSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(diskmanager.BlockDeviceInUse, func(device blockdevice.BlockDevice) (bool, error) {
		return false, nil
	})
	testing.PatchExecutable(c, s, "udevadm", `#!/bin/bash --norc`)
}

func (s *ListBlockDevicesSuite) TestListBlockDevices(c *tc.C) {
	s.PatchValue(diskmanager.BlockDeviceInUse, func(dev blockdevice.BlockDevice) (bool, error) {
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

	devices, err := diskmanager.ListBlockDevices(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.DeepEquals, []blockdevice.BlockDevice{{
		DeviceName: "sda",
		SizeMiB:    228936,
	}, {
		DeviceName: "sda1",
		SizeMiB:    243,
		UUID:       "7a62bd85-a350-4c09-8944-5b99bf2080c6",
		MountPoint: "/tmp",
	}, {
		DeviceName: "sda2",
		SizeMiB:    0, // truncated
		Label:      "boot",
	}, {
		DeviceName: "sdb",
		SizeMiB:    30533,
		InUse:      true,
	}, {
		DeviceName:     "sdb1",
		SizeMiB:        30532,
		Label:          "media",
		UUID:           "2c1c701d-f2ce-43a4-b345-33e2e39f9503",
		FilesystemType: "ext4",
	}})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesWWN(c *tc.C) {
	// If ID_WWN is found, then we should get
	// a WWN value.
	s.testListBlockDevicesExtended(c, `
ID_WWN=foo
`, "sda", blockdevice.BlockDevice{WWN: "foo"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesExtendedWWN(c *tc.C) {
	// If ID_WWN_WITH_EXTENSION is found, then we should use that
	// in preference to the ID_WWN value.
	s.testListBlockDevicesExtended(c, `
ID_WWN_WITH_EXTENSION=foobar
ID_WWN=foo
`, "sda", blockdevice.BlockDevice{WWN: "foobar"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesBusAddress(c *tc.C) {
	// If ID_BUS is scsi, then we should get a
	// BusAddress value.
	s.testListBlockDevicesExtended(c, `
DEVPATH=/a/b/c/d/1:2:3:4/block/sda
ID_BUS=scsi
`, "sda", blockdevice.BlockDevice{BusAddress: "scsi@1:2.3.4"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesHardwareId(c *tc.C) {
	// If ID_BUS and ID_SERIAL are both present, we
	// should get a HardwareId value.
	s.testListBlockDevicesExtended(c, `
ID_BUS=ata
ID_SERIAL=0980978987987
`, "sda", blockdevice.BlockDevice{HardwareId: "ata-0980978987987", SerialId: "0980978987987"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesSerialId(c *tc.C) {
	// If ID_SERIAL is found, then we should get
	// a SerialId value.
	s.testListBlockDevicesExtended(c, `
ID_SERIAL=0980978987987
`, "sda", blockdevice.BlockDevice{SerialId: "0980978987987"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesDeviceLinks(c *tc.C) {
	// Values from DEVLINKS should be split by space, and entered into
	// DeviceLinks verbatim.
	s.testListBlockDevicesExtended(c, `
DEVLINKS=/dev/disk/by-id/abc /dev/disk/by-id/def
`, "sda", blockdevice.BlockDevice{
		DeviceLinks: []string{"/dev/disk/by-id/abc", "/dev/disk/by-id/def"},
	})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesAll(c *tc.C) {
	s.testListBlockDevicesExtended(c, `
DEVPATH=/a/b/c/d/1:2:3:4/block/sda
ID_BUS=scsi
ID_SERIAL=0980978987987
`, "sda", blockdevice.BlockDevice{BusAddress: "scsi@1:2.3.4", HardwareId: "scsi-0980978987987", SerialId: "0980978987987"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesUnexpectedDevpathFormat(c *tc.C) {
	// If DEVPATH's format doesn't match what we expect, then we should
	// just not get the BusAddress value.
	s.testListBlockDevicesExtended(c, `
DEVPATH=/a/b/c/d/x:y:z:zy/block/sda
ID_BUS=ata
ID_SERIAL=0980978987987
`, "sda", blockdevice.BlockDevice{HardwareId: "ata-0980978987987", SerialId: "0980978987987"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesParition(c *tc.C) {
	// Test DEVPATH format for partition.
	s.testListBlockDevicesExtended(c, `
DEVPATH=/a/b/c/d/1:2:3:4/block/sda/sda1
ID_BUS=scsi
ID_SERIAL=0980978987987
`, "sda1", blockdevice.BlockDevice{BusAddress: "scsi@1:2.3.4", HardwareId: "scsi-0980978987987", SerialId: "0980978987987"})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesUnexpectedPropertyFormat(c *tc.C) {
	// If udevadm outputs in an unexpected format, we won't error;
	// we only error if some catastrophic error occurs while reading
	// from the udevadm command's stdout.
	s.testListBlockDevicesExtended(c, "nonsense", "sda", blockdevice.BlockDevice{})
}

func (s *ListBlockDevicesSuite) testListBlockDevicesExtended(
	c *tc.C,
	udevadmInfo string,
	deviceName string,
	expect blockdevice.BlockDevice,
) {
	testing.PatchExecutable(c, s, "lsblk", fmt.Sprintf(`#!/bin/bash --norc
cat <<EOF
KNAME="%s" SIZE="240057409536" LABEL="" UUID="" TYPE="disk"
EOF`, deviceName))
	testing.PatchExecutable(c, s, "udevadm", `#!/bin/bash --norc
cat <<EOF
`+strings.TrimSpace(udevadmInfo)+`
EOF`)

	expect.DeviceName = deviceName
	expect.SizeMiB = 228936

	devices, err := diskmanager.ListBlockDevices(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.DeepEquals, []blockdevice.BlockDevice{expect})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesLsblkError(c *tc.C) {
	testing.PatchExecutableThrowError(c, s, "lsblk", 123)
	devices, err := diskmanager.ListBlockDevices(context.Background())
	c.Assert(err, tc.ErrorMatches, "cannot list block devices: lsblk failed: exit status 123")
	c.Assert(devices, tc.IsNil)
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesBlockDeviceInUseError(c *tc.C) {
	s.PatchValue(diskmanager.BlockDeviceInUse, func(dev blockdevice.BlockDevice) (bool, error) {
		return false, errors.New("badness")
	})
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="240057409536" LABEL="" UUID="" TYPE="disk"
EOF`)

	// If the in-use check errors, the block device will be marked "in use"
	// to prevent it from being used, but no error will be returned.
	devices, err := diskmanager.ListBlockDevices(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.DeepEquals, []blockdevice.BlockDevice{{
		DeviceName: "sda",
		SizeMiB:    228936,
		InUse:      true,
	}})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesLsblkBadOutput(c *tc.C) {
	// Extra key/value pairs should be ignored; invalid sizes should
	// be logged and ignored (Size will be set to zero).
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="eleventy" LABEL="" UUID="" TYPE="disk"
KNAME="sdb" SIZE="1048576" LABEL="" UUID="" BOB="DOBBS" TYPE="disk"
EOF`)

	devices, err := diskmanager.ListBlockDevices(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, jc.DeepEquals, []blockdevice.BlockDevice{{
		DeviceName: "sda",
		SizeMiB:    0,
	}, {
		DeviceName: "sdb",
		SizeMiB:    1,
	}})
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesDeviceNotExist(c *tc.C) {
	s.PatchValue(diskmanager.BlockDeviceInUse, func(dev blockdevice.BlockDevice) (bool, error) {
		return false, os.ErrNotExist
	})
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="240057409536" LABEL="" UUID="" TYPE="disk"
KNAME="sdb" SIZE="32017047552" LABEL="" UUID="" TYPE="disk"
EOF`)

	devices, err := diskmanager.ListBlockDevices(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, tc.HasLen, 0)
}

func (s *ListBlockDevicesSuite) TestListBlockDevicesDeviceFiltering(c *tc.C) {
	testing.PatchExecutable(c, s, "lsblk", `#!/bin/bash --norc
cat <<EOF
KNAME="sda" SIZE="240057409536" LABEL="" UUID="" TYPE="disk"
KNAME="sda1" SIZE="254803968" LABEL="" UUID="" TYPE="part"
KNAME="loop0" SIZE="254803968" LABEL="" UUID="" TYPE="loop"
KNAME="sr0" SIZE="254803968" LABEL="" UUID="" TYPE="rom"
KNAME="whatever" SIZE="254803968" LABEL="" UUID="" TYPE="lvm"
EOF`)

	devices, err := diskmanager.ListBlockDevices(context.Background())
	c.Assert(err, tc.IsNil)
	c.Assert(devices, jc.DeepEquals, []blockdevice.BlockDevice{{
		DeviceName: "sda",
		SizeMiB:    228936,
	}, {
		DeviceName: "sda1",
		SizeMiB:    243,
	}, {
		DeviceName: "loop0",
		SizeMiB:    243,
	}})
}
