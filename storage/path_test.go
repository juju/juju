// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/juju/storage"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type BlockDevicePathSuite struct{}

var _ = gc.Suite(&BlockDevicePathSuite{})

func (s *BlockDevicePathSuite) TestBlockDevicePathLabel(c *gc.C) {
	testBlockDevicePath(c, storage.BlockDevice{
		Label:      "label",
		UUID:       "uuid",
		DeviceName: "name",
	}, "/dev/disk/by-label/label")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathUUID(c *gc.C) {
	testBlockDevicePath(c, storage.BlockDevice{
		UUID:       "uuid",
		DeviceName: "name",
	}, "/dev/disk/by-uuid/uuid")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathDeviceName(c *gc.C) {
	testBlockDevicePath(c, storage.BlockDevice{
		DeviceName: "name",
	}, "/dev/name")
}

func (s *BlockDevicePathSuite) TestBlockDevicePathError(c *gc.C) {
	_, err := storage.BlockDevicePath(storage.BlockDevice{Name: "0"})
	c.Assert(err, gc.ErrorMatches, `could not determine path for block device "0"`)
}

func testBlockDevicePath(c *gc.C, dev storage.BlockDevice, expect string) {
	path, err := storage.BlockDevicePath(dev)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.Equals, expect)
}
