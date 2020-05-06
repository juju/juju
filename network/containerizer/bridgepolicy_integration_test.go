// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer_test

import (
	"fmt"
	"strconv"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/network/containerizer"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

// bridgePolicyStateSuite includes tests that are backed by Mongo.
type bridgePolicyStateSuite struct {
	statetesting.StateSuite

	machine          containerizer.Machine
	containerMachine containerizer.Container
}

var _ = gc.Suite(&bridgePolicyStateSuite{})

func (s *bridgePolicyStateSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	var err error
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.machine = containerizer.NewMachine(m)
}

func (s *bridgePolicyStateSuite) addContainerMachine(c *gc.C) {
	// Add a container machine with s.machine as its host.
	containerTemplate := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(containerTemplate, s.machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	s.containerMachine = containerizer.NewMachine(container)
}

func (s *bridgePolicyStateSuite) assertNoDevicesOnMachine(c *gc.C, machine containerizer.Container) {
	s.assertAllLinkLayerDevicesOnMachineMatchCount(c, machine, 0)
}

func (s *bridgePolicyStateSuite) assertAllLinkLayerDevicesOnMachineMatchCount(
	c *gc.C, machine containerizer.Container, expectedCount int,
) {
	results, err := machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, expectedCount)
}

func (s *bridgePolicyStateSuite) createSpaceAndSubnet(c *gc.C, spaceName, CIDR string) {
	space, err := s.State.AddSpace(spaceName, corenetwork.Id(spaceName), nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSubnet(corenetwork.SubnetInfo{
		CIDR:    CIDR,
		SpaceID: space.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
}

// setupTwoSpaces creates a 'somespace' and a 'dmz' space, each with a single
// registered subnet. 10.0.0.0/24 for 'somespace', and '10.10.0.0/24' for 'dmz'
func (s *bridgePolicyStateSuite) setupTwoSpaces(c *gc.C) {
	s.createSpaceAndSubnet(c, "somespace", "10.0.0.0/24")
	s.createSpaceAndSubnet(c, "dmz", "10.10.0.0/24")
}

func (s *bridgePolicyStateSuite) createNICWithIP(c *gc.C, machine containerizer.Machine, deviceName, cidrAddress string) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       deviceName,
			Type:       corenetwork.EthernetDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   deviceName,
			CIDRAddress:  cidrAddress,
			ConfigMethod: corenetwork.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bridgePolicyStateSuite) createBridgeWithIP(
	c *gc.C, machine containerizer.Machine, bridgeName, cidrAddress string,
) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       bridgeName,
			Type:       corenetwork.BridgeDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   bridgeName,
			CIDRAddress:  cidrAddress,
			ConfigMethod: corenetwork.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

// createNICAndBridgeWithIP creates a network interface and a bridge on the
// machine, and assigns the requested CIDRAddress to the bridge.
func (s *bridgePolicyStateSuite) createNICAndBridgeWithIP(
	c *gc.C, machine containerizer.Machine, deviceName, bridgeName, cidrAddress string,
) {
	s.createBridgeWithIP(c, machine, bridgeName, cidrAddress)
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       deviceName,
			Type:       corenetwork.EthernetDevice,
			ParentName: bridgeName,
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bridgePolicyStateSuite) setupMachineInTwoSpaces(c *gc.C) {
	s.setupTwoSpaces(c)
	s.createNICAndBridgeWithIP(c, s.machine, "ens33", "br-ens33", "10.0.0.20/24")
	s.createNICAndBridgeWithIP(c, s.machine, "ens0p10", "br-ens0p10", "10.10.0.20/24")
}

func (s *bridgePolicyStateSuite) createLoopbackNIC(c *gc.C, machine containerizer.Machine) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "lo",
			Type:       corenetwork.LoopbackDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   "lo",
			CIDRAddress:  "127.0.0.1/24",
			ConfigMethod: corenetwork.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

// createAllDefaultDevices creates the loopback, lxcbr0, lxdbr0, and virbr0 devices
func (s *bridgePolicyStateSuite) createAllDefaultDevices(c *gc.C, machine containerizer.Machine) {
	// loopback
	s.createLoopbackNIC(c, machine)
	// container.DefaultLxcBridge
	s.createBridgeWithIP(c, machine, "lxcbr0", "10.0.3.1/24")
	// container.DefaultLxdBridge
	s.createBridgeWithIP(c, machine, "lxdbr0", "10.0.4.1/24")
	// container.DefaultKvmBridge
	s.createBridgeWithIP(c, machine, "virbr0", "192.168.124.1/24")
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesCorrectlyPaired(c *gc.C) {
	// The device names chosen and the order are very explicit. We
	// need to ensure that we have a list that does not sort well
	// alphabetically. This is because SetParentLinkLayerDevices()
	// uses a natural sort ordering and we want to verify the
	// pairing between the container's NIC name and its parent in
	// the host machine during this unit test.

	devicesArgs := []state.LinkLayerDeviceArgs{
		{
			Name: "br-eth10",
			Type: corenetwork.BridgeDevice,
		},
		{
			Name: "br-eth1",
			Type: corenetwork.BridgeDevice,
		},
		{
			Name: "br-eth10-100",
			Type: corenetwork.BridgeDevice,
		},
		{
			Name: "br-eth2",
			Type: corenetwork.BridgeDevice,
		},
		{
			Name: "br-eth0",
			Type: corenetwork.BridgeDevice,
		},
		{
			Name: "br-eth4",
			Type: corenetwork.BridgeDevice,
		},
		{
			Name: "br-eth3",
			Type: corenetwork.BridgeDevice,
		},
	}
	// Put each of those bridges into a different subnet that is part
	// of the same space.
	space, err := s.State.AddSpace("somespace", "somespace", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	devAddresses := make([]state.LinkLayerDeviceAddress, len(devicesArgs))
	for i, devArg := range devicesArgs {
		subnet := i*10 + 1
		subnetCIDR := fmt.Sprintf("10.%d.0.0/24", subnet)
		_, err = s.State.AddSubnet(corenetwork.SubnetInfo{
			CIDR:    subnetCIDR,
			SpaceID: space.Id(),
		})
		c.Assert(err, jc.ErrorIsNil)
		devAddresses[i] = state.LinkLayerDeviceAddress{
			DeviceName:   devArg.Name,
			CIDRAddress:  fmt.Sprintf("10.%d.0.10/24", subnet),
			ConfigMethod: corenetwork.StaticAddress,
		}
	}

	expectedParents := []string{
		"br-eth0",
		"br-eth1",
		"br-eth2",
		"br-eth3",
		"br-eth4",
		"br-eth10",
		"br-eth10-100",
	}

	err = s.machine.SetParentLinkLayerDevicesBeforeTheirChildren(devicesArgs[:])
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetDevicesAddresses(devAddresses...)
	c.Assert(err, jc.ErrorIsNil)
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)
	err = s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)

	containerDevices, err := s.containerMachine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerDevices, gc.HasLen, len(devicesArgs))

	for i, containerDevice := range containerDevices {
		c.Check(containerDevice.Name(), gc.Matches, "eth"+strconv.Itoa(i))
		c.Check(containerDevice.Type(), gc.Equals, corenetwork.EthernetDevice)
		c.Check(containerDevice.MTU(), gc.Equals, uint(0)) // inherited from the parent device.
		c.Check(containerDevice.MACAddress(), gc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
		c.Check(containerDevice.IsUp(), jc.IsTrue)
		c.Check(containerDevice.IsAutoStart(), jc.IsTrue)
		c.Check(containerDevice.ParentName(), gc.Equals, fmt.Sprintf("m#0#d#%s", expectedParents[i]))
	}
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesConstraintsBindOnlyOne(c *gc.C) {
	s.setupMachineInTwoSpaces(c)
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"dmz"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)

	containerDevices, err := s.containerMachine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerDevices, gc.HasLen, 1)

	containerDevice := containerDevices[0]
	c.Check(containerDevice.Name(), gc.Matches, "eth0")
	c.Check(containerDevice.Type(), gc.Equals, corenetwork.EthernetDevice)
	c.Check(containerDevice.MTU(), gc.Equals, uint(0)) // inherited from the parent device.
	c.Check(containerDevice.MACAddress(), gc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
	c.Check(containerDevice.IsUp(), jc.IsTrue)
	c.Check(containerDevice.IsAutoStart(), jc.IsTrue)
	// br-ens0p10 on the host machine is in space dmz, while br-ens33 is in space somespace
	c.Check(containerDevice.ParentName(), gc.Equals, `m#0#d#br-ens0p10`)
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesHostOneSpace(c *gc.C) {
	s.setupTwoSpaces(c)
	// Is put into the 'somespace' space
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.0.0.20/24")
	// We change the machine to be in 'dmz' instead of 'somespace', but it is
	// still in a single space. Adding a container to a machine that is in a
	// single space puts that container into the same space.
	err := s.machine.RemoveAllAddresses()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName: "br-eth0",
			// In the DMZ subnet
			CIDRAddress:  "10.10.0.20/24",
			ConfigMethod: corenetwork.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)

	containerDevices, err := s.containerMachine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	// c.Assert(containerDevices, gc.HasLen, 0)
	// c.Skip("known failure, we don't handle containers no bindings and no constraints")
	// Ideally we would get a single container device that matches to
	// the 'somespace' space.
	c.Assert(containerDevices, gc.HasLen, 1)

	containerDevice := containerDevices[0]
	c.Check(containerDevice.Name(), gc.Matches, "eth0")
	c.Check(containerDevice.Type(), gc.Equals, corenetwork.EthernetDevice)
	c.Check(containerDevice.MTU(), gc.Equals, uint(0)) // inherited from the parent device.
	c.Check(containerDevice.MACAddress(), gc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
	c.Check(containerDevice.IsUp(), jc.IsTrue)
	c.Check(containerDevice.IsAutoStart(), jc.IsTrue)
	c.Check(containerDevice.ParentName(), gc.Equals, `m#0#d#br-eth0`)
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesDefaultSpace(c *gc.C) {
	// TODO(jam): 2016-12-28 Eventually we probably want to have a
	// model-config level default-space, but for now, 'default' should not be
	// special.
	// The host machine is in both 'default' and 'dmz', and the container is
	// not requested to be in any particular space. But because we have
	// access to the 'default' space, we go ahead and use that for the
	// container.
	s.setupMachineInTwoSpaces(c)
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, gc.ErrorMatches, "no obvious space for container.*")
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesNoValidSpace(c *gc.C) {
	// The host machine will be in 2 spaces, but neither one is 'somespace',
	// thus we are unable to find a valid space to put the container in.
	s.setupTwoSpaces(c)
	// Is put into the 'dmz' space
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.10.0.20/24")
	// Second bridge is in the 'db' space
	s.createSpaceAndSubnet(c, "db", "10.20.0.0/24")
	s.createNICAndBridgeWithIP(c, s.machine, "ens4", "br-ens4", "10.20.0.10/24")
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, gc.ErrorMatches, `no obvious space for container "0/lxd/0", host machine has spaces: .*`)

	s.assertNoDevicesOnMachine(c, s.containerMachine)
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesMismatchConstraints(c *gc.C) {
	// Machine is in 'somespace' but container wants to be in 'dmz'
	s.setupTwoSpaces(c)
	// Is put into the 'somespace' space
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.0.0.20/24")
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"dmz"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `unable to find host bridge for space(s) "dmz" for container "0/lxd/0"`)

	s.assertNoDevicesOnMachine(c, s.containerMachine)
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesMissingBridge(c *gc.C) {
	// Machine is in 'somespace' and 'dmz' but doesn't have a bridge for 'dmz'
	s.setupTwoSpaces(c)
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.0.0.20/24")
	s.createNICWithIP(c, s.machine, "ens5", "10.20.0.10/24")
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"dmz"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `unable to find host bridge for space(s) "dmz" for container "0/lxd/0"`)

	s.assertNoDevicesOnMachine(c, s.containerMachine)
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesNoDefaultNoConstraints(c *gc.C) {
	// The host machine will be in 2 spaces, but neither one is 'somespace',
	// thus we are unable to find a valid space to put the container in.
	s.setupTwoSpaces(c)
	// In 'dmz'
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.10.0.20/24")
	// Second bridge is in the 'db' space
	s.createSpaceAndSubnet(c, "db", "10.20.0.0/24")
	s.createNICAndBridgeWithIP(c, s.machine, "ens4", "br-ens4", "10.20.0.10/24")
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, gc.ErrorMatches, `no obvious space for container "0/lxd/0", host machine has spaces: .*`)

	containerDevices, err := s.containerMachine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerDevices, gc.HasLen, 0)
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesTwoDevicesOneBridged(c *gc.C) {
	// The host machine has 2 devices in one space, but only one is bridged.
	// We'll only use the one that is bridged, and not complain about the other.
	s.setupTwoSpaces(c)
	// In 'somespace'
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.20/24")
	s.createNICAndBridgeWithIP(c, s.machine, "eth1", "br-eth1", "10.0.0.21/24")
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace"},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoDevicesOnMachine(c, s.containerMachine)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)

	containerDevices, err := s.containerMachine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerDevices, gc.HasLen, 1)

	containerDevice := containerDevices[0]
	c.Check(containerDevice.Name(), gc.Matches, "eth0")
	c.Check(containerDevice.Type(), gc.Equals, corenetwork.EthernetDevice)
	c.Check(containerDevice.MTU(), gc.Equals, uint(0)) // inherited from the parent device.
	c.Check(containerDevice.MACAddress(), gc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
	c.Check(containerDevice.IsUp(), jc.IsTrue)
	c.Check(containerDevice.IsAutoStart(), jc.IsTrue)
	// br-eth1 is a valid bridge in the 'somespace' space
	c.Check(containerDevice.ParentName(), gc.Equals, `m#0#d#br-eth1`)
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesTwoBridgedSameSpace(c *gc.C) {
	// The host machine has 2 devices and both are bridged into the desired space
	// We'll use both
	s.setupTwoSpaces(c)
	// In 'somespace'
	s.createNICAndBridgeWithIP(c, s.machine, "ens33", "br-ens33", "10.0.0.20/24")
	s.createNICAndBridgeWithIP(c, s.machine, "ens44", "br-ens44", "10.0.0.21/24")
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace"},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoDevicesOnMachine(c, s.containerMachine)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)

	containerDevices, err := s.containerMachine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerDevices, gc.HasLen, 2)

	containerDevice := containerDevices[0]
	c.Check(containerDevice.Name(), gc.Matches, "eth0")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(containerDevice.ParentName(), gc.Equals, `m#0#d#br-ens33`)
	containerDevice = containerDevices[1]
	c.Check(containerDevice.Name(), gc.Matches, "eth1")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(containerDevice.ParentName(), gc.Equals, `m#0#d#br-ens44`)
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesTwoBridgesNotInSpaces(c *gc.C) {
	// The host machine has 2 network devices and 2 bridges, but none of them
	// are in a known space. The container also has no requested space.
	// In that case, we will use all of the unknown bridges for container
	// devices.
	s.setupTwoSpaces(c)
	s.createNICAndBridgeWithIP(c, s.machine, "ens3", "br-ens3", "172.12.1.10/24")
	s.createNICAndBridgeWithIP(c, s.machine, "ens4", "br-ens4", "192.168.3.4/24")
	s.createAllDefaultDevices(c, s.machine)
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)

	containerDevices, err := s.containerMachine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerDevices, gc.HasLen, 2)

	containerDevice := containerDevices[0]
	c.Check(containerDevice.Name(), gc.Matches, "eth0")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(containerDevice.ParentName(), gc.Equals, `m#0#d#br-ens3`)
	containerDevice = containerDevices[1]
	c.Check(containerDevice.Name(), gc.Matches, "eth1")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(containerDevice.ParentName(), gc.Equals, `m#0#d#br-ens4`)
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesNoLocal(c *gc.C) {
	// The host machine has 1 network device and only local bridges, but none of them
	// are in a known space. The container also has no requested space.
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "ens3", "172.12.1.10/24")
	s.createAllDefaultDevices(c, s.machine)
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `unable to find host bridge for space(s) "alpha" for container "0/lxd/0"`)
	s.assertNoDevicesOnMachine(c, s.containerMachine)
}

func (s *bridgePolicyStateSuite) TestPopulateContainerLinkLayerDevicesUseLocal(c *gc.C) {
	// The host machine has 1 network device and only local bridges, but none of them
	// are in a known space. The container also has no requested space.
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "ens3", "172.12.1.10/24")
	s.createAllDefaultDevices(c, s.machine)
	s.addContainerMachine(c)
	s.assertNoDevicesOnMachine(c, s.containerMachine)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "local"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)

	containerDevices, err := s.containerMachine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerDevices, gc.HasLen, 1)

	containerDevice := containerDevices[0]
	c.Check(containerDevice.Name(), gc.Matches, "eth0")
	c.Check(containerDevice.ParentName(), gc.Equals, `m#0#d#lxdbr0`)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerNoneMissing(c *gc.C) {
	s.setupTwoSpaces(c)
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.0.0.20/24")
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerDefaultUnbridged(c *gc.C) {
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.20/24")
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, gc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerNoHostDevices(c *gc.C) {
	s.setupTwoSpaces(c)
	s.createSpaceAndSubnet(c, "third", "10.20.0.0/24")
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.20/24")
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"dmz", "third"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	_, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `host machine "0" has no available device in space(s) "dmz", "third"`)
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerTwoSpacesOneMissing(c *gc.C) {
	s.setupTwoSpaces(c)
	// dmz
	s.createNICAndBridgeWithIP(c, s.machine, "eth1", "br-eth1", "10.10.0.20/24")
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace", "dmz"},
	})

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, gc.NotNil)
	// both somespace and dmz are needed, but somespace is missing
	c.Assert(err.Error(), gc.Equals, `host machine "0" has no available device in space(s) "somespace"`)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerNoSpaces(c *gc.C) {
	// There is a "somespace" and "dmz" space, and our machine has 2 network
	// interfaces, but is not part of any known space. In this circumstance,
	// we should try to bridge all of the unknown space devices, not just one
	// of them. This is are fallback mode when we don't understand the spaces of a machine.
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "ens3", "172.12.0.10/24")
	s.createNICWithIP(c, s.machine, "ens4", "192.168.0.10/24")
	s.createAllDefaultDevices(c, s.machine)
	s.addContainerMachine(c)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	// No defined spaces for the container, no *known* spaces for the host
	// machine. Triggers the fallback code to have us bridge all devices.
	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, gc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "ens3",
		BridgeName: "br-ens3",
	}, {
		DeviceName: "ens4",
		BridgeName: "br-ens4",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerContainerNetworkingMethodLocal(c *gc.C) {
	// There is a "somespace" and "dmz" space, our machine has 1 network
	// interface, but is not part of a known space. We have containerNetworkingMethod set to "local",
	// which means we should fall back to using 'lxdbr0' instead of
	// bridging the host device.
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "ens3", "172.12.0.10/24")
	s.createAllDefaultDevices(c, s.machine)
	s.addContainerMachine(c)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "local"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	// No defined spaces for the container, no *known* spaces for the host
	// machine. Triggers the fallback code to have us bridge all devices.
	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerContainerNetworkingMethodLocalDefinedHostSpace(c *gc.C) {
	// There is a "somespace" and "dmz" space, our machine has 1 network
	// interface, but is not part of a known space. We have containerNetworkingMethod set to "local",
	// which means we should fall back to using 'lxdbr0' instead of
	// bridging the host device.
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.20/24")
	s.createAllDefaultDevices(c, s.machine)
	s.addContainerMachine(c)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "local"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	// No defined spaces for the container, host has spaces but we have
	// ContainerNetworkingMethodLocal set so we should fall back to lxdbr0
	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{})
	c.Check(reconfigureDelay, gc.Equals, 0)

	err = bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)

	containerDevices, err := s.containerMachine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerDevices, gc.HasLen, 1)

	containerDevice := containerDevices[0]
	c.Check(containerDevice.Name(), gc.Matches, "eth0")
	c.Check(containerDevice.ParentName(), gc.Equals, `m#0#d#lxdbr0`)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerContainerNetworkingMethodLocalNoAddress(c *gc.C) {
	// We should only use 'lxdbr0' instead of bridging the host device.
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "ens3", "172.12.0.10/24")
	err := s.machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "lxdbr0",
			Type:       corenetwork.BridgeDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.addContainerMachine(c)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "local"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	// No defined spaces for the container, no *known* spaces for the host
	// machine. Triggers the fallback code to have us bridge all devices.
	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerUnknownWithConstraint(c *gc.C) {
	// If we have a host machine where we don't understand its spaces, but
	// the container requests a specific space, we won't use the unknown
	// ones.
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "ens3", "172.12.0.10/24")
	s.createNICWithIP(c, s.machine, "ens4", "192.168.0.10/24")
	s.createAllDefaultDevices(c, s.machine)
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals,
		`host machine "0" has no available device in space(s) "somespace"`)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerUnknownAndDefault(c *gc.C) {
	// The host machine has 2 devices, one is in a known 'somespace' space, the other is in an unknown space.
	// We will ignore the unknown space and just return the one in 'somespace',
	// cause that is the only declared space on the machine.
	s.setupTwoSpaces(c)
	// Default
	s.createNICWithIP(c, s.machine, "ens3", "10.0.0.10/24")
	s.createNICWithIP(c, s.machine, "ens4", "192.168.0.10/24")
	s.createAllDefaultDevices(c, s.machine)
	s.addContainerMachine(c)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	// We don't need a container constraint, as the host machine is in a single space.
	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, gc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "ens3",
		BridgeName: "br-ens3",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerOneOfTwoBridged(c *gc.C) {
	// With two host devices that could be bridged, we will only ask for the
	// first one to be bridged.
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "ens3", "10.0.0.20/24")
	s.createNICWithIP(c, s.machine, "ens4", "10.0.0.21/24")
	s.createNICWithIP(c, s.machine, "ens5", "10.0.0.22/24")
	s.createNICWithIP(c, s.machine, "ens6", "10.0.0.23/24")
	s.createNICWithIP(c, s.machine, "ens7", "10.0.0.24/24")
	s.createNICWithIP(c, s.machine, "ens8", "10.0.0.25/24")
	s.createNICWithIP(c, s.machine, "ens3.1", "10.0.0.26/24")
	s.createNICWithIP(c, s.machine, "ens3:1", "10.0.0.27/24")
	s.createNICWithIP(c, s.machine, "ens2.1", "10.0.0.28/24")
	s.createNICWithIP(c, s.machine, "ens2.2", "10.0.0.29/24")
	s.createNICWithIP(c, s.machine, "ens20", "10.0.0.30/24")
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	// Only the first device (by sort order) should be selected
	c.Check(missing, gc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "ens2.1",
		BridgeName: "br-ens2-1",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerTwoHostDevicesOneBridged(c *gc.C) {
	// With two host devices that could be bridged, we will only ask for the
	// first one to be bridged.
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "ens3", "10.0.0.20/24")
	s.createNICAndBridgeWithIP(c, s.machine, "ens4", "br-ens4", "10.0.0.21/24") // TODO: different subnet?
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerNoConstraintsDefaultNotSpecial(c *gc.C) {
	// TODO(jam): 2016-12-28 Eventually we probably want to have a
	// model-config level default-space, but for now, 'somespace' should not be
	// special.
	s.setupTwoSpaces(c)
	// Default
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.20/24")
	// DMZ
	s.createNICWithIP(c, s.machine, "eth1", "10.10.0.20/24")
	s.addContainerMachine(c)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, gc.ErrorMatches, "no obvious space for container.*")
	c.Assert(missing, gc.IsNil)
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerTwoSpacesOneBridged(c *gc.C) {
	s.setupTwoSpaces(c)
	// somespace
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.20/24")
	// DMZ
	s.createNICAndBridgeWithIP(c, s.machine, "eth1", "br-eth1", "10.10.0.20/24")
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace", "dmz"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	// both somespace and dmz are needed, but somespace needs to be bridged
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerMultipleSpacesNoneBridged(c *gc.C) {
	s.setupTwoSpaces(c)
	// somespace
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.20/24")
	// DMZ
	s.createNICWithIP(c, s.machine, "eth1", "10.10.0.20/24")
	// abba
	s.createSpaceAndSubnet(c, "abba", "172.12.10.0/24")
	s.createNICWithIP(c, s.machine, "eth0.1", "172.12.10.3/24")
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace", "dmz", "abba"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	// both default and dmz are needed, but default needs to be bridged
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}, {
		DeviceName: "eth0.1",
		BridgeName: "br-eth0-1",
	}, {
		DeviceName: "eth1",
		BridgeName: "br-eth1",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerBondedNICs(c *gc.C) {
	s.setupTwoSpaces(c)
	// somespace
	// We call it 'zbond' so it sorts late instead of first
	err := s.machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "zbond0",
			Type:       corenetwork.BondDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "eth0",
			Type:       corenetwork.EthernetDevice,
			ParentName: "zbond0",
			IsUp:       true,
		},
		state.LinkLayerDeviceArgs{
			Name:       "eth1",
			Type:       corenetwork.EthernetDevice,
			ParentName: "zbond0",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   "zbond0",
			CIDRAddress:  "10.0.0.10/24",
			ConfigMethod: corenetwork.StaticAddress,
		},
		// TODO(jam): 2016-12-20 These devices *shouldn't* have IP addresses
		// when they are in a bond, however eventually we should detect what
		// space a device is in by something other than just IP address, and
		// we want to test that we don't try to bond these devices.
		// So for now we give them IP addresses so they show up in the space
		state.LinkLayerDeviceAddress{
			DeviceName:   "eth0",
			CIDRAddress:  "10.0.0.11/24",
			ConfigMethod: corenetwork.StaticAddress,
		},
		state.LinkLayerDeviceAddress{
			DeviceName:   "eth1",
			CIDRAddress:  "10.0.0.12/24",
			ConfigMethod: corenetwork.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.addContainerMachine(c)
	err = s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	// both somespace and dmz are needed, but somespace needs to be bridged
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "zbond0",
		BridgeName: "br-zbond0",
	}})
	// We are creating a bridge on a bond, so we use a non-zero delay
	c.Check(reconfigureDelay, gc.Equals, 13)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerVLAN(c *gc.C) {
	s.setupTwoSpaces(c)
	// We create an eth0 that has an address, and then an eth0.100 which is
	// VLAN tagged on top of that ethernet device.
	// "eth0" is in "somespace", "eth0.100" is in "dmz"
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.10/24")
	err := s.machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "eth0.100",
			Type:       corenetwork.VLAN8021QDevice,
			ParentName: "eth0",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	// In dmz
	err = s.machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   "eth0.100",
			CIDRAddress:  "10.10.0.11/24",
			ConfigMethod: corenetwork.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// We create a container in both spaces, and we should see that it wants
	// to bridge both devices.
	s.addContainerMachine(c)
	err = s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace", "dmz"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}, {
		DeviceName: "eth0.100",
		BridgeName: "br-eth0-100",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerVLANOnBond(c *gc.C) {
	s.setupTwoSpaces(c)
	// We have eth0 and eth1 that don't have IP addresses, that are in a
	// bond, which then has a VLAN on top of that bond. The VLAN should still
	// be a valid target for bridging
	err := s.machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "bond0",
			Type:       corenetwork.BondDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetLinkLayerDevices(
		[]state.LinkLayerDeviceArgs{{
			Name:       "eth0",
			Type:       corenetwork.EthernetDevice,
			ParentName: "bond0",
			IsUp:       true,
		}, {
			Name:       "eth1",
			Type:       corenetwork.EthernetDevice,
			ParentName: "bond0",
			IsUp:       true,
		}, {
			Name:       "bond0.100",
			Type:       corenetwork.VLAN8021QDevice,
			ParentName: "bond0",
			IsUp:       true,
		}}...,
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   "bond0",
			CIDRAddress:  "10.0.0.20/24", // somespace
			ConfigMethod: corenetwork.StaticAddress,
		},
		state.LinkLayerDeviceAddress{
			DeviceName:   "bond0.100",
			CIDRAddress:  "10.10.0.20/24", // dmz
			ConfigMethod: corenetwork.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// We create a container in both spaces, and we should see that it wants
	// to bridge both devices.
	s.addContainerMachine(c)
	err = s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace", "dmz"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "provider"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "bond0",
		BridgeName: "br-bond0",
	}, {
		DeviceName: "bond0.100",
		BridgeName: "br-bond0-100",
	}})
	c.Check(reconfigureDelay, gc.Equals, 13)
}

func (s *bridgePolicyStateSuite) TestFindMissingBridgesForContainerNetworkingMethodFAN(c *gc.C) {
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.20/24")
	s.addContainerMachine(c)
	err := s.containerMachine.SetConstraints(constraints.Value{
		Spaces: &[]string{"somespace"},
	})
	c.Assert(err, jc.ErrorIsNil)

	bridgePolicy, err := containerizer.NewBridgePolicy(cfg(c, 13, "fan"), s.State)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = bridgePolicy.FindMissingBridgesForContainer(s.machine, s.containerMachine)
	c.Assert(err, gc.ErrorMatches, `host machine "0" has no available FAN devices in space\(s\) "somespace"`)
}

var bridgeNames = map[string]string{
	"eno0":            "br-eno0",
	"enovlan.123":     "br-enovlan-123",
	"twelvechars0":    "br-twelvechars0",
	"thirteenchars":   "b-thirteenchars",
	"enfourteenchar":  "b-fourteenchar",
	"enfifteenchars0": "b-fifteenchars0",
	"fourteenchars1":  "b-5590a4-chars1",
	"fifteenchars.12": "b-38b496-ars-12",
	"zeros0526193032": "b-000000-193032",
	"enx00e07cc81e1d": "b-x00e07cc81e1d",
}

func (s *bridgePolicyStateSuite) TestBridgeNameForDevice(c *gc.C) {
	for deviceName, bridgeName := range bridgeNames {
		generatedBridgeName := containerizer.BridgeNameForDevice(deviceName)
		c.Assert(generatedBridgeName, gc.Equals, bridgeName)
	}
}

type configGetter struct {
	c *gc.C

	reconfDelay int
	netMethod   string
}

func (g configGetter) Config() *config.Config {
	cfg, err := config.New(false, map[string]interface{}{
		config.NameKey:                    "some-model",
		config.TypeKey:                    "some-cloud",
		config.UUIDKey:                    utils.MustNewUUID().String(),
		config.NetBondReconfigureDelayKey: g.reconfDelay,
		config.ContainerNetworkingMethod:  g.netMethod,
		config.FanConfig:                  "172.16.0.0/16=253.0.0.0/8",
	})
	g.c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func cfg(c *gc.C, reconfDelay int, netMethod string) configGetter {
	return configGetter{
		c:           c,
		reconfDelay: reconfDelay,
		netMethod:   netMethod,
	}
}
