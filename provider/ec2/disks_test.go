// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"strconv"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	amzec2 "gopkg.in/amz.v2/ec2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type DisksSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&DisksSuite{})

func (*DisksSuite) TestBlockDeviceNamer(c *gc.C) {
	var nextName func() (string, string, error)
	expect := func(expectRequest, expectActual string) {
		request, actual, err := nextName()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(request, gc.Equals, expectRequest)
		c.Assert(actual, gc.Equals, expectActual)
	}
	expectN := func(expectRequest, expectActual string) {
		for i := 1; i <= 6; i++ {
			request, actual, err := nextName()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(request, gc.Equals, expectRequest+strconv.Itoa(i))
			c.Assert(actual, gc.Equals, expectActual+strconv.Itoa(i))
		}
	}
	expectErr := func(expectErr string) {
		_, _, err := nextName()
		c.Assert(err, gc.ErrorMatches, expectErr)
	}

	// First without numbers.
	nextName = ec2.BlockDeviceNamer(false)
	expect("/dev/sdf", "xvdf")
	expect("/dev/sdg", "xvdg")
	expect("/dev/sdh", "xvdh")
	expect("/dev/sdi", "xvdi")
	expect("/dev/sdj", "xvdj")
	expect("/dev/sdk", "xvdk")
	expect("/dev/sdl", "xvdl")
	expect("/dev/sdm", "xvdm")
	expect("/dev/sdn", "xvdn")
	expect("/dev/sdo", "xvdo")
	expect("/dev/sdp", "xvdp")
	expectErr("too many EBS volumes to attach")

	// Now with numbers.
	nextName = ec2.BlockDeviceNamer(true)
	expect("/dev/sdf1", "xvdf1")
	expect("/dev/sdf2", "xvdf2")
	expect("/dev/sdf3", "xvdf3")
	expect("/dev/sdf4", "xvdf4")
	expect("/dev/sdf5", "xvdf5")
	expect("/dev/sdf6", "xvdf6")
	expectN("/dev/sdg", "xvdg")
	expectN("/dev/sdh", "xvdh")
	expectN("/dev/sdi", "xvdi")
	expectN("/dev/sdj", "xvdj")
	expectN("/dev/sdk", "xvdk")
	expectN("/dev/sdl", "xvdl")
	expectN("/dev/sdm", "xvdm")
	expectN("/dev/sdn", "xvdn")
	expectN("/dev/sdo", "xvdo")
	expectN("/dev/sdp", "xvdp")
	expectErr("too many EBS volumes to attach")
}

func (*DisksSuite) TestGetBlockDeviceMappings(c *gc.C) {
	volume0 := names.NewDiskTag("0")
	volume1 := names.NewDiskTag("1")

	mapping, volumes, volumeAttachments, err := ec2.GetBlockDeviceMappings(
		"pv", &environs.StartInstanceParams{Volumes: []storage.VolumeParams{
			{Tag: volume0, Size: 1234},
			{Tag: volume1, Size: 4321},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mapping, gc.DeepEquals, []amzec2.BlockDeviceMapping{{
		VolumeSize: 8,
		DeviceName: "/dev/sda1",
	}, {
		VirtualName: "ephemeral0",
		DeviceName:  "/dev/sdb",
	}, {
		VirtualName: "ephemeral1",
		DeviceName:  "/dev/sdc",
	}, {
		VirtualName: "ephemeral2",
		DeviceName:  "/dev/sdd",
	}, {
		VirtualName: "ephemeral3",
		DeviceName:  "/dev/sde",
	}, {
		VolumeSize: 2,
		DeviceName: "/dev/sdf1",
	}, {
		VolumeSize: 5,
		DeviceName: "/dev/sdf2",
	}})
	c.Assert(volumes, gc.DeepEquals, []storage.Volume{
		{Tag: volume0, Size: 2048},
		{Tag: volume1, Size: 5120},
	})
	c.Assert(volumeAttachments, gc.DeepEquals, []storage.VolumeAttachment{
		{Volume: volume0, DeviceName: "xvdf1"},
		{Volume: volume1, DeviceName: "xvdf2"},
	})
}
