// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	amzec2 "launchpad.net/goamz/ec2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
)

type Suite struct{}

var _ = gc.Suite(&Suite{})

type RootDiskTest struct {
	name       string
	constraint *uint64
	disksize   uint64
	device     amzec2.BlockDeviceMapping
}

var commonInstanceStoreDisks = []amzec2.BlockDeviceMapping{{
	DeviceName:  "/dev/sdb",
	VirtualName: "ephemeral0",
}, {
	DeviceName:  "/dev/sdc",
	VirtualName: "ephemeral1",
}, {
	DeviceName:  "/dev/sdd",
	VirtualName: "ephemeral2",
}, {
	DeviceName:  "/dev/sde",
	VirtualName: "ephemeral3",
}}

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

func (*Suite) TestRootDiskBlockDeviceMapping(c *gc.C) {
	for _, t := range rootDiskTests {
		c.Logf("Test %s", t.name)
		args := &environs.StartInstanceParams{
			Constraints: constraints.Value{RootDisk: t.constraint},
		}
		mappings, _, err := getBlockDeviceMappings(paravirtual, args)
		c.Assert(err, jc.ErrorIsNil)
		expected := append([]amzec2.BlockDeviceMapping{t.device}, commonInstanceStoreDisks...)
		c.Assert(mappings, gc.DeepEquals, expected)
	}
}

func pInt(i uint64) *uint64 {
	return &i
}

func (*Suite) TestPortsToIPPerms(c *gc.C) {
	testCases := []struct {
		about    string
		ports    []network.PortRange
		expected []amzec2.IPPerm
	}{{
		about: "single port",
		ports: []network.PortRange{{
			FromPort: 80,
			ToPort:   80,
			Protocol: "tcp",
		}},
		expected: []amzec2.IPPerm{{
			Protocol:  "tcp",
			FromPort:  80,
			ToPort:    80,
			SourceIPs: []string{"0.0.0.0/0"},
		}},
	}, {
		about: "multiple ports",
		ports: []network.PortRange{{
			FromPort: 80,
			ToPort:   82,
			Protocol: "tcp",
		}},
		expected: []amzec2.IPPerm{{
			Protocol:  "tcp",
			FromPort:  80,
			ToPort:    82,
			SourceIPs: []string{"0.0.0.0/0"},
		}},
	}, {
		about: "multiple port ranges",
		ports: []network.PortRange{{
			FromPort: 80,
			ToPort:   82,
			Protocol: "tcp",
		}, {
			FromPort: 100,
			ToPort:   120,
			Protocol: "tcp",
		}},
		expected: []amzec2.IPPerm{{
			Protocol:  "tcp",
			FromPort:  80,
			ToPort:    82,
			SourceIPs: []string{"0.0.0.0/0"},
		}, {
			Protocol:  "tcp",
			FromPort:  100,
			ToPort:    120,
			SourceIPs: []string{"0.0.0.0/0"},
		}},
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		ipperms := portsToIPPerms(t.ports)
		c.Assert(ipperms, gc.DeepEquals, t.expected)
	}
}
