// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
)

type AddressSuite struct{}

var _ = Suite(&AddressSuite{})

func (s *AddressSuite) TestNewAddress(c *C) {
	instanceaddress := instance.Address{"0.0.0.0", instance.Ipv4Address,
		"net", instance.NetworkUnknown}
	stateaddress := state.NewAddress(instanceaddress)
	c.Assert(stateaddress, NotNil)
}

func (s *AddressSuite) TestInstanceAddressRoundtrips(c *C) {
	instanceaddress := instance.Address{"0.0.0.0", instance.Ipv4Address,
		"net", instance.NetworkUnknown}
	stateaddress := state.NewAddress(instanceaddress)
	addr := stateaddress.InstanceAddress()
	c.Assert(addr, Equals, instanceaddress)
}
