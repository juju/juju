// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

// Unit describes the minimum set of methods required for interacting with the
// assertions provided by FirewallHelper.
type Unit interface {
	Name() string
	OpenClosePortsInSubnet(subnetID string, openPortRanges, closePortRanges []network.PortRange) error
}

// FirewallHelper can be used as a mixin to provide firewall-related assertions
// for test suites.
type FirewallHelper struct {
}

// AssertOpenUnitPort attempts to open a port on the specified subnet.
func (fw FirewallHelper) AssertOpenUnitPort(c *gc.C, u Unit, subnet, protocol string, port int) {
	fw.AssertOpenUnitPorts(c, u, subnet, protocol, port, port)
}

// AssertOpenUnitPorts attempts to open a port range on the specified subnet.
func (FirewallHelper) AssertOpenUnitPorts(c *gc.C, u Unit, subnet, protocol string, from, to int) {
	openRange := network.PortRange{
		Protocol: protocol,
		FromPort: from,
		ToPort:   to,
	}

	err := u.OpenClosePortsInSubnet(subnet, []network.PortRange{openRange}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

// AssertCloseUnitPort attempts to open a port on the specified subnet.
func (fw FirewallHelper) AssertCloseUnitPort(c *gc.C, u Unit, subnet, protocol string, port int) {
	fw.AssertCloseUnitPorts(c, u, subnet, protocol, port, port)
}

// AssertCloseUnitPorts attempts to close a port range on the specified subnet.
func (FirewallHelper) AssertCloseUnitPorts(c *gc.C, u Unit, subnet, protocol string, from, to int) {
	closeRange := network.PortRange{
		Protocol: protocol,
		FromPort: from,
		ToPort:   to,
	}

	err := u.OpenClosePortsInSubnet(subnet, nil, []network.PortRange{closeRange})
	c.Assert(err, jc.ErrorIsNil)
}
