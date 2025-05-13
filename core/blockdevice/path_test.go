// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/blockdevice"
)

type BlockDevicePathSuite struct{}

var _ = tc.Suite(&BlockDevicePathSuite{})

func (s *BlockDevicePathSuite) TestBlockDevicePathSerial(c *tc.C) {
	testBlockDevicePath(c, blockdevice.BlockDevice{
		HardwareId: "SPR_OSUM_123",
		DeviceName: "name",
	}, "/dev/disk/by-id/SPR_OSUM_123")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathWWN(c *tc.C) {
	testBlockDevicePath(c, blockdevice.BlockDevice{
		HardwareId: "SPR_OSUM_123",
		WWN:        "rr!",
		DeviceName: "name",
	}, "/dev/disk/by-id/wwn-rr!")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathUUID(c *tc.C) {
	testBlockDevicePath(c, blockdevice.BlockDevice{
		UUID:       "deadbeaf",
		DeviceName: "name",
	}, "/dev/disk/by-uuid/deadbeaf")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathDeviceName(c *tc.C) {
	testBlockDevicePath(c, blockdevice.BlockDevice{
		DeviceName: "name",
	}, "/dev/name")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathError(c *tc.C) {
	_, err := blockdevice.BlockDevicePath(blockdevice.BlockDevice{})
	c.Assert(err, tc.ErrorMatches, `could not determine path for block device`)
}

func testBlockDevicePath(c *tc.C, dev blockdevice.BlockDevice, expect string) {
	path, err := blockdevice.BlockDevicePath(dev)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.SamePath, expect)
}
