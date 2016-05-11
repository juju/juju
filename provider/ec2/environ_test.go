// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	amzec2 "gopkg.in/amz.v3/ec2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// Ensure EC2 provider supports the expected interfaces,
var (
	_ environs.NetworkingEnviron = (*environ)(nil)
	_ simplestreams.HasRegion    = (*environ)(nil)
	_ state.Prechecker           = (*environ)(nil)
	_ state.InstanceDistributor  = (*environ)(nil)
)

type Suite struct{}

var _ = gc.Suite(&Suite{})

type RootDiskTest struct {
	series     string
	name       string
	constraint *uint64
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

func (*Suite) TestRootDiskBlockDeviceMapping(c *gc.C) {
	var rootDiskTests = []RootDiskTest{{
		"trusty",
		"nil constraint ubuntu",
		nil,
		amzec2.BlockDeviceMapping{VolumeSize: 8, DeviceName: "/dev/sda1"},
	}, {
		"trusty",
		"too small constraint ubuntu",
		pInt(4000),
		amzec2.BlockDeviceMapping{VolumeSize: 8, DeviceName: "/dev/sda1"},
	}, {
		"trusty",
		"big constraint ubuntu",
		pInt(20 * 1024),
		amzec2.BlockDeviceMapping{VolumeSize: 20, DeviceName: "/dev/sda1"},
	}, {
		"trusty",
		"round up constraint ubuntu",
		pInt(20*1024 + 1),
		amzec2.BlockDeviceMapping{VolumeSize: 21, DeviceName: "/dev/sda1"},
	}, {
		"win2012r2",
		"nil constraint windows",
		nil,
		amzec2.BlockDeviceMapping{VolumeSize: 40, DeviceName: "/dev/sda1"},
	}, {
		"win2012r2",
		"too small constraint windows",
		pInt(30 * 1024),
		amzec2.BlockDeviceMapping{VolumeSize: 40, DeviceName: "/dev/sda1"},
	}, {
		"win2012r2",
		"big constraint windows",
		pInt(50 * 1024),
		amzec2.BlockDeviceMapping{VolumeSize: 50, DeviceName: "/dev/sda1"},
	}, {
		"win2012r2",
		"round up constraint windows",
		pInt(50*1024 + 1),
		amzec2.BlockDeviceMapping{VolumeSize: 51, DeviceName: "/dev/sda1"},
	}}

	for _, t := range rootDiskTests {
		c.Logf("Test %s", t.name)
		cons := constraints.Value{RootDisk: t.constraint}
		mappings := getBlockDeviceMappings(cons, t.series)
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
