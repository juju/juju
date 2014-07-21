// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type InfoSuite struct {
	info []network.Info
}

var _ = gc.Suite(&InfoSuite{})

func (n *InfoSuite) SetUpTest(c *gc.C) {
	n.info = []network.Info{
		{VLANTag: 1, InterfaceName: "eth0"},
		{VLANTag: 0, InterfaceName: "eth1"},
		{VLANTag: 42, InterfaceName: "br2"},
	}
}

func (n *InfoSuite) TestActualInterfaceName(c *gc.C) {
	c.Check(n.info[0].ActualInterfaceName(), gc.Equals, "eth0.1")
	c.Check(n.info[1].ActualInterfaceName(), gc.Equals, "eth1")
	c.Check(n.info[2].ActualInterfaceName(), gc.Equals, "br2.42")
}

func (n *InfoSuite) TestIsVirtual(c *gc.C) {
	c.Check(n.info[0].IsVirtual(), jc.IsTrue)
	c.Check(n.info[1].IsVirtual(), jc.IsFalse)
	c.Check(n.info[2].IsVirtual(), jc.IsTrue)
}

type NetworkSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&NetworkSuite{})

func (*NetworkSuite) TestInitializeFromConfig(c *gc.C) {
	c.Check(network.GetPreferIPv6(), jc.IsFalse)

	envConfig := testing.CustomEnvironConfig(c, testing.Attrs{
		"prefer-ipv6": true,
	})
	network.InitializeFromConfig(envConfig)
	c.Check(network.GetPreferIPv6(), jc.IsTrue)

	envConfig = testing.CustomEnvironConfig(c, testing.Attrs{
		"prefer-ipv6": false,
	})
	network.InitializeFromConfig(envConfig)
	c.Check(network.GetPreferIPv6(), jc.IsFalse)
}
