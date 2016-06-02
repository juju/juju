// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type BlockDeviceSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&BlockDeviceSerializationSuite{})

func (s *BlockDeviceSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "block devices"
	s.sliceName = "block-devices"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importBlockDevices(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["block-devices"] = []interface{}{}
	}
}

func allBlockDeviceArgs() BlockDeviceArgs {
	return BlockDeviceArgs{
		Name:           "/dev/sda",
		Links:          []string{"some", "data"},
		Label:          "sda",
		UUID:           "some-uuid",
		HardwareID:     "magic",
		BusAddress:     "bus stop",
		Size:           16 * 1024 * 1024 * 1024,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/",
	}
}

func (s *BlockDeviceSerializationSuite) TestNewBlockDevice(c *gc.C) {
	d := newBlockDevice(allBlockDeviceArgs())
	c.Check(d.Name(), gc.Equals, "/dev/sda")
	c.Check(d.Links(), jc.DeepEquals, []string{"some", "data"})
	c.Check(d.Label(), gc.Equals, "sda")
	c.Check(d.UUID(), gc.Equals, "some-uuid")
	c.Check(d.HardwareID(), gc.Equals, "magic")
	c.Check(d.BusAddress(), gc.Equals, "bus stop")
	c.Check(d.Size(), gc.Equals, uint64(16*1024*1024*1024))
	c.Check(d.FilesystemType(), gc.Equals, "ext4")
	c.Check(d.InUse(), jc.IsTrue)
	c.Check(d.MountPoint(), gc.Equals, "/")
}

func (s *BlockDeviceSerializationSuite) exportImport(c *gc.C, dev *blockdevice) *blockdevice {
	initial := blockdevices{
		Version:       1,
		BlockDevices_: []*blockdevice{dev},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	devices, err := importBlockDevices(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 1)
	return devices[0]
}

func (s *BlockDeviceSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := newBlockDevice(allBlockDeviceArgs())
	imported := s.exportImport(c, initial)
	c.Assert(imported, jc.DeepEquals, initial)
}

func (s *BlockDeviceSerializationSuite) TestImportEmpty(c *gc.C) {
	devices, err := importBlockDevices(emptyBlockDeviceMap())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 0)
}

func emptyBlockDeviceMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"version":       1,
		"block-devices": []interface{}{},
	}
}
