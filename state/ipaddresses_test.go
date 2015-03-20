// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type IPAddressSuite struct {
	ConnSuite
}

var _ = gc.Suite(&IPAddressSuite{})

func (s *IPAddressSuite) assertAddress(
	c *gc.C,
	ipAddr *state.IPAddress,
	addr network.Address,
	ipState state.AddressState,
	machineId, ifaceId, subnetId string,
) {
	c.Assert(ipAddr, gc.NotNil)
	c.Assert(ipAddr.MachineId(), gc.Equals, machineId)
	c.Assert(ipAddr.InterfaceId(), gc.Equals, ifaceId)
	c.Assert(ipAddr.SubnetId(), gc.Equals, subnetId)
	c.Assert(ipAddr.Value(), gc.Equals, addr.Value)
	c.Assert(ipAddr.Type(), gc.Equals, addr.Type)
	c.Assert(ipAddr.Scope(), gc.Equals, addr.Scope)
	c.Assert(ipAddr.State(), gc.Equals, ipState)
	c.Assert(ipAddr.Address(), jc.DeepEquals, addr)
	c.Assert(ipAddr.String(), gc.Equals, addr.String())
}

func (s *IPAddressSuite) TestAddIPAddress(c *gc.C) {
	for i, test := range []string{"0.1.2.3", "2001:db8::1"} {
		c.Logf("test %d: %q", i, test)
		addr := network.NewScopedAddress(test, network.ScopePublic)
		ipAddr, err := s.State.AddIPAddress(addr, "foobar")
		c.Assert(err, jc.ErrorIsNil)
		s.assertAddress(c, ipAddr, addr, state.AddressStateUnknown, "", "", "foobar")
		c.Assert(ipAddr.Life(), gc.Equals, state.Alive)

		// verify the address was stored in the state
		ipAddr, err = s.State.IPAddress(test)
		c.Assert(err, jc.ErrorIsNil)
		s.assertAddress(c, ipAddr, addr, state.AddressStateUnknown, "", "", "foobar")
	}
}

func (s *IPAddressSuite) TestAddIPAddressInvalid(c *gc.C) {
	addr := network.Address{Value: "foo"}
	_, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err, gc.ErrorMatches, `cannot add IP address "foo": address not valid`)
}

func (s *IPAddressSuite) TestAddIPAddressAlreadyExists(c *gc.C) {
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	_, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Assert(err, gc.ErrorMatches,
		`cannot add IP address "public:0.1.2.3": address already exists`,
	)
}

func (s *IPAddressSuite) TestIPAddressNotFound(c *gc.C) {
	_, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `IP address "0.1.2.3" not found`)
}

func (s *IPAddressSuite) TestEnsureDeadRemove(c *gc.C) {
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)

	// Should not be able to remove an Alive IP address.
	c.Assert(ipAddr.Life(), gc.Equals, state.Alive)
	err = ipAddr.Remove()
	msg := fmt.Sprintf("cannot remove IP address %q: IP address is not dead", ipAddr.String())
	c.Assert(err, gc.ErrorMatches, msg)

	err = ipAddr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// EnsureDead twice should not be an error
	err = ipAddr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	err = ipAddr.Remove()
	c.Assert(err, jc.ErrorIsNil)

	// Remove twice is also fine.
	err = ipAddr.Remove()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `IP address "0.1.2.3" not found`)
}

func (s *IPAddressSuite) TestSetStateDead(c *gc.C) {
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)

	copyIPAddr, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	err = copyIPAddr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	err = ipAddr.SetState(state.AddressStateAllocated)
	msg := fmt.Sprintf(`cannot set IP address %q to state "allocated": address is dead`, ipAddr.String())
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *IPAddressSuite) TestAllocateToDead(c *gc.C) {
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)

	copyIPAddr, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	err = copyIPAddr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	msg := fmt.Sprintf(`cannot allocate IP address %q to machine "foobar", interface "wibble": address is dead`, ipAddr.String())
	err = ipAddr.AllocateTo("foobar", "wibble")
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *IPAddressSuite) TestAddressStateString(c *gc.C) {
	for i, test := range []struct {
		ipState state.AddressState
		expect  string
	}{{
		state.AddressStateUnknown,
		"<unknown>",
	}, {
		state.AddressStateAllocated,
		"allocated",
	}, {
		state.AddressStateUnavailable,
		"unavailable",
	}} {
		c.Logf("test %d: %q -> %q", i, test.ipState, test.expect)
		c.Check(test.ipState.String(), gc.Equals, test.expect)
	}
}

func (s *IPAddressSuite) TestSetState(c *gc.C) {
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)

	for i, test := range []struct {
		initial, changeTo state.AddressState
		err               string
	}{{
		initial:  state.AddressStateUnknown,
		changeTo: state.AddressStateUnknown,
	}, {
		initial:  state.AddressStateUnknown,
		changeTo: state.AddressStateAllocated,
	}, {
		initial:  state.AddressStateUnknown,
		changeTo: state.AddressStateUnavailable,
	}, {
		initial:  state.AddressStateAllocated,
		changeTo: state.AddressStateAllocated,
	}, {
		initial:  state.AddressStateUnavailable,
		changeTo: state.AddressStateUnavailable,
	}, {
		initial:  state.AddressStateAllocated,
		changeTo: state.AddressStateUnknown,
		err: `cannot set IP address "public:0.1.2.3" to state "<unknown>": ` +
			`transition from "allocated" not valid`,
	}, {
		initial:  state.AddressStateUnavailable,
		changeTo: state.AddressStateUnknown,
		err: `cannot set IP address "public:0.1.2.3" to state "<unknown>": ` +
			`transition from "unavailable" not valid`,
	}, {
		initial:  state.AddressStateAllocated,
		changeTo: state.AddressStateUnavailable,
		err: `cannot set IP address "public:0.1.2.3" to state "unavailable": ` +
			`transition from "allocated" not valid`,
	}, {
		initial:  state.AddressStateUnavailable,
		changeTo: state.AddressStateAllocated,
		err: `cannot set IP address "public:0.1.2.3" to state "allocated": ` +
			`transition from "unavailable" not valid`,
	}} {
		c.Logf("test %d: %q -> %q ok:%v", i, test.initial, test.changeTo, test.err == "")
		ipAddr, err := s.State.AddIPAddress(addr, "foobar")
		c.Check(err, jc.ErrorIsNil)

		// Initially, all addresses have AddressStateUnknown.
		c.Assert(ipAddr.State(), gc.Equals, state.AddressStateUnknown)

		if test.initial != state.AddressStateUnknown {
			err = ipAddr.SetState(test.initial)
			c.Check(err, jc.ErrorIsNil)
		}
		err = ipAddr.SetState(test.changeTo)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
			c.Check(err, jc.Satisfies, errors.IsNotValid)
			c.Check(ipAddr.EnsureDead(), jc.ErrorIsNil)
			c.Check(ipAddr.Remove(), jc.ErrorIsNil)
			continue
		}
		c.Check(err, jc.ErrorIsNil)
		c.Check(ipAddr.State(), gc.Equals, test.changeTo)
		c.Check(ipAddr.EnsureDead(), jc.ErrorIsNil)
		c.Check(ipAddr.Remove(), jc.ErrorIsNil)
	}
}

func (s *IPAddressSuite) TestAllocateTo(c *gc.C) {
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.State(), gc.Equals, state.AddressStateUnknown)
	c.Assert(ipAddr.MachineId(), gc.Equals, "")
	c.Assert(ipAddr.InterfaceId(), gc.Equals, "")

	err = ipAddr.AllocateTo("wibble", "wobble")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.State(), gc.Equals, state.AddressStateAllocated)
	c.Assert(ipAddr.MachineId(), gc.Equals, "wibble")
	c.Assert(ipAddr.InterfaceId(), gc.Equals, "wobble")

	freshCopy, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(freshCopy.State(), gc.Equals, state.AddressStateAllocated)
	c.Assert(freshCopy.MachineId(), gc.Equals, "wibble")
	c.Assert(freshCopy.InterfaceId(), gc.Equals, "wobble")

	// allocating twice should fail.
	err = ipAddr.AllocateTo("m", "i")
	c.Assert(err, gc.ErrorMatches,
		`cannot allocate IP address "public:0.1.2.3" to machine "m", interface "i": `+
			`already allocated or unavailable`,
	)
}

func (s *IPAddressSuite) TestAddress(c *gc.C) {
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.Address(), jc.DeepEquals, addr)

}

func (s *IPAddressSuite) TestAllocatedIPAddresses(c *gc.C) {
	addresses := [][]string{
		{"0.1.2.3", "wibble"},
		{"0.1.2.4", "wibble"},
		{"0.1.2.5", "wobble"},
	}
	for _, details := range addresses {
		addr := network.NewScopedAddress(details[0], network.ScopePublic)
		ipAddr, err := s.State.AddIPAddress(addr, "foobar")
		c.Assert(err, jc.ErrorIsNil)
		err = ipAddr.AllocateTo(details[1], "wobble")
		c.Assert(err, jc.ErrorIsNil)
	}
	result, err := s.State.AllocatedIPAddresses("wibble")
	c.Assert(err, jc.ErrorIsNil)
	addr1, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	addr2, err := s.State.IPAddress("0.1.2.4")
	c.Assert(err, jc.ErrorIsNil)
	expected := []*state.IPAddress{addr1, addr2}
	c.Assert(result, jc.SameContents, expected)

}

func (s *IPAddressSuite) TestRefresh(c *gc.C) {
	rawAddr := network.NewAddress("0.1.2.3")
	addr, err := s.State.AddIPAddress(rawAddr, "foobar")
	c.Assert(err, jc.ErrorIsNil)

	addrCopy, err := s.State.IPAddress(rawAddr.Value)
	c.Assert(err, jc.ErrorIsNil)

	err = addr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(addrCopy.Life(), gc.Equals, state.Alive)
	err = addrCopy.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrCopy.Life(), gc.Equals, state.Dead)
}
