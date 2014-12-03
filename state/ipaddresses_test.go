// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type IPAddressSuite struct {
	ConnSuite
}

var _ = gc.Suite(&IPAddressSuite{})

func (s *IPAddressSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func(*config.Config) (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
}

func (s *IPAddressSuite) TestAddIPAddress(c *gc.C) {
	addr := network.NewAddress("192.168.1.0", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)

	checkAddress := func(ipAddr *state.IPAddress) {
		c.Assert(ipAddr.Value(), gc.Equals, "192.168.1.0")
		c.Assert(ipAddr.SubnetId(), gc.Equals, "foobar")
		c.Assert(ipAddr.Type(), gc.Equals, addr.Type)
		c.Assert(ipAddr.Scope(), gc.Equals, network.ScopePublic)
		c.Assert(ipAddr.State(), gc.Equals, state.AddressStateUnknown)
	}
	checkAddress(ipAddr)

	// verify the address was stored in the state
	ipAddr, err = s.State.IPAddress("192.168.1.0")
	c.Assert(err, jc.ErrorIsNil)
	checkAddress(ipAddr)
}

func (s *IPAddressSuite) TestAddIPAddressInvalid(c *gc.C) {
	addr := network.Address{Value: "foo"}
	_, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(errors.Cause(err), gc.ErrorMatches, `invalid IP address "foo"`)
}

func (s *IPAddressSuite) TestAddIPAddressAlreadyExists(c *gc.C) {
	addr := network.NewAddress("192.168.1.0", network.ScopePublic)
	_, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddIPAddress(addr, "foobar")
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)
}

func (s *IPAddressSuite) TestIPAddressNotFound(c *gc.C) {
	_, err := s.State.IPAddress("192.168.1.0")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *IPAddressSuite) TestIPAddressRemove(c *gc.C) {
	addr := network.NewAddress("192.168.1.0", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)

	err = ipAddr.Remove()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.IPAddress("192.168.1.0")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *IPAddressSuite) TestIPAddressSetState(c *gc.C) {
	addr := network.NewAddress("192.168.1.0", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.State(), gc.Equals, state.AddressStateUnknown)

	err = ipAddr.SetState(state.AddressStateAllocated)
	c.Assert(err, jc.ErrorIsNil)

	freshCopy, err := s.State.IPAddress("192.168.1.0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(freshCopy.State(), gc.Equals, state.AddressStateAllocated)

	// setting the state to the same state is permitted
	err = ipAddr.SetState(state.AddressStateAllocated)
	c.Assert(err, jc.ErrorIsNil)

	// setting back to unknown isn't permitted
	err = ipAddr.SetState(state.AddressStateUnknown)
	c.Assert(err, gc.ErrorMatches, `cannot set IP address .* to state "".*`)
}

func (s *IPAddressSuite) TestIPAddressSetMachineId(c *gc.C) {
	addr := network.NewAddress("192.168.1.0", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.MachineId(), gc.Equals, "")

	err = ipAddr.SetMachineId("wibble")
	c.Assert(err, jc.ErrorIsNil)

	freshCopy, err := s.State.IPAddress("192.168.1.0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(freshCopy.MachineId(), gc.Equals, "wibble")
}

func (s *IPAddressSuite) TestIPAddressSetInterfaceId(c *gc.C) {
	addr := network.NewAddress("192.168.1.0", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.InterfaceId(), gc.Equals, "")

	err = ipAddr.SetInterfaceId("wobble")
	c.Assert(err, jc.ErrorIsNil)

	freshCopy, err := s.State.IPAddress("192.168.1.0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(freshCopy.InterfaceId(), gc.Equals, "wobble")
}

func (s *IPAddressSuite) TestIPAddressAddress(c *gc.C) {
	addr := network.NewAddress("192.168.1.0", network.ScopePublic)
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.Address(), gc.Equals, addr)

}
