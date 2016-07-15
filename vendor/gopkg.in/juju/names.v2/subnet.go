// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"net"
)

const SubnetTagKind = "subnet"

// IsValidSubnet returns whether cidr is a valid subnet CIDR.
func IsValidSubnet(cidr string) bool {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err == nil && ipNet.String() == cidr {
		return true
	}
	return false
}

type SubnetTag struct {
	cidr string
}

func (t SubnetTag) String() string { return t.Kind() + "-" + t.cidr }
func (t SubnetTag) Kind() string   { return SubnetTagKind }
func (t SubnetTag) Id() string     { return t.cidr }

// NewSubnetTag returns the tag for subnet with the given CIDR.
func NewSubnetTag(cidr string) SubnetTag {
	if !IsValidSubnet(cidr) {
		panic(fmt.Sprintf("%s is not a valid subnet CIDR", cidr))
	}
	return SubnetTag{cidr: cidr}
}

// ParseSubnetTag parses a subnet tag string.
func ParseSubnetTag(subnetTag string) (SubnetTag, error) {
	tag, err := ParseTag(subnetTag)
	if err != nil {
		return SubnetTag{}, err
	}
	subt, ok := tag.(SubnetTag)
	if !ok {
		return SubnetTag{}, invalidTagError(subnetTag, SubnetTagKind)
	}
	return subt, nil
}
