// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/juju/testing/factory"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type AddressSuite struct{}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestNewAddress(c *gc.C) {
	instanceaddress := network.Address{"0.0.0.0", network.IPv4Address,
		"net", network.ScopeUnknown}
	stateaddress := state.NewAddress(instanceaddress)
	c.Assert(stateaddress, gc.NotNil)
}

func (s *AddressSuite) TestInstanceAddressRoundtrips(c *gc.C) {
	instanceaddress := network.Address{"0.0.0.0", network.IPv4Address,
		"net", network.ScopeUnknown}
	stateaddress := state.NewAddress(instanceaddress)
	addr := stateaddress.InstanceAddress()
	c.Assert(addr, gc.Equals, instanceaddress)
}

type StateServerAddressesSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&StateServerAddressesSuite{})

func (s *StateServerAddressesSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// Make sure there is a machine with manage state in existence.
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageEnviron, state.JobHostUnits},
		Addresses: []network.Address{
			{Value: "192.168.2.144", Type: network.IPv4Address},
			{Value: "10.0.1.2", Type: network.IPv4Address},
		},
	})
	c.Logf("machine addresses: %#v", machine.Addresses())
}

func (s *StateServerAddressesSuite) TestStateServerEnv(c *gc.C) {
	addresses, err := s.State.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, jc.SameContents, []string{"10.0.1.2:1234"})
}

func (s *StateServerAddressesSuite) TestOtherEnv(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, nil)
	defer st.Close()
	addresses, err := st.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, jc.SameContents, []string{"10.0.1.2:1234"})
}
