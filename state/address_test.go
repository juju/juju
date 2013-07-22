// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
)

type AddressSuite struct{}

var _ = Suite(&AddressSuite{})

func (s *AddressSuite) TestMakeAddress(c *C) {
	addr := state.Address{"0.0.0.0", state.Ipv4Address, "net",
		state.NetworkUnknown}
	c.Check(addr.Value, Equals, "0.0.0.0")
	c.Check(addr.Type, Equals, state.Ipv4Address)
	c.Check(addr.NetworkName, Equals, "net")
	c.Check(addr.NetworkScope, Equals, state.NetworkUnknown)
}
