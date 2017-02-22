// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"net"
	"strings"

	"gopkg.in/amz.v3/ec2"
)

type SubnetMatcher interface {
	Match(ec2.Subnet) bool
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

func (sm *cidrSubnetMatcher) Match(subnet ec2.Subnet) bool {
	_, existingIPNet, err := net.ParseCIDR(subnet.CIDRBlock)
	if err != nil {
		logger.Debugf("subnet %#v has invalid CIDRBlock", subnet)
		return false
	}
	if sm.CIDR == existingIPNet.String() {
		logger.Debugf("found subnet %q by matching subnet CIDR: %s", subnet.Id, sm.CIDR)
		return true
	}
	return false
}

type subnetIDMatcher struct {
	subnetID string
}

func (sm *subnetIDMatcher) Match(subnet ec2.Subnet) bool {
	if subnet.Id == sm.subnetID {
		logger.Debugf("found subnet %q by ID", subnet.Id)
		return true
	}
	return false
}

type subnetNameMatcher struct {
	name string
}

func (sm *subnetNameMatcher) Match(subnet ec2.Subnet) bool {
	for _, tag := range subnet.Tags {
		if tag.Key == "Name" && tag.Value == sm.name {
			logger.Debugf("found subnet %q matching name %q", subnet.Id, sm.name)
			return true
		}
	}
	return false
}
