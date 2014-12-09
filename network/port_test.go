// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

func (*PortRangeSuite) TestParsePortRangePorts(c *gc.C) {
	portRange, err := network.ParsePortRangePorts("10-100", "tcp")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(portRange.Protocol, gc.Equals, "tcp")
	c.Check(portRange.FromPort, gc.Equals, 10)
	c.Check(portRange.ToPort, gc.Equals, 100)
}

func (*PortRangeSuite) TestParsePortRangePortsSingle(c *gc.C) {
	portRange, err := network.ParsePortRangePorts("10", "tcp")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(portRange.Protocol, gc.Equals, "tcp")
	c.Check(portRange.FromPort, gc.Equals, 10)
	c.Check(portRange.ToPort, gc.Equals, 10)
}

func (*PortRangeSuite) TestParsePortRangePortsMultiRange(c *gc.C) {
	_, err := network.ParsePortRangePorts("10-55-100", "tcp")

	c.Check(err, gc.ErrorMatches, `invalid port range "10-55-100".*`)
}

func (*PortRangeSuite) TestParsePortRangePortsNonIntPort(c *gc.C) {
	_, err := network.ParsePortRangePorts("spam-100", "tcp")

	c.Check(err, gc.ErrorMatches, `invalid port "spam".*`)
}
