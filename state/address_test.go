// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type AddressSuite struct{}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestAddressConversion(c *gc.C) {
	netAddress := network.Address{
		Value: "0.0.0.0",
		Type:  network.IPv4Address,
		Scope: network.ScopeUnknown,
	}
	state.AssertAddressConversion(c, netAddress)
}

func (s *AddressSuite) TestHostPortConversion(c *gc.C) {
	netAddress := network.Address{
		Value: "0.0.0.0",
		Type:  network.IPv4Address,
		Scope: network.ScopeUnknown,
	}
	netHostPort := network.HostPort{
		Address: netAddress,
		Port:    4711,
	}
	state.AssertHostPortConversion(c, netHostPort)
}

type ControllerAddressesSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ControllerAddressesSuite{})

func (s *ControllerAddressesSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	// Make sure there is a machine with manage state in existence.
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel, state.JobHostUnits},
		Addresses: []network.Address{
			{Value: "192.168.2.144", Type: network.IPv4Address},
			{Value: "10.0.1.2", Type: network.IPv4Address},
		},
	})
	c.Logf("machine addresses: %#v", machine.Addresses())
}

func (s *ControllerAddressesSuite) TestControllerEnv(c *gc.C) {
	addresses, err := s.State.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, jc.SameContents, []string{"10.0.1.2:1234"})
}

func (s *ControllerAddressesSuite) TestOtherEnv(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	addresses, err := st.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, jc.SameContents, []string{"10.0.1.2:1234"})
}
