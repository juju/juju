// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

func (*PortRangeSuite) TestParsePortRange(c *gc.C) {
	portRange, err := network.ParsePortRange("8000-8099/tcp")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(portRange.Protocol, gc.Equals, "tcp")
	c.Check(portRange.FromPort, gc.Equals, 8000)
	c.Check(portRange.ToPort, gc.Equals, 8099)
}

func (*PortRangeSuite) TestParsePortRangeSingle(c *gc.C) {
	portRange, err := network.ParsePortRange("80/tcp")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(portRange.Protocol, gc.Equals, "tcp")
	c.Check(portRange.FromPort, gc.Equals, 80)
	c.Check(portRange.ToPort, gc.Equals, 80)
}

func (*PortRangeSuite) TestParsePortRangeDefaultProtocol(c *gc.C) {
	portRange, err := network.ParsePortRange("80")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(portRange.Protocol, gc.Equals, "tcp")
	c.Check(portRange.FromPort, gc.Equals, 80)
	c.Check(portRange.ToPort, gc.Equals, 80)
}

func (*PortRangeSuite) TestParsePortRangePortsMultiRange(c *gc.C) {
	_, err := network.ParsePortRange("10-55-100")

	c.Check(err, gc.ErrorMatches, `invalid port range "10-55-100".*`)
}

func (*PortRangeSuite) TestParsePortRangePortsNonIntPort(c *gc.C) {
	_, err := network.ParsePortRange("spam-100")

	c.Check(err, gc.ErrorMatches, `invalid port "spam".*`)
}
