// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs/network"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type networkSuite struct {
	info []network.Info
}

var _ = gc.Suite(&networkSuite{})

func (n *networkSuite) SetUpTest(c *gc.C) {
	n.info = []network.Info{
		{VLANTag: 1, InterfaceName: "eth0"},
		{VLANTag: 0, InterfaceName: "eth1"},
		{VLANTag: 42, InterfaceName: "br2"},
	}
}

func (n *networkSuite) TestActualInterfaceName(c *gc.C) {
	c.Check(n.info[0].ActualInterfaceName(), gc.Equals, "eth0.1")
	c.Check(n.info[1].ActualInterfaceName(), gc.Equals, "eth1")
	c.Check(n.info[2].ActualInterfaceName(), gc.Equals, "br2.42")
}

func (n *networkSuite) TestIsVirtual(c *gc.C) {
	c.Check(n.info[0].IsVirtual(), jc.IsTrue)
	c.Check(n.info[1].IsVirtual(), jc.IsFalse)
	c.Check(n.info[2].IsVirtual(), jc.IsTrue)
}
