// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type InterfacesSuite struct {
	ConnSuite

	spaces  []*state.Space
	subnets []*state.Subnet

	machine *state.Machine
	ifaces  []*state.NetworkInterface
}

var _ = gc.Suite(&InterfacesSuite{})

func (s *InterfacesSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	var err error

	s.spaces = make([]*state.Space, 2)
	s.spaces[0], err = s.State.AddSpace("apps", network.Id("space-1"), nil, true)
	c.Check(err, jc.ErrorIsNil)

	s.spaces[1], err = s.State.AddSpace("admin", "", nil, false)
	c.Check(err, jc.ErrorIsNil)

	s.subnets = make([]*state.Subnet, 3)
	s.subnets[0], err = s.State.AddSubnet(state.SubnetInfo{
		ProviderId:       network.Id("subnet-1"),
		CIDR:             "0.1.2.0/24",
		AvailabilityZone: "zone1",
		SpaceName:        "apps",
	})
	c.Check(err, jc.ErrorIsNil)

	s.subnets[1], err = s.State.AddSubnet(state.SubnetInfo{
		CIDR:      "0.2.3.0/24",
		VLANTag:   42,
		SpaceName: "admin",
	})
	c.Check(err, jc.ErrorIsNil)

	s.subnets[2], err = s.State.AddSubnet(state.SubnetInfo{
		CIDR:              "0.3.0.0/16",
		AllocatableIPLow:  "0.3.0.100",
		AllocatableIPHigh: "0.3.100.100",
	})
	c.Check(err, jc.ErrorIsNil)

	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Check(err, jc.ErrorIsNil)

	s.ifaces = make([]*state.NetworkInterface, 2)
	s.ifaces[0], err = s.machine.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:  "aa:bb:cc:dd:ee:ff",
		DeviceName:  "eth0",
		DeviceIndex: 0,
		ProviderID:  "nic-0",
		SubnetID:    "0.1.2.0/24",
	})
	c.Check(err, jc.ErrorIsNil)

	s.ifaces[1], err = s.machine.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:  "aa:bb:cc:dd:ee:ff",
		DeviceName:  "eth0.42",
		DeviceIndex: 0,
		ProviderID:  "nic-42",
		SubnetID:    "0.2.3.0/24",
		IsVirtual:   true,
	})
	c.Check(err, jc.ErrorIsNil)
}

func (s *InterfacesSuite) TestGetterMethods(c *gc.C) {
	c.Check(s.ifaces[0].ID(), gc.Equals, "m#0#p#nic-0#d#eth0#a#aa:bb:cc:dd:ee:ff#s#0.1.2.0/24")
	c.Check(s.ifaces[0].String(), gc.Equals, `network interface "eth0" on machine "0"`)
	uuid, err := s.ifaces[0].UUID()
	c.Check(err, jc.ErrorIsNil)
	c.Check(uuid.String(), jc.Satisfies, utils.IsValidUUIDString)
	c.Check(s.ifaces[0].MACAddress(), gc.Equals, "aa:bb:cc:dd:ee:ff")
	c.Check(s.ifaces[0].DeviceName(), gc.Equals, "eth0")
	c.Check(s.ifaces[0].DeviceIndex(), gc.Equals, 0)
	c.Check(s.ifaces[0].SubnetID(), gc.Equals, s.subnets[0].CIDR())
	c.Check(s.ifaces[0].SubnetTag(), gc.Equals, s.subnets[0].Tag())
	c.Check(s.ifaces[0].MachineID(), gc.Equals, s.machine.Id())
	c.Check(s.ifaces[0].MachineTag(), gc.Equals, s.machine.Tag())
	c.Check(s.ifaces[0].IsVirtual(), jc.IsFalse)
	c.Check(s.ifaces[0].IsPhysical(), jc.IsTrue)
	c.Check(s.ifaces[0].ProviderID(), gc.Equals, network.Id("nic-0"))

	c.Check(s.ifaces[1].IsVirtual(), jc.IsTrue)
	c.Check(s.ifaces[1].IsPhysical(), jc.IsFalse)
	c.Check(s.ifaces[1].MACAddress(), gc.Equals, s.ifaces[0].MACAddress())
	c.Check(s.ifaces[1].SubnetID(), gc.Equals, s.subnets[1].CIDR())
}

func (s *InterfacesSuite) TestRefresh(c *gc.C) {
	ifaceCopy := *s.ifaces[0]
	err := s.ifaces[0].Remove()
	c.Check(err, jc.ErrorIsNil)

	c.Check(ifaceCopy.String(), gc.Equals, s.ifaces[0].String())
	err = ifaceCopy.Refresh()
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *InterfacesSuite) TestRemove(c *gc.C) {
	err := s.ifaces[1].Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.ifaces[1].Refresh()
	errMatch := `network interface "eth0.42" on machine "0" not found`
	c.Check(err, gc.ErrorMatches, errMatch)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
