// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	amzec2 "launchpad.net/goamz/ec2"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
)

type Suite struct{}

var _ = gc.Suite(&Suite{})

type RootDiskTest struct {
	name       string
	constraint *uint64
	disksize   uint64
	device     amzec2.BlockDeviceMapping
}

var rootDiskTests = []RootDiskTest{
	{
		"nil constraint",
		nil,
		8192,
		amzec2.BlockDeviceMapping{VolumeSize: 8, DeviceName: "/dev/sda1"},
	},
	{
		"too small constraint",
		pInt(4000),
		8192,
		amzec2.BlockDeviceMapping{VolumeSize: 8, DeviceName: "/dev/sda1"},
	},
	{
		"big constraint",
		pInt(20 * 1024),
		20 * 1024,
		amzec2.BlockDeviceMapping{VolumeSize: 20, DeviceName: "/dev/sda1"},
	},
	{
		"round up constraint",
		pInt(20*1024 + 1),
		21 * 1024,
		amzec2.BlockDeviceMapping{VolumeSize: 21, DeviceName: "/dev/sda1"},
	},
}

func (*Suite) TestRootDisk(c *gc.C) {
	for _, t := range rootDiskTests {
		c.Logf("Test %s", t.name)
		cons := constraints.Value{RootDisk: t.constraint}
		device, size := getDiskSize(cons)
		c.Check(size, gc.Equals, t.disksize)
		c.Check(device, gc.DeepEquals, t.device)
	}
}

func pInt(i uint64) *uint64 {
	return &i
}
