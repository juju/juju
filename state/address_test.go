// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
)

type AddressSuite struct{}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestNewAddress(c *gc.C) {
	instanceaddress := instance.Address{"0.0.0.0", instance.Ipv4Address,
		"net", instance.NetworkUnknown}
	stateaddress := state.NewAddress(instanceaddress)
	c.Assert(stateaddress, gc.NotNil)
}

func (s *AddressSuite) TestInstanceAddressRoundtrips(c *gc.C) {
	instanceaddress := instance.Address{"0.0.0.0", instance.Ipv4Address,
		"net", instance.NetworkUnknown}
	stateaddress := state.NewAddress(instanceaddress)
	addr := stateaddress.InstanceAddress()
	c.Assert(addr, gc.Equals, instanceaddress)
}
