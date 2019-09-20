// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/state"
)

type BlockDeviceSuite struct {
}

var _ = gc.Suite(&BlockDeviceSuite{})

func (s *BlockDeviceSuite) TestBlockDeviceMatchingSerialID(c *gc.C) {
	blockDevices := []state.BlockDeviceInfo{
		{
			DeviceName: "sdb",
			SerialId:   "543554ff-3b88-4",
		},
		{
			DeviceName: "sdc",
			WWN:        "wow",
		},
	}
	volumeInfo := state.VolumeInfo{
		VolumeId: "543554ff-3b88-43b9-83fc-4d69de28490b",
	}
	atachmentInfo := state.VolumeAttachmentInfo{}
	planBlockInfo := state.BlockDeviceInfo{}
	blockDeviceInfo, ok := storagecommon.MatchingBlockDevice(blockDevices, volumeInfo, atachmentInfo, planBlockInfo)
	c.Assert(ok, jc.IsTrue)
	c.Assert(blockDeviceInfo, jc.DeepEquals, &state.BlockDeviceInfo{
		DeviceName: "sdb",
		SerialId:   "543554ff-3b88-4",
	})
}

func (s *BlockDeviceSuite) TestBlockDeviceMatchingHardwareID(c *gc.C) {
	blockDevices := []state.BlockDeviceInfo{
		{
			DeviceName: "sdb",
			HardwareId: "ide-543554ff-3b88-4",
		},
		{
			DeviceName: "sdc",
		},
	}
	volumeInfo := state.VolumeInfo{
		HardwareId: "ide-543554ff-3b88-4",
	}
	atachmentInfo := state.VolumeAttachmentInfo{}
	planBlockInfo := state.BlockDeviceInfo{}
	blockDeviceInfo, ok := storagecommon.MatchingBlockDevice(blockDevices, volumeInfo, atachmentInfo, planBlockInfo)
	c.Assert(ok, jc.IsTrue)
	c.Assert(blockDeviceInfo, jc.DeepEquals, &state.BlockDeviceInfo{
		DeviceName: "sdb",
		HardwareId: "ide-543554ff-3b88-4",
	})
}
