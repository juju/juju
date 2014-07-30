// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
)

type NetworkInterfaceSuite struct {
	ConnSuite
	machine *state.Machine
	network *state.Network
	iface   *state.NetworkInterface
}

var _ = gc.Suite(&NetworkInterfaceSuite{})

func (s *NetworkInterfaceSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.network, err = s.State.AddNetwork(state.NetworkInfo{"net1", "net1", "0.1.2.3/24", 42})
	c.Assert(err, gc.IsNil)
	s.iface, err = s.machine.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:    "aa:bb:cc:dd:ee:ff",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     true,
	})
	c.Assert(err, gc.IsNil)
}

func (s *NetworkInterfaceSuite) TestGetterMethods(c *gc.C) {
	c.Assert(s.iface.Id(), gc.Not(gc.Equals), "")
	c.Assert(s.iface.MACAddress(), gc.Equals, "aa:bb:cc:dd:ee:ff")
	c.Assert(s.iface.InterfaceName(), gc.Equals, "eth0")
	c.Assert(s.iface.NetworkName(), gc.Equals, s.network.Name())
	c.Assert(s.iface.NetworkTag(), gc.Equals, s.network.Tag().String())
	c.Assert(s.iface.MachineId(), gc.Equals, s.machine.Id())
	c.Assert(s.iface.MachineTag(), gc.Equals, s.machine.Tag().String())
	c.Assert(s.iface.IsVirtual(), jc.IsTrue)
	c.Assert(s.iface.IsPhysical(), jc.IsFalse)
	c.Assert(s.iface.IsDisabled(), jc.IsFalse)
}

func (s *NetworkInterfaceSuite) TestSetAndIsDisabled(c *gc.C) {
	err := s.iface.SetDisabled(true)
	c.Assert(err, gc.IsNil)
	c.Assert(s.iface.IsDisabled(), jc.IsTrue)

	err = s.iface.SetDisabled(false)
	c.Assert(err, gc.IsNil)
	c.Assert(s.iface.IsDisabled(), jc.IsFalse)
}

func (s *NetworkInterfaceSuite) TestRefresh(c *gc.C) {
	ifaceCopy := *s.iface
	err := s.iface.SetDisabled(true)
	c.Assert(err, gc.IsNil)
	c.Assert(ifaceCopy.IsDisabled(), jc.IsFalse)
	err = ifaceCopy.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(ifaceCopy.IsDisabled(), jc.IsTrue)
}

func (s *NetworkInterfaceSuite) TestRemove(c *gc.C) {
	err := s.iface.Remove()
	c.Assert(err, gc.IsNil)
	err = s.iface.Refresh()
	errMatch := `network interface &state\.NetworkInterface\{.*\} not found`
	c.Check(err, gc.ErrorMatches, errMatch)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
