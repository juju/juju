// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type AddressSuite struct{}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestAddressConversion(c *gc.C) {
	netAddress := network.Address{
		Value:       "0.0.0.0",
		Type:        network.IPv4Address,
		NetworkName: "net",
		Scope:       network.ScopeUnknown,
	}
	state.AssertAddressConversion(c, netAddress)
}

func (s *AddressSuite) TestHostPortConversion(c *gc.C) {
	netAddress := network.Address{
		Value:       "0.0.0.0",
		Type:        network.IPv4Address,
		NetworkName: "net",
		Scope:       network.ScopeUnknown,
	}
	netHostPort := network.HostPort{netAddress, 4711}
	state.AssertHostPortConversion(c, netHostPort)
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
