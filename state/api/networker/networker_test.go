// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/network"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/networker"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
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

	expectedMachineInfo   []network.Info
	expectedContainerInfo []network.Info
}

var _ = gc.Suite(&networkerSuite{})

func (s *networkerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create several networks.
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

	// Create a machine to login and use.
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = s.machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
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
	}}
	err = s.machine.SetInstanceInfo("i-am", "fake_nonce", &hwChars, s.networks, s.machineIfaces)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAsMachine(c, s.machine.Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)

	// Create and provision a container.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	s.container, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)
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
	err = s.container.SetInstanceInfo("i-container", "fake_nonce", &hwChars, s.networks[:2],
		s.containerIfaces)
	c.Assert(err, gc.IsNil)

	// Create and provision a nested container.
	s.nestedContainer, err = s.State.AddMachineInsideMachine(template, s.container.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)
	s.nestedContainerIfaces = []state.NetworkInterfaceInfo{{
		MACAddress:    "aa:bb:cc:dd:ee:d0",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     false,
	}}
	err = s.nestedContainer.SetInstanceInfo("i-nested", "fake_nonce", &hwChars, s.networks[:1],
		s.nestedContainerIfaces)
	c.Assert(err, gc.IsNil)

	// Create the networker API facade.
	s.networker = s.st.Networker()
	c.Assert(s.networker, gc.NotNil)

	// Expected results of MachineNetworkInfo for a machine and a containers
	s.expectedMachineInfo = []network.Info{{
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		CIDR:          "0.1.2.0/24",
		NetworkName:   "net1",
		ProviderId:    "net1",
		VLANTag:       0,
		InterfaceName: "eth0",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		CIDR:          "0.1.2.0/24",
		NetworkName:   "net1",
		ProviderId:    "net1",
		VLANTag:       0,
		InterfaceName: "eth1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		CIDR:          "0.2.2.0/24",
		NetworkName:   "vlan42",
		ProviderId:    "vlan42",
		VLANTag:       42,
		InterfaceName: "eth1",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		CIDR:          "0.3.2.0/24",
		NetworkName:   "vlan69",
		ProviderId:    "vlan69",
		VLANTag:       69,
		InterfaceName: "eth0",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f2",
		CIDR:          "0.5.2.0/24",
		NetworkName:   "net2",
		ProviderId:    "net2",
		VLANTag:       0,
		InterfaceName: "eth2",
		IsVirtual:     false,
	}}
	s.expectedContainerInfo = []network.Info{{
		MACAddress:    "aa:bb:cc:dd:ee:e0",
		CIDR:          "0.1.2.0/24",
		NetworkName:   "net1",
		ProviderId:    "net1",
		VLANTag:       0,
		InterfaceName: "eth0",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:e1",
		CIDR:          "0.1.2.0/24",
		NetworkName:   "net1",
		ProviderId:    "net1",
		VLANTag:       0,
		InterfaceName: "eth1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:e1",
		CIDR:          "0.2.2.0/24",
		NetworkName:   "vlan42",
		ProviderId:    "vlan42",
		VLANTag:       42,
		InterfaceName: "eth1",
		IsVirtual:     true,
	}}
}

func (s *networkerSuite) TestMachineNetworkInfoPermissionDenied(c *gc.C) {
	tags := []string{"foo-42", "unit-mysql-0", "service-mysql", "user-foo", "machine-1", s.nestedContainer.Tag()}
	for _, tag := range tags {
		info, err := s.networker.MachineNetworkInfo(tag)
		c.Assert(err, gc.ErrorMatches, "permission denied")
		c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
		c.Assert(info, gc.IsNil)
	}
}

func (s *networkerSuite) TestMachineNetworkInfo(c *gc.C) {
	results, err := s.networker.MachineNetworkInfo("machine-0")
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, s.expectedMachineInfo)

	results, err = s.networker.MachineNetworkInfo("machine-0-lxc-0")
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, s.expectedContainerInfo)
}
