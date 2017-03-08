// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package ec2_test

import (
	jc "github.com/juju/testing/checkers"
	amzec2 "gopkg.in/amz.v3/ec2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/ec2"
)

type subnetMatcherSuite struct{}

var _ = gc.Suite(&subnetMatcherSuite{})

var cannedSubnets = []amzec2.Subnet{{
	Id:                  "subnet-1234abcd",
	State:               "available",
	VPCId:               "vpc-deadbeef",
	CIDRBlock:           "172.30.0.0/24",
	AvailableIPCount:    250,
	AvailZone:           "eu-west-1a",
	DefaultForAZ:        false,
	MapPublicIPOnLaunch: true,
	Tags:                []amzec2.Tag{{Key: "Name", Value: "a"}},
}, {
	Id:                  "subnet-2345bcde",
	State:               "available",
	VPCId:               "vpc-deadbeef",
	CIDRBlock:           "172.30.1.0/24",
	AvailableIPCount:    250,
	AvailZone:           "eu-west-1b",
	DefaultForAZ:        false,
	MapPublicIPOnLaunch: true,
	Tags:                []amzec2.Tag{{Key: "Name", Value: "b"}},
}, {
	Id:                  "subnet-3456cdef",
	State:               "available",
	VPCId:               "vpc-deadbeef",
	CIDRBlock:           "172.30.2.0/24",
	AvailableIPCount:    250,
	AvailZone:           "eu-west-1c",
	DefaultForAZ:        false,
	MapPublicIPOnLaunch: true,
	Tags:                []amzec2.Tag{{Key: "Name", Value: "c"}},
}, {
	Id:                  "subnet-fedc6543",
	State:               "available",
	VPCId:               "vpc-deadbeef",
	CIDRBlock:           "172.30.100.0/24",
	AvailableIPCount:    250,
	AvailZone:           "eu-west-1a",
	DefaultForAZ:        false,
	MapPublicIPOnLaunch: true,
	Tags:                []amzec2.Tag{{Key: "Name", Value: "db-a"}},
}, {
	Id:                  "subnet-edcb5432",
	State:               "available",
	VPCId:               "vpc-deadbeef",
	CIDRBlock:           "172.30.101.0/24",
	AvailableIPCount:    250,
	AvailZone:           "eu-west-1b",
	DefaultForAZ:        false,
	MapPublicIPOnLaunch: true,
	Tags:                []amzec2.Tag{{Key: "Name", Value: "db-b"}},
}, {
	Id:                  "subnet-dcba4321",
	State:               "available",
	VPCId:               "vpc-deadbeef",
	CIDRBlock:           "172.30.102.0/24",
	AvailableIPCount:    250,
	AvailZone:           "eu-west-1c",
	DefaultForAZ:        false,
	MapPublicIPOnLaunch: true,
	Tags: []amzec2.Tag{
		{Key: "UserTag", Value: "b"},
		{Key: "Name", Value: "db-c"},
	},
}}

func checkSubnetMatch(c *gc.C, query, expectedSubnetID string) {
	matcher := ec2.CreateSubnetMatcher(query)
	anyMatch := false
	for _, subnet := range cannedSubnets {
		match := matcher.Match(subnet)
		if subnet.Id == expectedSubnetID {
			c.Check(match, jc.IsTrue,
				gc.Commentf("query %q was supposed to match subnet %#v", query, subnet))
		} else {
			c.Check(match, jc.IsFalse,
				gc.Commentf("query %q was not supposed to match subnet %#v", query, subnet))
		}
		if match {
			anyMatch = true
		}
	}
	if expectedSubnetID == "" {
		c.Check(anyMatch, jc.IsFalse, gc.Commentf("we expected there to be no matches"))
	} else {
		c.Check(anyMatch, jc.IsTrue, gc.Commentf("we expected to find at least one match, but found none"))
	}
}

func (*subnetMatcherSuite) TestCIDRMatch(c *gc.C) {
	checkSubnetMatch(c, "172.30.101.0/24", "subnet-edcb5432")
	// We are a little lenient, the host portion doesn't matter as long as the subnet portion is the same
	checkSubnetMatch(c, "172.30.101.50/24", "subnet-edcb5432")
	// There is no such subnet (yet)
	checkSubnetMatch(c, "172.30.103.0/24", "")
}

func (*subnetMatcherSuite) TestSubnetIDMatch(c *gc.C) {
	checkSubnetMatch(c, "subnet-dcba4321", "subnet-dcba4321")
	// Typo matches nothing
	checkSubnetMatch(c, "subnet-dcba432", "")
}

func (*subnetMatcherSuite) TestSubnetNameMatch(c *gc.C) {
	checkSubnetMatch(c, "b", "subnet-2345bcde")
	// We shouldn't be confused by tags other than "Name"
	checkSubnetMatch(c, "db-c", "subnet-dcba4321")
	// No such named subnet
	checkSubnetMatch(c, "db-q", "")
}
