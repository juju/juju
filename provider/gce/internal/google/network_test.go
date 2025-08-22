// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	gc "gopkg.in/check.v1"
)

type firewallNameSuite struct{}

var _ = gc.Suite(&firewallNameSuite{})

func (s *firewallNameSuite) TestSimplePattern(c *gc.C) {
	res := matchesPrefix("juju-3-123", "juju-3")
	c.Assert(res, gc.Equals, true)
}

func (s *firewallNameSuite) TestExactMatch(c *gc.C) {
	res := matchesPrefix("juju-3", "juju-3")
	c.Assert(res, gc.Equals, true)
}

func (s *firewallNameSuite) TestThatJujuMachineIDsDoNotCollide(c *gc.C) {
	res := matchesPrefix("juju-30-123", "juju-3")
	c.Assert(res, gc.Equals, false)
}
