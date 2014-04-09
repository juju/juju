// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type networkSuite struct{}

var _ = gc.Suite(&networkSuite{})

func (s *networkSuite) TestNetworkTag(c *gc.C) {
	c.Assert(names.NetworkTag("net1"), gc.Equals, "network-net1")
}

func (s *networkSuite) TestIsNetwork(c *gc.C) {
	c.Assert(names.IsNetwork("net1"), jc.IsTrue)
	c.Assert(names.IsNetwork("42"), jc.IsTrue)
	c.Assert(names.IsNetwork(""), jc.IsFalse)
}
