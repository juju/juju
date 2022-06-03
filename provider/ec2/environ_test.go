// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/storage"
)

// Ensure EC2 provider supports the expected interfaces,
var (
	_ environs.NetworkingEnviron = (*environ)(nil)
	_ config.ConfigSchemaSource  = (*environProvider)(nil)
	_ simplestreams.HasRegion    = (*environ)(nil)
	_ context.Distributor        = (*environ)(nil)
)

type Suite struct{}

var _ = gc.Suite(&Suite{})

type RootDiskTest struct {
	series         string
	name           string
	constraint     *uint64
	rootDiskParams *storage.VolumeParams
	device         types.BlockDeviceMapping
}

var commonInstanceStoreDisks = []types.BlockDeviceMapping{{
	DeviceName:  aws.String("/dev/sdb"),
	VirtualName: aws.String("ephemeral0"),
}, {
	DeviceName:  aws.String("/dev/sdc"),
	VirtualName: aws.String("ephemeral1"),
}, {
	DeviceName:  aws.String("/dev/sdd"),
	VirtualName: aws.String("ephemeral2"),
}, {
	DeviceName:  aws.String("/dev/sde"),
	VirtualName: aws.String("ephemeral3"),
}}

func (*Suite) TestRootDiskBlockDeviceMapping(c *gc.C) {
	var rootDiskTests = []RootDiskTest{{
		"trusty",
		"nil constraint ubuntu",
		nil,
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"trusty",
		"too small constraint ubuntu",
		pInt(4000),
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"trusty",
		"big constraint ubuntu",
		pInt(20 * 1024),
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(20)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"trusty",
		"round up constraint ubuntu",
		pInt(20*1024 + 1),
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(21)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"win2012r2",
		"nil constraint windows",
		nil,
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(40)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"win2012r2",
		"too small constraint windows",
		pInt(30 * 1024),
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(40)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"win2012r2",
		"big constraint windows",
		pInt(50 * 1024),
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(50)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"win2012r2",
		"round up constraint windows",
		pInt(50*1024 + 1),
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(51)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"trusty",
		"nil constraint ubuntu with root encryption",
		nil,
		&storage.VolumeParams{
			Attributes: map[string]interface{}{
				"encrypted": true,
			},
		},
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8), Encrypted: aws.Bool(true), VolumeType: types.VolumeTypeGp2}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"trusty",
		"nil constraint ubuntu with root custom key encryption",
		nil,
		&storage.VolumeParams{
			Attributes: map[string]interface{}{
				"encrypted":  true,
				"kms-key-id": "1234",
			},
		},
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8), Encrypted: aws.Bool(true), KmsKeyId: aws.String("1234"), VolumeType: types.VolumeTypeGp2}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"trusty",
		"nil constraint ubuntu with root volume type",
		nil,
		&storage.VolumeParams{
			Attributes: map[string]interface{}{
				"volume-type": "magnetic",
			},
		},
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8), VolumeType: types.VolumeTypeStandard}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"trusty",
		"nil constraint ubuntu with throughput",
		nil,
		&storage.VolumeParams{
			Attributes: map[string]interface{}{
				"volume-type": "gp3",
				"throughput":  "10",
			},
		},
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8), VolumeType: types.VolumeTypeGp3, Throughput: aws.Int32(10)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"trusty",
		"nil constraint ubuntu with throughput",
		nil,
		&storage.VolumeParams{
			Attributes: map[string]interface{}{
				"volume-type": "gp3",
				"throughput":  "1G",
			},
		},
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8), VolumeType: types.VolumeTypeGp3, Throughput: aws.Int32(1024)}, DeviceName: aws.String("/dev/sda1")},
	}}

	for _, t := range rootDiskTests {
		c.Logf("Test %s", t.name)
		cons := constraints.Value{RootDisk: t.constraint}
		mappings, err := getBlockDeviceMappings(cons, t.series, false, t.rootDiskParams)
		c.Assert(err, jc.ErrorIsNil)
		expected := append([]types.BlockDeviceMapping{t.device}, commonInstanceStoreDisks...)
		c.Assert(mappings, gc.DeepEquals, expected)
	}
}

func pInt(i uint64) *uint64 {
	return &i
}

func (*Suite) TestPortsToIPPerms(c *gc.C) {
	testCases := []struct {
		about    string
		rules    firewall.IngressRules
		expected []types.IpPermission
	}{{
		about: "single port",
		rules: firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))},
		expected: []types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(80),
				ToPort:     aws.Int32(80),
				IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
				Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0")}},
			},
		},
	}, {
		about: "multiple ports",
		rules: firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80-82/tcp"))},
		expected: []types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(80),
				ToPort:     aws.Int32(82),
				IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
				Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0")}},
			},
		},
	}, {
		about: "multiple port ranges",
		rules: firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("80-82/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("100-120/tcp")),
		},
		expected: []types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(80),
				ToPort:     aws.Int32(82),
				IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
				Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0")}},
			}, {
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(100),
				ToPort:     aws.Int32(120),
				IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
				Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0")}},
			},
		},
	}, {
		about: "source ranges",
		rules: firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80-82/tcp"), "192.168.1.0/24", "0.0.0.0/0")},
		expected: []types.IpPermission{{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(80),
			ToPort:     aws.Int32(82),
			IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}, {CidrIp: aws.String("192.168.1.0/24")}},
		}},
	}, {
		about: "mixed IPV4 and IPV6 CIDRs",
		rules: firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80-82/tcp"), "192.168.1.0/24", "0.0.0.0/0", "::/0")},
		expected: []types.IpPermission{{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(80),
			ToPort:     aws.Int32(82),
			IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}, {CidrIp: aws.String("192.168.1.0/24")}},
			Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0")}},
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
func (*Suite) TestSupportsNetworking(c *gc.C) {
	var env *environ
	_, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)
}

func (*Suite) TestSupportsSpaces(c *gc.C) {
	callCtx := context.NewEmptyCloudCallContext()
	var env *environ
	supported, err := env.SupportsSpaces(callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsTrue)
	c.Check(environs.SupportsSpaces(callCtx, env), jc.IsTrue)
}

func (*Suite) TestSupportsSpaceDiscovery(c *gc.C) {
	supported, err := (&environ{}).SupportsSpaceDiscovery(context.NewEmptyCloudCallContext())
	// TODO(jam): 2016-02-01 the comment on the interface says the error should
	// conform to IsNotSupported, but all of the implementations just return
	// nil for error and 'false' for supported.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsFalse)
}

func (*Suite) TestSupportsContainerAddresses(c *gc.C) {
	callCtx := context.NewEmptyCloudCallContext()
	var env *environ
	supported, err := env.SupportsContainerAddresses(callCtx)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(supported, jc.IsFalse)
	c.Check(environs.SupportsContainerAddresses(callCtx, env), jc.IsFalse)
}

func (*Suite) TestSelectSubnetIDsForZone(c *gc.C) {
	subnetZones := map[network.Id][]string{
		network.Id("bar"): {"foo"},
	}
	placement := network.Id("")
	az := "foo"

	var env *environ
	subnets, err := env.selectSubnetIDsForZone(subnetZones, placement, az)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.DeepEquals, []network.Id{"bar"})
}

func (*Suite) TestSelectSubnetIDsForZones(c *gc.C) {
	subnetZones := map[network.Id][]string{
		network.Id("bar"): {"foo"},
		network.Id("baz"): {"foo"},
	}
	placement := network.Id("")
	az := "foo"

	var env *environ
	subnets, err := env.selectSubnetIDsForZone(subnetZones, placement, az)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.DeepEquals, []network.Id{"bar", "baz"})
}

func (*Suite) TestSelectSubnetIDsForZoneWithPlacement(c *gc.C) {
	subnetZones := map[network.Id][]string{
		network.Id("bar"): {"foo"},
		network.Id("baz"): {"foo"},
	}
	placement := network.Id("baz")
	az := "foo"

	var env *environ
	subnets, err := env.selectSubnetIDsForZone(subnetZones, placement, az)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.DeepEquals, []network.Id{"baz"})
}

func (*Suite) TestSelectSubnetIDsForZoneWithIncorrectPlacement(c *gc.C) {
	subnetZones := map[network.Id][]string{
		network.Id("bar"): {"foo"},
		network.Id("baz"): {"foo"},
	}
	placement := network.Id("boom")
	az := "foo"

	var env *environ
	_, err := env.selectSubnetIDsForZone(subnetZones, placement, az)
	c.Assert(err, gc.ErrorMatches, `subnets "boom" in AZ "foo" not found`)
}

func (*Suite) TestSelectSubnetIDForInstance(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockContext := NewMockProviderCallContext(ctrl)

	subnetZones := map[network.Id][]string{
		network.Id("some-sub"): {"some-az"},
		network.Id("baz"):      {"foo"},
	}
	placement := network.Id("")
	az := "foo"

	var env *environ
	subnet, err := env.selectSubnetIDForInstance(mockContext, false, subnetZones, placement, az)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet, gc.DeepEquals, "baz")
}

func (*Suite) TestSelectSubnetIDForInstanceSelection(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockContext := NewMockProviderCallContext(ctrl)

	subnetZones := map[network.Id][]string{
		network.Id("baz"): {"foo"},
		network.Id("taz"): {"foo"},
	}
	placement := network.Id("")
	az := "foo"

	var env *environ
	subnet, err := env.selectSubnetIDForInstance(mockContext, false, subnetZones, placement, az)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.HasSuffix(subnet, "az"), jc.IsTrue)
}

func (*Suite) TestSelectSubnetIDForInstanceWithNoMatchingZones(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockContext := NewMockProviderCallContext(ctrl)

	subnetZones := map[network.Id][]string{}
	placement := network.Id("")
	az := "invalid"

	var env *environ
	subnet, err := env.selectSubnetIDForInstance(mockContext, false, subnetZones, placement, az)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet, gc.Equals, "")
}

func (*Suite) TestGetValidSubnetZoneMapOneSpaceConstraint(c *gc.C) {
	allSubnetZones := []map[network.Id][]string{
		{network.Id("sub-1"): {"az-1"}},
	}

	args := environs.StartInstanceParams{
		Constraints:    constraints.MustParse("spaces=admin"),
		SubnetsToZones: allSubnetZones,
	}

	subnetZones, err := getValidSubnetZoneMap(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnetZones, gc.DeepEquals, allSubnetZones[0])
}

func (*Suite) TestGetValidSubnetZoneMapOneBindingFanFiltered(c *gc.C) {
	allSubnetZones := []map[network.Id][]string{{
		network.Id("sub-1"):       {"az-1"},
		network.Id("sub-INFAN-2"): {"az-2"},
	}}

	args := environs.StartInstanceParams{
		SubnetsToZones: allSubnetZones,
		EndpointBindings: map[string]network.Id{
			"":    "space-1",
			"ep1": "space-1",
			"ep2": "space-1",
		},
	}

	subnetZones, err := getValidSubnetZoneMap(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnetZones, gc.DeepEquals, map[network.Id][]string{
		"sub-1": {"az-1"},
	})
}

func (*Suite) TestGetValidSubnetZoneMapNoIntersectionError(c *gc.C) {
	allSubnetZones := []map[network.Id][]string{
		{network.Id("sub-1"): {"az-1"}},
		{network.Id("sub-2"): {"az-2"}},
	}

	args := environs.StartInstanceParams{
		SubnetsToZones: allSubnetZones,
		Constraints:    constraints.MustParse("spaces=admin"),
		EndpointBindings: map[string]network.Id{
			"":    "space-1",
			"ep1": "space-1",
			"ep2": "space-1",
		},
	}

	_, err := getValidSubnetZoneMap(args)
	c.Assert(err, gc.ErrorMatches,
		`unable to satisfy supplied space requirements; spaces: \[admin\], bindings: \[space-1\]`)
}

func (*Suite) TestGetValidSubnetZoneMapIntersectionSelectsCorrectIndex(c *gc.C) {
	allSubnetZones := []map[network.Id][]string{
		{network.Id("sub-1"): {"az-1"}},
		{network.Id("sub-2"): {"az-2"}},
		{network.Id("sub-3"): {"az-2"}},
	}

	args := environs.StartInstanceParams{
		SubnetsToZones: allSubnetZones,
		Constraints:    constraints.MustParse("spaces=space-2,space-3"),
		EndpointBindings: map[string]network.Id{
			"":    "space-1",
			"ep1": "space-2",
			"ep2": "space-2",
		},
	}

	// space-2 is common to the bindings and constraints and is at index 1
	// of the sorted union.
	// This should result in the selection of the same index from the
	// subnets-to-zones map.

	subnetZones, err := getValidSubnetZoneMap(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnetZones, gc.DeepEquals, allSubnetZones[1])
}

func (*Suite) TestGatherNilAZ(c *gc.C) {
	az := gatherAvailabilityZones(nil)
	c.Assert(az, gc.HasLen, 0)
}

func (*Suite) TestGatherEmptyAZ(c *gc.C) {
	instances := []instances.Instance{}
	az := gatherAvailabilityZones(instances)
	c.Assert(az, gc.HasLen, 0)
}

func (*Suite) TestGatherAZ(c *gc.C) {
	instances := []instances.Instance{
		&sdkInstance{
			i: types.Instance{
				InstanceId: ptrString("id1"),
				Placement: &types.Placement{
					AvailabilityZone: ptrString("aaa"),
				},
			},
		},
		&sdkInstance{
			i: types.Instance{
				InstanceId: ptrString("id2"),
				Placement: &types.Placement{
					AvailabilityZone: ptrString("bbb"),
				},
			},
		},
		&sdkInstance{
			i: types.Instance{
				InstanceId: ptrString("id3"),
			},
		},
	}
	az := gatherAvailabilityZones(instances)
	c.Assert(az, gc.DeepEquals, map[instance.Id]string{
		"id1": "aaa",
		"id2": "bbb",
	})
}

func ptrString(s string) *string {
	return &s
}
