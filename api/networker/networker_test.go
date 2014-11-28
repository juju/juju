// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/networker"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type networkerSuite struct {
	testing.JujuConnSuite

	networks []state.NetworkInfo

	machine         *state.Machine
	container       *state.Machine
	nestedContainer *state.Machine

	machineIfaces         []state.NetworkInterfaceInfo
	containerIfaces       []state.NetworkInterfaceInfo
	nestedContainerIfaces []state.NetworkInterfaceInfo

	st        *api.State
	networker *networker.State
}

var _ = gc.Suite(&networkerSuite{})

// Create several networks.
func (s *networkerSuite) setUpNetworks(c *gc.C) {
	s.networks = []state.NetworkInfo{{
		Name:       "net1",
		ProviderId: "net1",
		CIDR:       "0.1.2.0/24",
		VLANTag:    0,
	}, {
		Name:       "vlan42",
		ProviderId: "vlan42",
		CIDR:       "0.2.2.0/24",
		VLANTag:    42,
	}, {
		Name:       "vlan69",
		ProviderId: "vlan69",
		CIDR:       "0.3.2.0/24",
		VLANTag:    69,
	}, {
		Name:       "vlan123",
		ProviderId: "vlan123",
		CIDR:       "0.4.2.0/24",
		VLANTag:    123,
	}, {
		Name:       "net2",
		ProviderId: "net2",
		CIDR:       "0.5.2.0/24",
		VLANTag:    0,
	}}
}

// Create a machine and login to it.
func (s *networkerSuite) setUpMachine(c *gc.C) {
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	hwChars := instance.MustParseHardware("cpu-cores=123", "mem=4G")
	s.machineIfaces = []state.NetworkInterfaceInfo{{
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		InterfaceName: "eth1",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		InterfaceName: "eth1.42",
		NetworkName:   "vlan42",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		InterfaceName: "eth0.69",
		NetworkName:   "vlan69",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f2",
		InterfaceName: "eth2",
		NetworkName:   "net2",
		IsVirtual:     false,
		Disabled:      true,
	}}
	err = s.machine.SetInstanceInfo("i-am", "fake_nonce", &hwChars, s.networks, s.machineIfaces)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAsMachine(c, s.machine.Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)
}

// Create and provision a container and a nested container.
func (s *networkerSuite) setUpContainers(c *gc.C) {
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	var err error
	s.container, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	s.containerIfaces = []state.NetworkInterfaceInfo{{
		MACAddress:    "aa:bb:cc:dd:ee:e0",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:e1",
		InterfaceName: "eth1",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:e1",
		InterfaceName: "eth1.42",
		NetworkName:   "vlan42",
		IsVirtual:     true,
	}}
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err = s.container.SetInstanceInfo("i-container", "fake_nonce", &hwChars, s.networks[:2],
		s.containerIfaces)
	c.Assert(err, jc.ErrorIsNil)

	s.nestedContainer, err = s.State.AddMachineInsideMachine(template, s.container.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	s.nestedContainerIfaces = []state.NetworkInterfaceInfo{{
		MACAddress:    "aa:bb:cc:dd:ee:d0",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     false,
	}}
	err = s.nestedContainer.SetInstanceInfo("i-too", "fake_nonce", &hwChars, s.networks[:1],
		s.nestedContainerIfaces)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.setUpNetworks(c)
	s.setUpMachine(c)
	s.setUpContainers(c)

	// Create the networker API facade.
	s.networker = s.st.Networker()
	c.Assert(s.networker, gc.NotNil)
}

func (s *networkerSuite) TestMachineNetworkInfoPermissionDenied(c *gc.C) {
	info, err := s.networker.MachineNetworkInfo(names.NewMachineTag("1"))
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(info, gc.IsNil)
}

func (s *networkerSuite) TestMachineNetworkInfo(c *gc.C) {
	// Expected results of MachineNetworkInfo for a machine and containers
	expectedMachineInfo := []network.Info{{
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		CIDR:          "0.1.2.0/24",
		NetworkName:   "net1",
		ProviderId:    "net1",
		VLANTag:       0,
		InterfaceName: "eth0",
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		CIDR:          "0.1.2.0/24",
		NetworkName:   "net1",
		ProviderId:    "net1",
		VLANTag:       0,
		InterfaceName: "eth1",
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		CIDR:          "0.2.2.0/24",
		NetworkName:   "vlan42",
		ProviderId:    "vlan42",
		VLANTag:       42,
		InterfaceName: "eth1",
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		CIDR:          "0.3.2.0/24",
		NetworkName:   "vlan69",
		ProviderId:    "vlan69",
		VLANTag:       69,
		InterfaceName: "eth0",
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f2",
		CIDR:          "0.5.2.0/24",
		NetworkName:   "net2",
		ProviderId:    "net2",
		VLANTag:       0,
		InterfaceName: "eth2",
		Disabled:      true,
	}}
	expectedContainerInfo := []network.Info{{
		MACAddress:    "aa:bb:cc:dd:ee:e0",
		CIDR:          "0.1.2.0/24",
		NetworkName:   "net1",
		ProviderId:    "net1",
		VLANTag:       0,
		InterfaceName: "eth0",
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:e1",
		CIDR:          "0.1.2.0/24",
		NetworkName:   "net1",
		ProviderId:    "net1",
		VLANTag:       0,
		InterfaceName: "eth1",
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:e1",
		CIDR:          "0.2.2.0/24",
		NetworkName:   "vlan42",
		ProviderId:    "vlan42",
		VLANTag:       42,
		InterfaceName: "eth1",
	}}
	expectedNestedContainerInfo := []network.Info{{
		MACAddress:    "aa:bb:cc:dd:ee:d0",
		CIDR:          "0.1.2.0/24",
		NetworkName:   "net1",
		ProviderId:    "net1",
		VLANTag:       0,
		InterfaceName: "eth0",
	}}

	results, err := s.networker.MachineNetworkInfo(names.NewMachineTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, expectedMachineInfo)

	results, err = s.networker.MachineNetworkInfo(names.NewMachineTag("0/lxc/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, expectedContainerInfo)

	results, err = s.networker.MachineNetworkInfo(names.NewMachineTag("0/lxc/0/lxc/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, expectedNestedContainerInfo)
}

func (s *networkerSuite) TestWatchInterfacesPermissionDenied(c *gc.C) {
	w, err := s.networker.WatchInterfaces(names.NewMachineTag("1"))
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(w, gc.IsNil)
}

func (s *networkerSuite) TestWatchInterfaces(c *gc.C) {
	// Read dynamically generated document Ids.
	ifaces, err := s.machine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces, gc.HasLen, 5)

	// Start network interface watcher.
	w, err := s.networker.WatchInterfaces(names.NewMachineTag("0"))
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)
	wc.AssertOneChange()

	// Disable the first interface.
	err = ifaces[0].Disable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Disable the first interface again, should not report.
	err = ifaces[0].Disable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Enable the first interface.
	err = ifaces[0].Enable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Enable the first interface again, should not report.
	err = ifaces[0].Enable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove the network interface.
	err = ifaces[0].Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Add the new interface.
	_, err = s.machine.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:    "aa:bb:cc:dd:ee:f3",
		InterfaceName: "eth3",
		NetworkName:   "net2",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Add the new interface on the container, should not report.
	_, err = s.container.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:    "aa:bb:cc:dd:ee:e3",
		InterfaceName: "eth3",
		NetworkName:   "net2",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Read dynamically generated document Ids.
	containerIfaces, err := s.container.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerIfaces, gc.HasLen, 4)

	// Disable the first interface on the second machine, should not report.
	err = containerIfaces[0].Disable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove the network interface on the second machine, should not report.
	err = containerIfaces[0].Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher; check Changes chan closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
