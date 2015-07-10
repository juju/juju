// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
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
	c.Assert(ipAddr.Id(), gc.Equals, s.State.EnvironUUID()+":"+addr.Value)
	c.Assert(ipAddr.InstanceId(), gc.Equals, instance.UnknownId)
	c.Assert(ipAddr.MACAddress(), gc.Equals, "")
}

func (s *IPAddressSuite) createMachine(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	return machine
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

func (s *IPAddressSuite) TestIPAddressByTag(c *gc.C) {
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	added, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := added.UUID()
	c.Assert(err, jc.ErrorIsNil)
	tag := names.NewIPAddressTag(uuid.String())
	found, err := s.State.IPAddressByTag(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Id(), gc.Equals, added.Id())
}

func (s *IPAddressSuite) TestIPAddressFindEntity(c *gc.C) {
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	added, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := added.UUID()
	c.Assert(err, jc.ErrorIsNil)
	tag := names.NewIPAddressTag(uuid.String())
	found, err := s.State.FindEntity(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Tag(), gc.Equals, tag)
}

func (s *IPAddressSuite) TestIPAddressByTagNotFound(c *gc.C) {
	tag := names.NewIPAddressTag("42424242-1111-2222-3333-0123456789ab")
	_, err := s.State.IPAddressByTag(tag)
	c.Assert(err, gc.ErrorMatches, `IP address "ipaddress-42424242-1111-2222-3333-0123456789ab" not found`)
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
	machine := s.createMachine(c)
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)

	copyIPAddr, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	err = copyIPAddr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	msg := fmt.Sprintf(`cannot allocate IP address %q to machine %q, interface "frogger": address is dead`, ipAddr.String(), machine.Id())
	err = ipAddr.AllocateTo(machine.Id(), "frogger", "01:23:45:67:89:ab")
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *IPAddressSuite) TestAllocateToProvisionedMachine(c *gc.C) {
	machine := s.createMachine(c)

	addr := network.NewAddress("0.1.2.3")
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)

	err = ipAddr.AllocateTo(machine.Id(), "fake", "01:23:45:67:89:ab")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(ipAddr.InstanceId(), gc.Equals, instance.Id("foo"))
	c.Assert(ipAddr.MACAddress(), gc.Equals, "01:23:45:67:89:ab")
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
	machine := s.createMachine(c)

	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.State(), gc.Equals, state.AddressStateUnknown)
	c.Assert(ipAddr.MachineId(), gc.Equals, "")
	c.Assert(ipAddr.InterfaceId(), gc.Equals, "")
	c.Assert(ipAddr.InstanceId(), gc.Equals, instance.UnknownId)

	err = ipAddr.AllocateTo(machine.Id(), "wobble", "01:23:45:67:89:ab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.State(), gc.Equals, state.AddressStateAllocated)
	c.Assert(ipAddr.MachineId(), gc.Equals, machine.Id())
	c.Assert(ipAddr.InterfaceId(), gc.Equals, "wobble")
	c.Assert(ipAddr.InstanceId(), gc.Equals, instance.Id("foo"))
	c.Assert(ipAddr.MACAddress(), gc.Equals, "01:23:45:67:89:ab")

	freshCopy, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(freshCopy.State(), gc.Equals, state.AddressStateAllocated)
	c.Assert(freshCopy.MachineId(), gc.Equals, machine.Id())
	c.Assert(freshCopy.InterfaceId(), gc.Equals, "wobble")
	c.Assert(freshCopy.InstanceId(), gc.Equals, instance.Id("foo"))
	c.Assert(freshCopy.MACAddress(), gc.Equals, "01:23:45:67:89:ab")

	// allocating twice should fail.
	machine2 := s.createMachine(c)
	err = ipAddr.AllocateTo(machine2.Id(), "i", "01:23:45:67:89:ac")

	msg := fmt.Sprintf(
		`cannot allocate IP address "public:0.1.2.3" to machine %q, interface "i": `+
			`already allocated or unavailable`, machine2.Id())
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *IPAddressSuite) TestAddress(c *gc.C) {
	addr := network.NewScopedAddress("0.1.2.3", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.Address(), jc.DeepEquals, addr)
}

func (s *IPAddressSuite) TestAllocatedIPAddresses(c *gc.C) {
	machine := s.createMachine(c)
	machine2 := s.createMachine(c)
	addresses := [][]string{
		{"0.1.2.3", machine.Id()},
		{"0.1.2.4", machine.Id()},
		{"0.1.2.5", machine2.Id()},
	}
	for _, details := range addresses {
		addr := network.NewScopedAddress(details[0], network.ScopePublic)
		ipAddr, err := s.State.AddIPAddress(addr, "foobar")
		c.Assert(err, jc.ErrorIsNil)
		err = ipAddr.AllocateTo(details[1], "wobble", "01:23:45:67:89:ab")
		c.Assert(err, jc.ErrorIsNil)
	}
	result, err := s.State.AllocatedIPAddresses(machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	addr1, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	addr2, err := s.State.IPAddress("0.1.2.4")
	c.Assert(err, jc.ErrorIsNil)
	expected := []*state.IPAddress{addr1, addr2}
	c.Assert(result, jc.SameContents, expected)
}

func (s *IPAddressSuite) TestDeadIPAddresses(c *gc.C) {
	machine := s.createMachine(c)

	addresses := []string{
		"0.1.2.3",
		"0.1.2.4",
		"0.1.2.5",
		"0.1.2.6",
	}
	for i, details := range addresses {
		addr := network.NewAddress(details)
		ipAddr, err := s.State.AddIPAddress(addr, "foobar")
		c.Assert(err, jc.ErrorIsNil)
		err = ipAddr.AllocateTo(machine.Id(), "wobble", "01:23:45:67:89:ab")
		c.Assert(err, jc.ErrorIsNil)
		if i%2 == 0 {
			err := ipAddr.EnsureDead()
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(ipAddr.Life(), gc.Equals, state.Alive)
		}
	}

	ipAddresses, err := s.State.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	addr1, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	addr3, err := s.State.IPAddress("0.1.2.5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddresses, jc.SameContents, []*state.IPAddress{addr1, addr3})
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
