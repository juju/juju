// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	amzec2 "gopkg.in/amz.v3/ec2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/network"
)

// Ensure EC2 provider supports the expected interfaces,
var (
	_ environs.NetworkingEnviron = (*environ)(nil)
	_ config.ConfigSchemaSource  = (*environProvider)(nil)
	_ simplestreams.HasRegion    = (*environ)(nil)
	_ context.Distributor        = (*environ)(nil)
)

type environSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&environSuite{})

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

func (*environSuite) TestRootDiskBlockDeviceMapping(c *gc.C) {
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
		mappings := getBlockDeviceMappings(cons, t.series, false)
		expected := append([]amzec2.BlockDeviceMapping{t.device}, commonInstanceStoreDisks...)
		c.Assert(mappings, gc.DeepEquals, expected)
	}
}

func pInt(i uint64) *uint64 {
	return &i
}

func (*environSuite) TestPortsToIPPerms(c *gc.C) {
	testCases := []struct {
		about    string
		rules    []network.IngressRule
		expected []amzec2.IPPerm
	}{{
		about: "single port",
		rules: []network.IngressRule{network.MustNewIngressRule("tcp", 80, 80)},
		expected: []amzec2.IPPerm{{
			Protocol:  "tcp",
			FromPort:  80,
			ToPort:    80,
			SourceIPs: []string{"0.0.0.0/0"},
		}},
	}, {
		about: "multiple ports",
		rules: []network.IngressRule{network.MustNewIngressRule("tcp", 80, 82)},
		expected: []amzec2.IPPerm{{
			Protocol:  "tcp",
			FromPort:  80,
			ToPort:    82,
			SourceIPs: []string{"0.0.0.0/0"},
		}},
	}, {
		about: "multiple port ranges",
		rules: []network.IngressRule{
			network.MustNewIngressRule("tcp", 80, 82),
			network.MustNewIngressRule("tcp", 100, 120),
		},
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
	}, {
		about: "source ranges",
		rules: []network.IngressRule{network.MustNewIngressRule("tcp", 80, 82, "192.168.1.0/24", "0.0.0.0/0")},
		expected: []amzec2.IPPerm{{
			Protocol:  "tcp",
			FromPort:  80,
			ToPort:    82,
			SourceIPs: []string{"192.168.1.0/24", "0.0.0.0/0"},
		}},
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		ipperms := rulesToIPPerms(t.rules)
		c.Assert(ipperms, gc.DeepEquals, t.expected)
	}
}

// These Support checks are currently valid with a 'nil' environ pointer. If
// that changes, the tests will need to be updated. (we know statically what is
// supported.)
func (*environSuite) TestSupportsNetworking(c *gc.C) {
	var env *environ
	_, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)
}

func (*environSuite) TestSupportsSpaces(c *gc.C) {
	callCtx := context.NewCloudCallContext()
	var env *environ
	supported, err := env.SupportsSpaces(callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsTrue)
	c.Check(environs.SupportsSpaces(callCtx, env), jc.IsTrue)
}

func (*environSuite) TestSupportsSpaceDiscovery(c *gc.C) {
	var env *environ
	supported, err := env.SupportsSpaceDiscovery(context.NewCloudCallContext())
	// TODO(jam): 2016-02-01 the comment on the interface says the error should
	// conform to IsNotSupported, but all of the implementations just return
	// nil for error and 'false' for supported.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsFalse)
}

func (*environSuite) TestSupportsContainerAddresses(c *gc.C) {
	callCtx := context.NewCloudCallContext()
	var env *environ
	supported, err := env.SupportsContainerAddresses(callCtx)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(supported, jc.IsFalse)
	c.Check(environs.SupportsContainerAddresses(callCtx, env), jc.IsFalse)
}

func (*environSuite) TestSpaceSetIDReturnsVPCIfSet(c *gc.C) {
	vpcID := "vpc-123"

	env := &environ{
		ecfgUnlocked: &environConfig{
			attrs: map[string]interface{}{
				"vpc-id": "vpc-123",
			},
		},
		defaultVPCChecked: true,
		defaultVPC: &amzec2.VPC{
			Id: vpcID,
		},
	}

	spaceSetID, err := env.SpaceSetID(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaceSetID, gc.Equals, vpcID)
}

func (*environSuite) TestSpaceSetIDReturnsCredentialIfVPCNotSet(c *gc.C) {
	vpcID := ""
	accessKey := "access-123"
	cred := cloud.NewCredential("whatever", map[string]string{"access-key": accessKey})

	env := &environ{
		ecfgUnlocked: &environConfig{
			attrs: map[string]interface{}{
				"vpc-id": "",
			},
		},
		defaultVPCChecked: true,
		defaultVPC: &amzec2.VPC{
			Id: vpcID,
		},
		cloud: environs.CloudSpec{
			Credential: &cred,
		},
	}

	spaceSetID, err := env.SpaceSetID(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaceSetID, gc.Equals, accessKey)
}
