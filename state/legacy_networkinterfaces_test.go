// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type NetworkInterfaceSuite struct {
	ConnSuite
	machine     *state.Machine
	net1        *state.Network
	vlan42      *state.Network
	ifaceNet1   *state.NetworkInterface
	ifaceVLAN42 *state.NetworkInterface
}

var _ = gc.Suite(&NetworkInterfaceSuite{})

func (s *NetworkInterfaceSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.net1, err = s.State.AddNetwork(state.NetworkInfo{"net1", "net1", "0.1.2.3/24", 0})
	c.Assert(err, jc.ErrorIsNil)
	s.vlan42, err = s.State.AddNetwork(state.NetworkInfo{"vlan42", "vlan42", "0.2.3.4/24", 42})
	c.Assert(err, jc.ErrorIsNil)
	s.ifaceNet1, err = s.machine.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:    "aa:bb:cc:dd:ee:ff",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     false,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.ifaceVLAN42, err = s.machine.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:    "aa:bb:cc:dd:ee:ff",
		InterfaceName: "eth0.42",
		NetworkName:   "vlan42",
		IsVirtual:     true,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NetworkInterfaceSuite) TestGetterMethods(c *gc.C) {
	c.Assert(s.ifaceNet1.Id(), gc.Not(gc.Equals), "")
	c.Assert(s.ifaceNet1.MACAddress(), gc.Equals, "aa:bb:cc:dd:ee:ff")
	c.Assert(s.ifaceNet1.InterfaceName(), gc.Equals, "eth0")
	c.Assert(s.ifaceNet1.RawInterfaceName(), gc.Equals, "eth0")
	c.Assert(s.ifaceNet1.NetworkName(), gc.Equals, s.net1.Name())
	c.Assert(s.ifaceNet1.NetworkTag(), gc.Equals, s.net1.Tag())
	c.Assert(s.ifaceNet1.MachineId(), gc.Equals, s.machine.Id())
	c.Assert(s.ifaceNet1.MachineTag(), gc.Equals, s.machine.Tag())
	c.Assert(s.ifaceNet1.IsVirtual(), jc.IsFalse)
	c.Assert(s.ifaceNet1.IsPhysical(), jc.IsTrue)
	c.Assert(s.ifaceNet1.IsDisabled(), jc.IsFalse)

	c.Assert(s.ifaceVLAN42.NetworkName(), gc.Equals, s.vlan42.Name())
	c.Assert(s.ifaceVLAN42.MACAddress(), gc.Equals, "aa:bb:cc:dd:ee:ff")
	c.Assert(s.ifaceVLAN42.InterfaceName(), gc.Equals, "eth0.42")
	c.Assert(s.ifaceVLAN42.RawInterfaceName(), gc.Equals, "eth0")
	c.Assert(s.ifaceVLAN42.IsVirtual(), jc.IsTrue)
	c.Assert(s.ifaceVLAN42.IsPhysical(), jc.IsFalse)
	c.Assert(s.ifaceVLAN42.IsDisabled(), jc.IsFalse)
}

func (s *NetworkInterfaceSuite) TestEnableDisableAndIsDisabled(c *gc.C) {
	c.Assert(s.ifaceNet1.IsDisabled(), jc.IsFalse)
	c.Assert(s.ifaceVLAN42.IsDisabled(), jc.IsFalse)

	err := s.ifaceNet1.Disable()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.ifaceNet1.IsDisabled(), jc.IsTrue)
	// Test eth0.42 is disabled as well when eth0 is.
	err = s.ifaceVLAN42.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.ifaceVLAN42.IsDisabled(), jc.IsTrue)

	err = s.ifaceNet1.Enable()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.ifaceNet1.IsDisabled(), jc.IsFalse)
	// eth0.42 is not automatically enabled when eth0 is.
	err = s.ifaceVLAN42.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.ifaceVLAN42.IsDisabled(), jc.IsTrue)
}

func (s *NetworkInterfaceSuite) TestRefresh(c *gc.C) {
	ifaceCopy := *s.ifaceNet1
	err := s.ifaceNet1.Disable()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaceCopy.IsDisabled(), jc.IsFalse)
	err = ifaceCopy.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaceCopy.IsDisabled(), jc.IsTrue)
}

func (s *NetworkInterfaceSuite) TestRemove(c *gc.C) {
	err := s.ifaceNet1.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.ifaceNet1.Refresh()
	errMatch := `network interface &state\.NetworkInterface\{.*\} not found`
	c.Check(err, gc.ErrorMatches, errMatch)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
