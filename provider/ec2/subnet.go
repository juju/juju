// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"net"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kr/pretty"
)

type SubnetMatcher interface {
	Match(types.Subnet) bool
}

// CreateSubnetMatcher creates a SubnetMatcher that handles a particular method
// of comparison based on the content of the subnet query. If the query looks
// like a CIDR, then we will match subnets with the same CIDR. If it follows
// the syntax of a "subnet-XXXX" then we will match the Subnet ID. Everything
// else is just matched as a Name.
func CreateSubnetMatcher(subnetQuery string) SubnetMatcher {
	logger.Debugf("searching for subnet matching placement directive %q", subnetQuery)
	_, ipNet, err := net.ParseCIDR(subnetQuery)
	if err == nil {
		return &cidrSubnetMatcher{
			ipNet: ipNet,
			CIDR:  ipNet.String(),
		}
	}
	if strings.HasPrefix(subnetQuery, "subnet-") {
		return &subnetIDMatcher{
			subnetID: subnetQuery,
		}
	}
	return &subnetNameMatcher{
		name: subnetQuery,
	}
}

type cidrSubnetMatcher struct {
	ipNet *net.IPNet
	CIDR  string
}

var _ SubnetMatcher = (*cidrSubnetMatcher)(nil)

func (sm *cidrSubnetMatcher) Match(subnet types.Subnet) bool {
	_, existingIPNet, err := net.ParseCIDR(aws.ToString(subnet.CidrBlock))
	if err != nil {
		logger.Debugf("subnet %s has invalid CIDRBlock", pretty.Sprint(subnet))
		return false
	}
	if sm.CIDR == existingIPNet.String() {
		logger.Debugf("found subnet %q by matching subnet CIDR: %s", aws.ToString(subnet.SubnetId), sm.CIDR)
		return true
	}
	return false
}

type subnetIDMatcher struct {
	subnetID string
}

func (sm *subnetIDMatcher) Match(subnet types.Subnet) bool {
	if subnetID := aws.ToString(subnet.SubnetId); subnetID == sm.subnetID {
		logger.Debugf("found subnet %q by ID", subnetID)
		return true
	}
	return false
}

type subnetNameMatcher struct {
	name string
}

func (sm *subnetNameMatcher) Match(subnet types.Subnet) bool {
	for _, tag := range subnet.Tags {
		if aws.ToString(tag.Key) == "Name" && aws.ToString(tag.Value) == sm.name {
			logger.Debugf("found subnet %q matching name %q", aws.ToString(subnet.SubnetId), sm.name)
			return true
		}
	}
	return false
}
