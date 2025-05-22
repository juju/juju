// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	stdtesting "testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/ec2"
)

type subnetMatcherSuite struct{}

func TestSubnetMatcherSuite(t *stdtesting.T) {
	tc.Run(t, &subnetMatcherSuite{})
}

var cannedSubnets = []types.Subnet{{
	SubnetId:                aws.String("subnet-1234abcd"),
	State:                   "available",
	VpcId:                   aws.String("vpc-deadbeef"),
	CidrBlock:               aws.String("172.30.0.0/24"),
	AvailableIpAddressCount: aws.Int32(250),
	AvailabilityZone:        aws.String("eu-west-1a"),
	DefaultForAz:            aws.Bool(false),
	MapPublicIpOnLaunch:     aws.Bool(true),
	Tags:                    []types.Tag{{Key: aws.String("Name"), Value: aws.String("a")}},
}, {
	SubnetId:                aws.String("subnet-2345bcde"),
	State:                   "available",
	VpcId:                   aws.String("vpc-deadbeef"),
	CidrBlock:               aws.String("172.30.1.0/24"),
	AvailableIpAddressCount: aws.Int32(250),
	AvailabilityZone:        aws.String("eu-west-1b"),
	DefaultForAz:            aws.Bool(false),
	MapPublicIpOnLaunch:     aws.Bool(true),
	Tags:                    []types.Tag{{Key: aws.String("Name"), Value: aws.String("b")}},
}, {
	SubnetId:                aws.String("subnet-3456cdef"),
	State:                   "available",
	VpcId:                   aws.String("vpc-deadbeef"),
	CidrBlock:               aws.String("172.30.2.0/24"),
	AvailableIpAddressCount: aws.Int32(250),
	AvailabilityZone:        aws.String("eu-west-1c"),
	DefaultForAz:            aws.Bool(false),
	MapPublicIpOnLaunch:     aws.Bool(true),
	Tags:                    []types.Tag{{Key: aws.String("Name"), Value: aws.String("c")}},
}, {
	SubnetId:                aws.String("subnet-fedc6543"),
	State:                   "available",
	VpcId:                   aws.String("vpc-deadbeef"),
	CidrBlock:               aws.String("172.30.100.0/24"),
	AvailableIpAddressCount: aws.Int32(250),
	AvailabilityZone:        aws.String("eu-west-1a"),
	DefaultForAz:            aws.Bool(false),
	MapPublicIpOnLaunch:     aws.Bool(true),
	Tags:                    []types.Tag{{Key: aws.String("Name"), Value: aws.String("db-a")}},
}, {
	SubnetId:                aws.String("subnet-edcb5432"),
	State:                   "available",
	VpcId:                   aws.String("vpc-deadbeef"),
	CidrBlock:               aws.String("172.30.101.0/24"),
	AvailableIpAddressCount: aws.Int32(250),
	AvailabilityZone:        aws.String("eu-west-1b"),
	DefaultForAz:            aws.Bool(false),
	MapPublicIpOnLaunch:     aws.Bool(true),
	Tags:                    []types.Tag{{Key: aws.String("Name"), Value: aws.String("db-b")}},
}, {
	SubnetId:                aws.String("subnet-dcba4321"),
	State:                   "available",
	VpcId:                   aws.String("vpc-deadbeef"),
	CidrBlock:               aws.String("172.30.102.0/24"),
	AvailableIpAddressCount: aws.Int32(250),
	AvailabilityZone:        aws.String("eu-west-1c"),
	DefaultForAz:            aws.Bool(false),
	MapPublicIpOnLaunch:     aws.Bool(true),
	Tags: []types.Tag{
		{Key: aws.String("UserTag"), Value: aws.String("b")},
		{Key: aws.String("Name"), Value: aws.String("db-c")},
	},
}}

func checkSubnetMatch(c *tc.C, query, expectedSubnetID string) {
	matcher := ec2.CreateSubnetMatcher(query)
	anyMatch := false
	for _, subnet := range cannedSubnets {
		match := matcher.Match(subnet)
		if aws.ToString(subnet.SubnetId) == expectedSubnetID {
			c.Check(match, tc.IsTrue,
				tc.Commentf("query %q was supposed to match subnet %#v", query, subnet))
		} else {
			c.Check(match, tc.IsFalse,
				tc.Commentf("query %q was not supposed to match subnet %#v", query, subnet))
		}
		if match {
			anyMatch = true
		}
	}
	if expectedSubnetID == "" {
		c.Check(anyMatch, tc.IsFalse, tc.Commentf("we expected there to be no matches"))
	} else {
		c.Check(anyMatch, tc.IsTrue, tc.Commentf("we expected to find at least one match, but found none"))
	}
}

func (*subnetMatcherSuite) TestCIDRMatch(c *tc.C) {
	checkSubnetMatch(c, "172.30.101.0/24", "subnet-edcb5432")
	// We are a little lenient, the host portion doesn't matter as long as the subnet portion is the same
	checkSubnetMatch(c, "172.30.101.50/24", "subnet-edcb5432")
	// There is no such subnet (yet)
	checkSubnetMatch(c, "172.30.103.0/24", "")
}

func (*subnetMatcherSuite) TestSubnetIDMatch(c *tc.C) {
	checkSubnetMatch(c, "subnet-dcba4321", "subnet-dcba4321")
	// Typo matches nothing
	checkSubnetMatch(c, "subnet-dcba432", "")
}

func (*subnetMatcherSuite) TestSubnetNameMatch(c *tc.C) {
	checkSubnetMatch(c, "b", "subnet-2345bcde")
	// We shouldn't be confused by tags other than "Name"
	checkSubnetMatch(c, "db-c", "subnet-dcba4321")
	// No such named subnet
	checkSubnetMatch(c, "db-q", "")
}
