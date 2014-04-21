// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
)

type NetworkSuite struct {
	ConnSuite
	machine *state.Machine
	network *state.Network
	vlan    *state.Network
}

var _ = gc.Suite(&NetworkSuite{})

func (s *NetworkSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.network, err = s.State.AddNetwork(state.NetworkInfo{"net1", "net1", "0.1.2.3/24", 0})
	c.Assert(err, gc.IsNil)
	s.vlan, err = s.State.AddNetwork(state.NetworkInfo{"vlan", "vlan", "0.1.2.3/30", 42})
	c.Assert(err, gc.IsNil)
}

func (s *NetworkSuite) TestGetterMethods(c *gc.C) {
	c.Assert(s.network.Name(), gc.Equals, "net1")
	c.Assert(string(s.network.ProviderId()), gc.Equals, "net1")
	c.Assert(s.network.Tag(), gc.Equals, "network-net1")
	c.Assert(s.network.CIDR(), gc.Equals, "0.1.2.3/24")
	c.Assert(s.network.VLANTag(), gc.Equals, 0)
	c.Assert(s.vlan.VLANTag(), gc.Equals, 42)
	c.Assert(s.network.IsVLAN(), jc.IsFalse)
	c.Assert(s.vlan.IsVLAN(), jc.IsTrue)
}

func (s *NetworkSuite) TestInterfaces(c *gc.C) {
	ifaces, err := s.network.Interfaces()
	c.Assert(err, gc.IsNil)
	c.Assert(ifaces, gc.HasLen, 0)

	iface0, err := s.machine.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     false,
	})
	c.Assert(err, gc.IsNil)
	iface1, err := s.machine.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		InterfaceName: "eth1",
		NetworkName:   "net1",
		IsVirtual:     false,
	})
	c.Assert(err, gc.IsNil)

	ifaces, err = s.network.Interfaces()
	c.Assert(err, gc.IsNil)
	c.Assert(ifaces, gc.HasLen, 2)
	c.Assert(ifaces, jc.DeepEquals, []*state.NetworkInterface{iface0, iface1})
}
