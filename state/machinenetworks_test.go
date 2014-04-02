// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
)

type MachineNetworkSuite struct {
	ConnSuite
	machine *state.Machine
	network *state.MachineNetwork
	vlan    *state.MachineNetwork
}

var _ = gc.Suite(&MachineNetworkSuite{})

func (s *MachineNetworkSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.network, err = s.State.AddMachineNetwork("net1", "0.1.2.3/24", 0)
	c.Assert(err, gc.IsNil)
	s.vlan, err = s.State.AddMachineNetwork("vlan", "0.1.2.3/30", 42)
	c.Assert(err, gc.IsNil)
}

func (s *MachineNetworkSuite) TestRemove(c *gc.C) {
	// Add an interface and verify we can't remove the network.
	iface, err := s.machine.AddNetworkInterface("aa:bb:cc:dd:ee:ff", "eth0", "net1")
	c.Assert(err, gc.IsNil)
	err := s.network.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove machine network "net1" with existing interfaces`)
	// Now remove it and retry.
	err = iface.Remove()
	c.Assert(err, gc.IsNil)
	err = s.network.Remove()
	c.Assert(err, gc.IsNil)
	err = s.network.Remove()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *MachineNetworkSuite) TestGetterMethods(c *gc.C) {
	c.Assert(s.network.Name(), gc.Equals, "net1")
	c.Assert(s.network.CIDR(), gc.Equals, "0.1.2.3/24")
	c.Assert(s.network.VLANTag(), gc.Equals, 0)
	c.Assert(s.vlan.VLANTag(), gc.Equals, 42)
	c.Assert(s.network.IsVLANg(), jc.IsFalse)
	c.Assert(s.vlan.IsVLAN(), gc.IsFalse)
}

func (s *MachineNetworkSuite) TestInterfaces(c *gc.C) {
	ifaces, err := s.network.Interfaces()
	c.Assert(err, gc.IsNil)
	c.Assert(ifaces, gc.HasLen, 0)

	iface0, err := s.machine.AddNetworkInterface("aa:bb:cc:dd:ee:f0", "eth0", "net1")
	c.Assert(err, gc.IsNil)
	iface1, err := s.machine.AddNetworkInterface("aa:bb:cc:dd:ee:f1", "eth1", "net1")
	c.Assert(err, gc.IsNil)

	ifaces, err = s.network.Interfaces()
	c.Assert(err, gc.IsNil)
	c.Assert(ifaces, jc.SameContents, []*state.NetworkInterfaces{iface0, iface1})
}
