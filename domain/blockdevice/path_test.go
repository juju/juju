// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/blockdevice"
)

type BlockDevicePathSuite struct{}

var _ = gc.Suite(&BlockDevicePathSuite{})

func (s *BlockDevicePathSuite) TestBlockDevicePathSerial(c *gc.C) {
	testBlockDevicePath(c, blockdevice.BlockDevice{
		HardwareId: "SPR_OSUM_123",
		DeviceName: "name",
	}, "/dev/disk/by-id/SPR_OSUM_123")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathWWN(c *gc.C) {
	testBlockDevicePath(c, blockdevice.BlockDevice{
		HardwareId: "SPR_OSUM_123",
		WWN:        "rr!",
		DeviceName: "name",
	}, "/dev/disk/by-id/wwn-rr!")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathUUID(c *gc.C) {
	testBlockDevicePath(c, blockdevice.BlockDevice{
		UUID:       "deadbeaf",
		DeviceName: "name",
	}, "/dev/disk/by-uuid/deadbeaf")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathDeviceName(c *gc.C) {
	testBlockDevicePath(c, blockdevice.BlockDevice{
		DeviceName: "name",
	}, "/dev/name")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathError(c *gc.C) {
	_, err := blockdevice.BlockDevicePath(blockdevice.BlockDevice{})
	c.Assert(err, gc.ErrorMatches, `could not determine path for block device`)
}

func testBlockDevicePath(c *gc.C, dev blockdevice.BlockDevice, expect string) {
	path, err := blockdevice.BlockDevicePath(dev)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, jc.SamePath, expect)
}
