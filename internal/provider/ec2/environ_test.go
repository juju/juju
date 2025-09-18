// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/storage"
)

// Ensure EC2 provider supports the expected interfaces,
var (
	_ environs.NetworkingEnviron = (*environ)(nil)
	_ config.ConfigSchemaSource  = (*environProvider)(nil)
	_ simplestreams.HasRegion    = (*environ)(nil)
)

type Suite struct{}

func TestSuite(t *testing.T) {
	tc.Run(t, &Suite{})
}

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

func (*Suite) TestRootDiskBlockDeviceMapping(c *tc.C) {
	var rootDiskTests = []RootDiskTest{{
		"jammy",
		"nil constraint ubuntu",
		nil,
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"jammy",
		"too small constraint ubuntu",
		pInt(4000),
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"jammy",
		"big constraint ubuntu",
		pInt(20 * 1024),
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(20)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"jammy",
		"round up constraint ubuntu",
		pInt(20*1024 + 1),
		nil,
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(21)}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"jammy",
		"nil constraint ubuntu with root encryption",
		nil,
		&storage.VolumeParams{
			Attributes: map[string]interface{}{
				"encrypted": true,
			},
		},
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8), Encrypted: aws.Bool(true), VolumeType: types.VolumeTypeGp2}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"jammy",
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
		"jammy",
		"nil constraint ubuntu with root volume type",
		nil,
		&storage.VolumeParams{
			Attributes: map[string]interface{}{
				"volume-type": "magnetic",
			},
		},
		types.BlockDeviceMapping{Ebs: &types.EbsBlockDevice{VolumeSize: aws.Int32(8), VolumeType: types.VolumeTypeStandard}, DeviceName: aws.String("/dev/sda1")},
	}, {
		"jammy",
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
		"jammy",
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
		c.Assert(err, tc.ErrorIsNil)
		expected := append([]types.BlockDeviceMapping{t.device}, commonInstanceStoreDisks...)
		c.Assert(mappings, tc.DeepEquals, expected)
	}
}

func pInt(i uint64) *uint64 {
	return &i
}

func (*Suite) TestPortsToIPPerms(c *tc.C) {
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
				IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0"), Description: aws.String("juju ingress to 80/tcp")}},
				Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0"), Description: aws.String("juju ingress to 80/tcp")}},
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
				IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0"), Description: aws.String("juju ingress to 80-82/tcp")}},
				Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0"), Description: aws.String("juju ingress to 80-82/tcp")}},
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
				IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0"), Description: aws.String("juju ingress to 80-82/tcp")}},
				Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0"), Description: aws.String("juju ingress to 80-82/tcp")}},
			}, {
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(100),
				ToPort:     aws.Int32(120),
				IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0"), Description: aws.String("juju ingress to 100-120/tcp")}},
				Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0"), Description: aws.String("juju ingress to 100-120/tcp")}},
			},
		},
	}, {
		about: "source ranges",
		rules: firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80-82/tcp"), "192.168.1.0/24", "0.0.0.0/0")},
		expected: []types.IpPermission{{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(80),
			ToPort:     aws.Int32(82),
			IpRanges: []types.IpRange{
				{CidrIp: aws.String("0.0.0.0/0"), Description: aws.String("juju ingress to 80-82/tcp")},
				{CidrIp: aws.String("192.168.1.0/24"), Description: aws.String("juju ingress to 80-82/tcp from 192.168.1.0/24")},
			},
		}},
	}, {
		about: "mixed IPV4 and IPV6 CIDRs",
		rules: firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80-82/tcp"), "192.168.1.0/24", "0.0.0.0/0", "::/0")},
		expected: []types.IpPermission{{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(80),
			ToPort:     aws.Int32(82),
			IpRanges: []types.IpRange{
				{CidrIp: aws.String("0.0.0.0/0"), Description: aws.String("juju ingress to 80-82/tcp")},
				{CidrIp: aws.String("192.168.1.0/24"), Description: aws.String("juju ingress to 80-82/tcp from 192.168.1.0/24")},
			},
			Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0"), Description: aws.String("juju ingress to 80-82/tcp")}},
		}},
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		ipperms := rulesToIPPerms(t.rules)
		c.Assert(ipperms, tc.DeepEquals, t.expected)
	}
}

// These Support checks are currently valid with a 'nil' environ pointer. If
// that changes, the tests will need to be updated. (we know statically what is
// supported.)
func (*Suite) TestSupportsNetworking(c *tc.C) {
	var env *environ
	_, supported := environs.SupportsNetworking(env)
	c.Assert(supported, tc.IsTrue)
}

func (*Suite) TestSupportsSpaces(c *tc.C) {
	var env *environ
	supported, err := env.SupportsSpaces()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(supported, tc.IsTrue)
	c.Check(environs.SupportsSpaces(env), tc.IsTrue)
}

func (*Suite) TestSupportsSpaceDiscovery(c *tc.C) {
	supported, err := (&environ{}).SupportsSpaceDiscovery()
	// TODO(jam): 2016-02-01 the comment on the interface says the error should
	// conform to IsNotSupported, but all of the implementations just return
	// nil for error and 'false' for supported.
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(supported, tc.IsFalse)
}

func (*Suite) TestGatherNilAZ(c *tc.C) {
	az := gatherAvailabilityZones(nil)
	c.Assert(az, tc.HasLen, 0)
}

func (*Suite) TestGatherEmptyAZ(c *tc.C) {
	instances := []instances.Instance{}
	az := gatherAvailabilityZones(instances)
	c.Assert(az, tc.HasLen, 0)
}

func (*Suite) TestGatherAZ(c *tc.C) {
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
	c.Assert(az, tc.DeepEquals, map[instance.Id]string{
		"id1": "aaa",
		"id2": "bbb",
	})
}

func ptrString(s string) *string {
	return &s
}
