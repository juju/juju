// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"net"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type networkConfigSuite struct {
	testhelpers.IsolationSuite

	source *MockConfigSource

	ovsBridges            set.Strings
	defaultRouteGatewayIP net.IP
	defaultRouteDevice    string
	bridgePorts           map[string][]string
}

func TestNetworkConfigSuite(t *testing.T) {
	tc.Run(t, &networkConfigSuite{})
}

func (s *networkConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.ovsBridges = set.NewStrings()
	s.defaultRouteGatewayIP = net.ParseIP("1.2.3.4")
	s.defaultRouteDevice = "eth0"
	s.bridgePorts = make(map[string][]string)
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigInterfacesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.source.EXPECT().Interfaces().Return(nil, errors.New("boom"))

	observedConfig, err := network.GetObservedNetworkConfig(s.source)
	c.Check(err, tc.ErrorMatches, "detecting network interfaces: boom")
	c.Check(observedConfig, tc.IsNil)
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigInterfaceAddressesError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	nic := NewMockConfigSourceNIC(ctrl)
	exp := nic.EXPECT()
	exp.Name().Return("eth0").MinTimes(1)
	exp.Type().Return(network.EthernetDevice).MinTimes(1)
	exp.IsUp().Return(true)
	exp.Index().Return(2)
	exp.HardwareAddr().Return(net.HardwareAddr{})
	exp.MTU().Return(1500)
	exp.Addresses().Return(nil, errors.New("bam"))

	s.source.EXPECT().Interfaces().Return([]network.ConfigSourceNIC{nic}, nil)

	observedConfig, err := network.GetObservedNetworkConfig(s.source)
	c.Check(err, tc.ErrorMatches, `detecting addresses for "eth0": bam`)
	c.Check(observedConfig, tc.IsNil)
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigNilAddressError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	nic := NewMockConfigSourceNIC(ctrl)
	exp := nic.EXPECT()
	exp.Name().Return("eth1").MinTimes(1)
	exp.Type().Return(network.EthernetDevice).MinTimes(1)
	exp.IsUp().Return(true)
	exp.Index().Return(2)
	exp.HardwareAddr().Return(parseMAC(c, "aa:bb:cc:dd:ee:ff"))
	exp.MTU().Return(1500)
	exp.Addresses().Return([]network.ConfigSourceAddr{nil}, nil)

	s.source.EXPECT().Interfaces().Return([]network.ConfigSourceNIC{nic}, nil)

	observedConfig, err := network.GetObservedNetworkConfig(s.source)
	c.Check(err, tc.ErrorMatches, `cannot parse nil address on interface "eth1"`)
	c.Check(observedConfig, tc.IsNil)
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigNoInterfaceAddresses(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	nic := NewMockConfigSourceNIC(ctrl)
	exp := nic.EXPECT()

	// Note that eth1 is not the default gateway.
	exp.Name().Return("eth1").MinTimes(1)
	exp.Type().Return(network.EthernetDevice).MinTimes(1)
	exp.IsUp().Return(true)
	exp.Index().Return(2)
	exp.HardwareAddr().Return(parseMAC(c, "aa:bb:cc:dd:ee:ff"))
	exp.MTU().Return(1500)
	exp.Addresses().Return(nil, nil)

	s.source.EXPECT().Interfaces().Return([]network.ConfigSourceNIC{nic}, nil)

	observedConfig, err := network.GetObservedNetworkConfig(s.source)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(observedConfig, tc.DeepEquals, network.InterfaceInfos{{
		DeviceIndex:   2,
		MACAddress:    "aa:bb:cc:dd:ee:ff",
		MTU:           1500,
		InterfaceName: "eth1",
		InterfaceType: "ethernet",
		ConfigType:    "manual",
		Origin:        network.OriginMachine,
	}})
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigDefaultGatewayWithAddresses(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	ip1, ipNet1, err := net.ParseCIDR("1.2.3.4/24")
	c.Assert(err, tc.ErrorIsNil)

	addr1 := NewMockConfigSourceAddr(ctrl)
	addr1.EXPECT().IP().Return(ip1)
	addr1.EXPECT().IPNet().Return(ipNet1)
	addr1.EXPECT().IsSecondary().Return(false)

	// Not the address not in CIDR form will result in config without a CIDR.
	addr2 := NewMockConfigSourceAddr(ctrl)
	addr2.EXPECT().IP().Return(net.ParseIP("559c:f8c5:812a:fa1f:21fe:5613:3f20:b081"))
	addr2.EXPECT().IPNet().Return(nil)
	addr2.EXPECT().IsSecondary().Return(true)

	nic := NewMockConfigSourceNIC(ctrl)
	exp := nic.EXPECT()

	// eth0 matches the device returned as the default gateway.
	exp.Name().Return("eth0").MinTimes(1)
	exp.Type().Return(network.EthernetDevice).MinTimes(1)
	exp.IsUp().Return(true)
	exp.Index().Return(2)
	exp.HardwareAddr().Return(parseMAC(c, "aa:bb:cc:dd:ee:ff"))
	exp.MTU().Return(1500)
	exp.Addresses().Return([]network.ConfigSourceAddr{addr1, addr2}, nil)

	s.source.EXPECT().Interfaces().Return([]network.ConfigSourceNIC{nic}, nil)

	observedConfig, err := network.GetObservedNetworkConfig(s.source)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(observedConfig, tc.DeepEquals, network.InterfaceInfos{
		{
			DeviceIndex:      2,
			MACAddress:       "aa:bb:cc:dd:ee:ff",
			MTU:              1500,
			InterfaceName:    "eth0",
			InterfaceType:    "ethernet",
			ConfigType:       "static",
			IsDefaultGateway: true,
			GatewayAddress:   network.NewMachineAddress("1.2.3.4").AsProviderAddress(),
			Origin:           network.OriginMachine,
			Addresses: []network.ProviderAddress{
				network.NewMachineAddress(
					"1.2.3.4",
					network.WithCIDR("1.2.3.0/24"),
					network.WithConfigType("static"),
					network.WithScope("public"),
				).AsProviderAddress(),
				network.NewMachineAddress(
					"559c:f8c5:812a:fa1f:21fe:5613:3f20:b081",
					network.WithConfigType("static"),
					network.WithSecondary(true),
					network.WithScope("public"),
				).AsProviderAddress(),
			},
		},
	})
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigForOVSDevice(c *tc.C) {
	s.ovsBridges.Add("ovsbr0")

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	nic := NewMockConfigSourceNIC(ctrl)
	exp := nic.EXPECT()

	exp.Name().Return("ovsbr0").MinTimes(1)
	exp.Type().Return(network.BridgeDevice).MinTimes(1)
	exp.IsUp().Return(true)
	exp.Index().Return(2)
	exp.HardwareAddr().Return(parseMAC(c, "aa:bb:cc:dd:ee:ff"))
	exp.MTU().Return(1500)
	exp.Addresses().Return(nil, nil)

	s.source.EXPECT().Interfaces().Return([]network.ConfigSourceNIC{nic}, nil)

	observedConfig, err := network.GetObservedNetworkConfig(s.source)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(observedConfig, tc.DeepEquals, network.InterfaceInfos{{
		DeviceIndex:     2,
		MACAddress:      "aa:bb:cc:dd:ee:ff",
		MTU:             1500,
		InterfaceName:   "ovsbr0",
		InterfaceType:   "bridge",
		VirtualPortType: "openvswitch",
		ConfigType:      "manual",
		Origin:          network.OriginMachine,
	}})
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigBridgePortsHaveParentSet(c *tc.C) {
	s.bridgePorts["br-eth1"] = []string{"eth1"}

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	nic1 := NewMockConfigSourceNIC(ctrl)
	exp1 := nic1.EXPECT()

	exp1.Name().Return("eth1").MinTimes(1)
	exp1.Type().Return(network.EthernetDevice).MinTimes(1)
	exp1.IsUp().Return(true)
	exp1.Index().Return(2)
	exp1.HardwareAddr().Return(parseMAC(c, "aa:bb:cc:dd:ee:ff"))
	exp1.MTU().Return(1500)
	exp1.Addresses().Return(nil, nil)

	nic2 := NewMockConfigSourceNIC(ctrl)
	exp2 := nic2.EXPECT()

	exp2.Name().Return("br-eth1").MinTimes(1)
	exp2.Type().Return(network.BridgeDevice).MinTimes(1)
	exp2.IsUp().Return(true)
	exp2.Index().Return(3)
	exp2.HardwareAddr().Return(parseMAC(c, "aa:bb:cc:dd:ee:ff"))
	exp2.MTU().Return(1500)
	exp2.Addresses().Return(nil, nil)

	s.source.EXPECT().Interfaces().Return([]network.ConfigSourceNIC{nic1, nic2}, nil)

	observedConfig, err := network.GetObservedNetworkConfig(s.source)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(observedConfig, tc.DeepEquals, network.InterfaceInfos{
		{
			DeviceIndex:         2,
			MACAddress:          "aa:bb:cc:dd:ee:ff",
			MTU:                 1500,
			InterfaceName:       "eth1",
			InterfaceType:       "ethernet",
			ParentInterfaceName: "br-eth1",
			ConfigType:          "manual",
			Origin:              network.OriginMachine,
		},
		{
			DeviceIndex:   3,
			MACAddress:    "aa:bb:cc:dd:ee:ff",
			MTU:           1500,
			InterfaceName: "br-eth1",
			InterfaceType: "bridge",
			ConfigType:    "manual",
			Origin:        network.OriginMachine,
		},
	})
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigHostLocalBridgeAddressesAreMachineLocal(c *tc.C) {
	s.defaultRouteGatewayIP = net.ParseIP("10.227.223.1")
	s.defaultRouteDevice = "eth0"

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	eth0 := s.mockNIC(c, ctrl, "eth0", network.EthernetDevice, 2,
		s.mockAddr(c, ctrl, "10.227.223.162/24"),
	)
	lxdbr0 := s.mockNIC(c, ctrl, network.DefaultLXDBridge, network.BridgeDevice, 3,
		s.mockAddr(c, ctrl, "10.19.175.1/24"),
		s.mockAddr(c, ctrl, "fd42:cc3f:e5c5:338a::1/64"),
	)
	docker0 := s.mockNIC(c, ctrl, network.DefaultDockerBridge, network.BridgeDevice, 4,
		s.mockAddr(c, ctrl, "172.17.0.1/16"),
	)

	s.source.EXPECT().Interfaces().Return([]network.ConfigSourceNIC{eth0, lxdbr0, docker0}, nil)

	observedConfig, err := network.GetObservedNetworkConfig(s.source)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(observedConfig, tc.DeepEquals, network.InterfaceInfos{
		{
			DeviceIndex:      2,
			MACAddress:       "aa:bb:cc:dd:ee:ff",
			MTU:              1500,
			InterfaceName:    "eth0",
			InterfaceType:    network.EthernetDevice,
			ConfigType:       network.ConfigStatic,
			IsDefaultGateway: true,
			GatewayAddress:   network.NewMachineAddress("10.227.223.1").AsProviderAddress(),
			Origin:           network.OriginMachine,
			Addresses: []network.ProviderAddress{
				network.NewMachineAddress(
					"10.227.223.162",
					network.WithCIDR("10.227.223.0/24"),
					network.WithConfigType(network.ConfigStatic),
					network.WithScope(network.ScopeCloudLocal),
				).AsProviderAddress(),
			},
		},
		{
			DeviceIndex:   3,
			MACAddress:    "aa:bb:cc:dd:ee:ff",
			MTU:           1500,
			InterfaceName: network.DefaultLXDBridge,
			InterfaceType: network.BridgeDevice,
			ConfigType:    network.ConfigStatic,
			Origin:        network.OriginMachine,
			Addresses: []network.ProviderAddress{
				network.NewMachineAddress(
					"10.19.175.1",
					network.WithCIDR("10.19.175.0/24"),
					network.WithConfigType(network.ConfigStatic),
					network.WithScope(network.ScopeMachineLocal),
				).AsProviderAddress(),
				network.NewMachineAddress(
					"fd42:cc3f:e5c5:338a::1",
					network.WithCIDR("fd42:cc3f:e5c5:338a::/64"),
					network.WithConfigType(network.ConfigStatic),
					network.WithScope(network.ScopeMachineLocal),
				).AsProviderAddress(),
			},
		},
		{
			DeviceIndex:   4,
			MACAddress:    "aa:bb:cc:dd:ee:ff",
			MTU:           1500,
			InterfaceName: network.DefaultDockerBridge,
			InterfaceType: network.BridgeDevice,
			ConfigType:    network.ConfigStatic,
			Origin:        network.OriginMachine,
			Addresses: []network.ProviderAddress{
				network.NewMachineAddress(
					"172.17.0.1",
					network.WithCIDR("172.17.0.0/16"),
					network.WithConfigType(network.ConfigStatic),
					network.WithScope(network.ScopeMachineLocal),
				).AsProviderAddress(),
			},
		},
	})
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigHostLocalBridgeDefaultGatewayKeepsDerivedScope(c *tc.C) {
	s.defaultRouteGatewayIP = net.ParseIP("10.100.1.10")
	s.defaultRouteDevice = network.DefaultLXDBridge

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lxdbr0 := s.mockNIC(c, ctrl, network.DefaultLXDBridge, network.BridgeDevice, 2,
		s.mockAddr(c, ctrl, "10.100.1.1/24"),
	)

	s.source.EXPECT().Interfaces().Return([]network.ConfigSourceNIC{lxdbr0}, nil)

	observedConfig, err := network.GetObservedNetworkConfig(s.source)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(observedConfig, tc.DeepEquals, network.InterfaceInfos{{
		DeviceIndex:      2,
		MACAddress:       "aa:bb:cc:dd:ee:ff",
		MTU:              1500,
		InterfaceName:    network.DefaultLXDBridge,
		InterfaceType:    network.BridgeDevice,
		ConfigType:       network.ConfigStatic,
		IsDefaultGateway: true,
		GatewayAddress:   network.NewMachineAddress("10.100.1.10").AsProviderAddress(),
		Origin:           network.OriginMachine,
		Addresses: []network.ProviderAddress{
			network.NewMachineAddress(
				"10.100.1.1",
				network.WithCIDR("10.100.1.0/24"),
				network.WithConfigType(network.ConfigStatic),
				network.WithScope(network.ScopeCloudLocal),
			).AsProviderAddress(),
		},
	}})
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigHostLocalBridgeOnlyCloudLocalReclassified(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	lxdbr0 := s.mockNIC(c, ctrl, network.DefaultLXDBridge, network.BridgeDevice, 2,
		s.mockAddr(c, ctrl, "10.19.175.1/24"),
		s.mockAddr(c, ctrl, "240.0.0.1/8"),
		s.mockAddr(c, ctrl, "203.0.113.1/29"),
		s.mockAddr(c, ctrl, "169.254.10.1/16"),
	)

	s.source.EXPECT().Interfaces().Return([]network.ConfigSourceNIC{lxdbr0}, nil)

	observedConfig, err := network.GetObservedNetworkConfig(s.source)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(observedConfig, tc.DeepEquals, network.InterfaceInfos{{
		DeviceIndex:   2,
		MACAddress:    "aa:bb:cc:dd:ee:ff",
		MTU:           1500,
		InterfaceName: network.DefaultLXDBridge,
		InterfaceType: network.BridgeDevice,
		ConfigType:    network.ConfigStatic,
		Origin:        network.OriginMachine,
		Addresses: []network.ProviderAddress{
			network.NewMachineAddress(
				"10.19.175.1",
				network.WithCIDR("10.19.175.0/24"),
				network.WithConfigType(network.ConfigStatic),
				network.WithScope(network.ScopeMachineLocal),
			).AsProviderAddress(),
			network.NewMachineAddress(
				"240.0.0.1",
				network.WithCIDR("240.0.0.0/8"),
				network.WithConfigType(network.ConfigStatic),
				network.WithScope(network.ScopeFanLocal),
			).AsProviderAddress(),
			network.NewMachineAddress(
				"203.0.113.1",
				network.WithCIDR("203.0.113.0/29"),
				network.WithConfigType(network.ConfigStatic),
				network.WithScope(network.ScopePublic),
			).AsProviderAddress(),
			network.NewMachineAddress(
				"169.254.10.1",
				network.WithCIDR("169.254.0.0/16"),
				network.WithConfigType(network.ConfigStatic),
				network.WithScope(network.ScopeLinkLocal),
			).AsProviderAddress(),
		},
	}})
}

func (s *networkConfigSuite) mockNIC(
	c *tc.C, ctrl *gomock.Controller,
	name string, devType network.LinkLayerDeviceType, index int,
	addrs ...network.ConfigSourceAddr,
) *MockConfigSourceNIC {
	nic := NewMockConfigSourceNIC(ctrl)
	exp := nic.EXPECT()
	exp.Name().Return(name).MinTimes(1)
	exp.Type().Return(devType).MinTimes(1)
	exp.IsUp().Return(true)
	exp.Index().Return(index)
	exp.HardwareAddr().Return(parseMAC(c, "aa:bb:cc:dd:ee:ff"))
	exp.MTU().Return(1500)
	exp.Addresses().Return(addrs, nil)
	return nic
}

func (s *networkConfigSuite) mockAddr(c *tc.C, ctrl *gomock.Controller, cidr string) *MockConfigSourceAddr {
	ip, ipNet, err := net.ParseCIDR(cidr)
	c.Assert(err, tc.ErrorIsNil)

	addr := NewMockConfigSourceAddr(ctrl)
	addr.EXPECT().IP().Return(ip)
	addr.EXPECT().IPNet().Return(ipNet)
	addr.EXPECT().IsSecondary().Return(false)
	return addr
}

func (s *networkConfigSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.source = NewMockConfigSource(ctrl)
	exp := s.source.EXPECT()
	exp.OvsManagedBridges().Return(s.ovsBridges, nil).MaxTimes(1)
	exp.DefaultRoute().Return(s.defaultRouteGatewayIP, s.defaultRouteDevice, nil).MaxTimes(1)

	if len(s.bridgePorts) == 0 {
		exp.GetBridgePorts(gomock.Any()).Return(nil).AnyTimes()
	} else {
		for brName, ports := range s.bridgePorts {
			exp.GetBridgePorts(brName).Return(ports)
		}
	}

	return ctrl
}

func parseMAC(c *tc.C, val string) net.HardwareAddr {
	mac, err := net.ParseMAC(val)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}
