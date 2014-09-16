// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/networker"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
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

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
	networker  *networker.NetworkerAPI
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

// Create a machine to use.
func (s *networkerSuite) setUpMachine(c *gc.C) {
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
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
	c.Assert(err, gc.IsNil)
}

// Create and provision a container and a nested container.
func (s *networkerSuite) setUpContainers(c *gc.C) {
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	var err error
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
	hwChars := instance.MustParseHardware("arch=i386", "mem=4G")
	err = s.container.SetInstanceInfo("i-container", "fake_nonce", &hwChars, s.networks[:2],
		s.containerIfaces)
	c.Assert(err, gc.IsNil)

	s.nestedContainer, err = s.State.AddMachineInsideMachine(template, s.container.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)
	s.nestedContainerIfaces = []state.NetworkInterfaceInfo{{
		MACAddress:    "aa:bb:cc:dd:ee:d0",
		InterfaceName: "eth0",
		NetworkName:   "net1",
	}}
	err = s.nestedContainer.SetInstanceInfo("i-too", "fake_nonce", &hwChars, s.networks[:1],
		s.nestedContainerIfaces)
	c.Assert(err, gc.IsNil)
}

func (s *networkerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.setUpNetworks(c)
	s.setUpMachine(c)
	s.setUpContainers(c)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming we logged in as a machine agent.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine.Tag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()

	// Create a networker API for the machine.
	var err error
	s.networker, err = networker.NewNetworkerAPI(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, gc.IsNil)
}

func (s *networkerSuite) TestNetworkerNonMachineAgent(c *gc.C) {
	// Fails with not a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("ubuntu/1")
	aNetworker, err := networker.NewNetworkerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(aNetworker, gc.IsNil)
}

func (s *networkerSuite) TestMachineNetworkInfoPermissions(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "service-bar"},
		{Tag: "foo-42"},
		{Tag: "unit-mysql-0"},
		{Tag: "service-mysql"},
		{Tag: "user-foo"},
		{Tag: "machine-1"},
		{Tag: "machine-0-lxc-42"},
	}}
	results, err := s.networker.MachineNetworkInfo(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.MachineNetworkInfoResults{
		Results: []params.MachineNetworkInfoResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError("machine 0/lxc/42")},
		},
	})
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

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-0-lxc-0"},
		{Tag: "machine-0-lxc-0-lxc-0"},
	}}
	results, err := s.networker.MachineNetworkInfo(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.MachineNetworkInfoResults{
		Results: []params.MachineNetworkInfoResult{
			{Error: nil, Info: expectedMachineInfo},
			{Error: nil, Info: expectedContainerInfo},
			{Error: nil, Info: expectedNestedContainerInfo},
		},
	})
}

func (s *networkerSuite) TestWatchInterfacesPermissions(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "service-bar"},
		{Tag: "foo-42"},
		{Tag: "unit-mysql-0"},
		{Tag: "service-mysql"},
		{Tag: "user-foo"},
		{Tag: "machine-1"},
		{Tag: "machine-0-lxc-42"},
	}}
	results, err := s.networker.WatchInterfaces(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError("machine 0/lxc/42")},
		},
	})
}

func (s *networkerSuite) TestWatchInterfaces(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-0-lxc-0"},
		{Tag: "machine-0-lxc-0-lxc-0"},
	}}
	result, err := s.networker.WatchInterfaces(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{NotifyWatcherId: "2"},
			{NotifyWatcherId: "3"},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 3)
	for _, watcherId := range []string{"1", "2", "3"} {
		resource := s.resources.Get(watcherId)
		defer statetesting.AssertStop(c, resource)

		// Check that the WatchInterfaces has consumed the initial event ("returned" in
		// the Watch call)
		wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
		wc.AssertNoChange()
	}
}
