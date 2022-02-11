// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"errors"
	"net"

	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
)

type networkConfigSuite struct {
	testing.IsolationSuite

	source *MockConfigSource

	ovsBridges            set.Strings
	defaultRouteGatewayIP net.IP
	defaultRouteDevice    string
	bridgePorts           map[string][]string
}

var _ = gc.Suite(&networkConfigSuite{})

func (s *networkConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.ovsBridges = set.NewStrings()
	s.defaultRouteGatewayIP = net.ParseIP("1.2.3.4")
	s.defaultRouteDevice = "eth0"
	s.bridgePorts = make(map[string][]string)
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigInterfacesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.source.EXPECT().Interfaces().Return(nil, errors.New("boom"))

	observedConfig, err := common.GetObservedNetworkConfig(s.source)
	c.Check(err, gc.ErrorMatches, "detecting network interfaces: boom")
	c.Check(observedConfig, gc.IsNil)
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigInterfaceAddressesError(c *gc.C) {
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

	observedConfig, err := common.GetObservedNetworkConfig(s.source)
	c.Check(err, gc.ErrorMatches, `detecting addresses for "eth0": bam`)
	c.Check(observedConfig, gc.IsNil)
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigNilAddressError(c *gc.C) {
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

	observedConfig, err := common.GetObservedNetworkConfig(s.source)
	c.Check(err, gc.ErrorMatches, `cannot parse nil address on interface "eth1"`)
	c.Check(observedConfig, gc.IsNil)
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigNoInterfaceAddresses(c *gc.C) {
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

	observedConfig, err := common.GetObservedNetworkConfig(s.source)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   2,
		MACAddress:    "aa:bb:cc:dd:ee:ff",
		MTU:           1500,
		InterfaceName: "eth1",
		InterfaceType: "ethernet",
		ConfigType:    "manual",
		NetworkOrigin: "machine",
	}})
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigDefaultGatewayWithAddresses(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	ip1, ipNet1, err := net.ParseCIDR("1.2.3.4/24")
	c.Assert(err, jc.ErrorIsNil)

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

	observedConfig, err := common.GetObservedNetworkConfig(s.source)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{
		{
			DeviceIndex:      2,
			MACAddress:       "aa:bb:cc:dd:ee:ff",
			MTU:              1500,
			InterfaceName:    "eth0",
			InterfaceType:    "ethernet",
			ConfigType:       "static",
			IsDefaultGateway: true,
			GatewayAddress:   "1.2.3.4",
			NetworkOrigin:    "machine",
			Addresses: []params.Address{
				{
					Value:      "1.2.3.4",
					CIDR:       "1.2.3.0/24",
					ConfigType: "static",
				}, {
					Value:       "559c:f8c5:812a:fa1f:21fe:5613:3f20:b081",
					ConfigType:  "static",
					IsSecondary: true,
				},
			},
		},
	})
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigForOVSDevice(c *gc.C) {
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

	observedConfig, err := common.GetObservedNetworkConfig(s.source)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:     2,
		MACAddress:      "aa:bb:cc:dd:ee:ff",
		MTU:             1500,
		InterfaceName:   "ovsbr0",
		InterfaceType:   "bridge",
		VirtualPortType: "openvswitch",
		ConfigType:      "manual",
		NetworkOrigin:   "machine",
	}})
}

func (s *networkConfigSuite) TestGetObservedNetworkConfigBridgePortsHaveParentSet(c *gc.C) {
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

	observedConfig, err := common.GetObservedNetworkConfig(s.source)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{
		{
			DeviceIndex:         2,
			MACAddress:          "aa:bb:cc:dd:ee:ff",
			MTU:                 1500,
			InterfaceName:       "eth1",
			InterfaceType:       "ethernet",
			ParentInterfaceName: "br-eth1",
			ConfigType:          "manual",
			NetworkOrigin:       "machine",
		},
		{
			DeviceIndex:   3,
			MACAddress:    "aa:bb:cc:dd:ee:ff",
			MTU:           1500,
			InterfaceName: "br-eth1",
			InterfaceType: "bridge",
			ConfigType:    "manual",
			NetworkOrigin: "machine",
		},
	})
}

func (s *networkConfigSuite) setupMocks(c *gc.C) *gomock.Controller {
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

func parseMAC(c *gc.C, val string) net.HardwareAddr {
	mac, err := net.ParseMAC(val)
	c.Assert(err, jc.ErrorIsNil)
	return mac
}
