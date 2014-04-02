// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
)

type NetworkInterfaceSuite struct {
	ConnSuite
	machine *state.Machine
	network *state.MachineNetwork
	iface   *state.NetworkInterface
}

var _ = gc.Suite(&NetworkInterfaceSuite{})

func (s *NetworkInterfaceSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.network, err = s.State.AddMachineNetwork("net1", "0.1.2.3/24", 42)
	c.Assert(err, gc.IsNil)
	s.iface, err = s.machine.AddNetworkInterface("aa:bb:cc:dd:ee:ff", "eth0", "net1")
	c.Assert(err, gc.IsNil)
}

func (s *NetworkInterfaceSuite) TestRemove(c *gc.C) {
	err := s.iface.Remove()
	c.Assert(err, gc.IsNil)
	err = s.iface.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	err = s.iface.Remove()
	c.Assert(err, gc.IsNil)
}

func (s *NetworkInterfaceSuite) TestRemoveAbort(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.iface.Remove(), gc.IsNil)
	}).Check()
	err := s.iface.Remove()
	c.Assert(err, gc.IsNil)
}

func (s *NetworkInterfaceSuite) TestGetterMethods(c *gc.C) {
	c.Assert(s.iface.MACAddress(), gc.Equals, "aa:bb:cc:dd:ee:ff")
	c.Assert(s.iface.Name(), gc.Equals, "eth0")
	c.Assert(s.iface.NetworkName(), gc.Equals, s.network.Name())
	c.Assert(s.iface.MachineId(), gc.Equals, s.machine.Id())
}
