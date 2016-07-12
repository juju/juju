// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
)

type BlockDevicePathSuite struct{}

var _ = gc.Suite(&BlockDevicePathSuite{})

func (s *BlockDevicePathSuite) TestBlockDevicePathSerial(c *gc.C) {
	testBlockDevicePath(c, storage.BlockDevice{
		HardwareId: "SPR_OSUM_123",
		DeviceName: "name",
	}, "/dev/disk/by-id/SPR_OSUM_123")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathDeviceName(c *gc.C) {
	testBlockDevicePath(c, storage.BlockDevice{
		DeviceName: "name",
	}, "/dev/name")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathError(c *gc.C) {
	_, err := storage.BlockDevicePath(storage.BlockDevice{})
	c.Assert(err, gc.ErrorMatches, `could not determine path for block device`)
}

func testBlockDevicePath(c *gc.C, dev storage.BlockDevice, expect string) {
	path, err := storage.BlockDevicePath(dev)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, jc.SamePath, expect)
}

func (s *BlockDevicePathSuite) TestSortBlockDevices(c *gc.C) {
	devices := []storage.BlockDevice{{
		DeviceName:  "sdb",
		DeviceLinks: []string{"by-b", "by-a"},
	}, {
		DeviceName:  "sda",
		DeviceLinks: []string{"by-c", "by-d"},
	}}
	storage.SortBlockDevices(devices)

	expected := []storage.BlockDevice{{
		DeviceName:  "sda",
		DeviceLinks: []string{"by-c", "by-d"},
	}, {
		DeviceName:  "sdb",
		DeviceLinks: []string{"by-a", "by-b"},
	}}

	c.Assert(devices, jc.DeepEquals, expected)
}
