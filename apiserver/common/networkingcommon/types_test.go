// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type TypesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&TypesSuite{})

func (s *TypesSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

var observedNetworkConfigs = []params.NetworkConfig{{
	DeviceIndex:   1,
	InterfaceName: "lo",
	InterfaceType: string(network.LoopbackDevice),
	MACAddress:    "",
	CIDR:          "127.0.0.0/8",
	Address:       "127.0.0.1",
	MTU:           65536,
	ConfigType:    string(network.ConfigLoopback),
	NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:   10,
	InterfaceName: "br-eth0",
	InterfaceType: string(network.BridgeDevice),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	Addresses: []params.Address{
		{
			Value:      "10.20.19.100",
			CIDR:       "10.20.19.0/24",
			ConfigType: "dhcp",
		},
		{
			Value:       "10.20.19.123",
			CIDR:        "10.20.19.0/24",
			ConfigType:  "static",
			IsSecondary: true,
		},
	},
	MTU:           1500,
	ConfigType:    string(network.ConfigDHCP),
	NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:   12,
	InterfaceName: "br-eth0.100",
	InterfaceType: string(network.BridgeDevice),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.100.19.0/24",
	Address:       "10.100.19.100",
	MTU:           1500,
	NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:   14,
	InterfaceName: "br-eth0.250",
	InterfaceType: string(network.BridgeDevice),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.250.19.0/24",
	Address:       "10.250.19.100",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
	NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:   16,
	InterfaceName: "br-eth0.50",
	InterfaceType: string(network.BridgeDevice),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.50.19.0/24",
	Address:       "10.50.19.100",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
	NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:         2,
	InterfaceName:       "eth0",
	ParentInterfaceName: "br-eth0",
	InterfaceType:       string(network.EthernetDevice),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	NetworkOrigin:       params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:         13,
	InterfaceName:       "eth0.100",
	ParentInterfaceName: "br-eth0.100",
	InterfaceType:       string(network.VLAN8021QDevice),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	NetworkOrigin:       params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:         15,
	InterfaceName:       "eth0.250",
	ParentInterfaceName: "br-eth0.250",
	InterfaceType:       string(network.VLAN8021QDevice),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	NetworkOrigin:       params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:         17,
	InterfaceName:       "eth0.50",
	ParentInterfaceName: "br-eth0.50",
	InterfaceType:       string(network.VLAN8021QDevice),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	NetworkOrigin:       params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:   11,
	InterfaceName: "br-eth1",
	InterfaceType: string(network.BridgeDevice),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.20.19.0/24",
	Address:       "10.20.19.105",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
	NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:   18,
	InterfaceName: "br-eth1.11",
	InterfaceType: string(network.BridgeDevice),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.11.19.0/24",
	Address:       "10.11.19.101",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
	NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:   20,
	InterfaceName: "br-eth1.12",
	InterfaceType: string(network.BridgeDevice),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	Addresses: []params.Address{{
		Value:      "10.12.19.101",
		CIDR:       "10.12.19.0/24",
		ConfigType: string(network.ConfigStatic),
	}},
	MTU:           1500,
	NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:   22,
	InterfaceName: "br-eth1.13",
	InterfaceType: string(network.BridgeDevice),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.13.19.0/24",
	Address:       "10.13.19.101",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
	NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:         3,
	InterfaceName:       "eth1",
	ParentInterfaceName: "br-eth1",
	InterfaceType:       string(network.EthernetDevice),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	NetworkOrigin:       params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:         19,
	InterfaceName:       "eth1.11",
	ParentInterfaceName: "br-eth1.11",
	InterfaceType:       string(network.VLAN8021QDevice),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	NetworkOrigin:       params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:         21,
	InterfaceName:       "eth1.12",
	ParentInterfaceName: "br-eth1.12",
	InterfaceType:       string(network.VLAN8021QDevice),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	NetworkOrigin:       params.NetworkOrigin(network.OriginMachine),
}, {
	DeviceIndex:         23,
	InterfaceName:       "eth1.13",
	ParentInterfaceName: "br-eth1.13",
	InterfaceType:       string(network.VLAN8021QDevice),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	NetworkOrigin:       params.NetworkOrigin(network.OriginMachine),
}}

var expectedLinkLayerDeviceArgsWithFinalNetworkConfig = []state.LinkLayerDeviceArgs{{
	Name:        "lo",
	MTU:         65536,
	Type:        network.LoopbackDevice,
	IsAutoStart: true,
	IsUp:        true,
}, {
	Name:        "br-eth0",
	MTU:         1500,
	Type:        network.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
}, {
	Name:        "br-eth0.100",
	MTU:         1500,
	Type:        network.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "",
}, {
	Name:        "br-eth0.250",
	MTU:         1500,
	Type:        network.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "",
}, {
	Name:        "br-eth0.50",
	MTU:         1500,
	Type:        network.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "",
}, {
	Name:        "eth0",
	MTU:         1500,
	Type:        network.EthernetDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0",
}, {
	Name:        "eth0.100",
	MTU:         1500,
	Type:        network.VLAN8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0.100",
}, {
	Name:        "eth0.250",
	MTU:         1500,
	Type:        network.VLAN8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0.250",
}, {
	Name:        "eth0.50",
	MTU:         1500,
	Type:        network.VLAN8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0.50",
}, {
	Name:        "br-eth1",
	MTU:         1500,
	Type:        network.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
}, {
	Name:        "br-eth1.11",
	MTU:         1500,
	Type:        network.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "",
}, {
	Name:        "br-eth1.12",
	MTU:         1500,
	Type:        network.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "",
}, {
	Name:        "br-eth1.13",
	MTU:         1500,
	Type:        network.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "",
}, {
	Name:        "eth1",
	MTU:         1500,
	Type:        network.EthernetDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1",
}, {
	Name:        "eth1.11",
	MTU:         1500,
	Type:        network.VLAN8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1.11",
}, {
	Name:        "eth1.12",
	MTU:         1500,
	Type:        network.VLAN8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1.12",
}, {
	Name:        "eth1.13",
	MTU:         1500,
	Type:        network.VLAN8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1.13",
}}

var expectedLinkLayerDeviceAddressesWithFinalNetworkConfig = []state.LinkLayerDeviceAddress{{
	DeviceName:   "lo",
	ConfigMethod: network.ConfigLoopback,
	CIDRAddress:  "127.0.0.1/8",
	Origin:       network.OriginMachine,
}, {
	DeviceName:   "br-eth0",
	ConfigMethod: network.ConfigDHCP,
	CIDRAddress:  "10.20.19.100/24",
	Origin:       network.OriginMachine,
}, {
	DeviceName:   "br-eth0",
	ConfigMethod: network.ConfigStatic,
	CIDRAddress:  "10.20.19.123/24",
	Origin:       network.OriginMachine,
	IsSecondary:  true,
}, {
	DeviceName:   "br-eth0.100",
	ConfigMethod: network.ConfigStatic,
	CIDRAddress:  "10.100.19.100/24",
	Origin:       network.OriginMachine,
}, {
	DeviceName:   "br-eth0.250",
	ConfigMethod: network.ConfigStatic,
	CIDRAddress:  "10.250.19.100/24",
	Origin:       network.OriginMachine,
}, {
	DeviceName:   "br-eth0.50",
	ConfigMethod: network.ConfigStatic,
	CIDRAddress:  "10.50.19.100/24",
	Origin:       network.OriginMachine,
}, {
	DeviceName:   "br-eth1",
	ConfigMethod: network.ConfigStatic,
	CIDRAddress:  "10.20.19.105/24",
	Origin:       network.OriginMachine,
}, {
	DeviceName:   "br-eth1.11",
	ConfigMethod: network.ConfigStatic,
	CIDRAddress:  "10.11.19.101/24",
	Origin:       network.OriginMachine,
}, {
	DeviceName:   "br-eth1.12",
	ConfigMethod: network.ConfigStatic,
	CIDRAddress:  "10.12.19.101/24",
	Origin:       network.OriginMachine,
}, {
	DeviceName:   "br-eth1.13",
	ConfigMethod: network.ConfigStatic,
	CIDRAddress:  "10.13.19.101/24",
	Origin:       network.OriginMachine,
}}

func (s *TypesSuite) TestNetworkInterfacesToStateArgs(c *gc.C) {
	interfaces := params.InterfaceInfoFromNetworkConfig(observedNetworkConfigs)
	devicesArgs, devicesAddrs := NetworkInterfacesToStateArgs(interfaces)

	c.Check(devicesArgs, jc.DeepEquals, expectedLinkLayerDeviceArgsWithFinalNetworkConfig)
	c.Check(devicesAddrs, jc.DeepEquals, expectedLinkLayerDeviceAddressesWithFinalNetworkConfig)
}

func (s *TypesSuite) TestAddressMatchingFromObservedConfig(c *gc.C) {
	cfg := []params.NetworkConfig{
		{DeviceIndex: 1, MACAddress: "", CIDR: "127.0.0.0/8", MTU: 65536, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "lo", ParentInterfaceName: "", InterfaceType: "loopback", Disabled: false, NoAutoStart: false, ConfigType: "loopback", Address: "127.0.0.1", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 1, MACAddress: "", CIDR: "::1/128", MTU: 65536, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "lo", ParentInterfaceName: "", InterfaceType: "loopback", Disabled: false, NoAutoStart: false, ConfigType: "loopback", Address: "::1", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 2, MACAddress: "ac:1f:6b:65:65:a4", CIDR: "", MTU: 1500, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "eno1", ParentInterfaceName: "", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 3, MACAddress: "ac:1f:6b:65:65:a5", CIDR: "", MTU: 1500, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "eno2", ParentInterfaceName: "", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 4, MACAddress: "ac:1f:6b:65:66:46", CIDR: "", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "eno3", ParentInterfaceName: "br-eno3", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 5, MACAddress: "ac:1f:6b:65:66:47", CIDR: "2001:41f0:86dd:1000::/64", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "eno4", ParentInterfaceName: "", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "static", Address: "2001:41f0:86dd:1000:ae1f:6bff:fe65:6647", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 5, MACAddress: "ac:1f:6b:65:66:47", CIDR: "", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "eno4", ParentInterfaceName: "", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 6, MACAddress: "ac:1f:6b:65:66:46", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "eno3.6", ParentInterfaceName: "br-eno3-6", InterfaceType: "802.1q", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 7, MACAddress: "ac:1f:6b:65:66:46", CIDR: "", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "eno3.7", ParentInterfaceName: "br-eno3-7", InterfaceType: "802.1q", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 8, MACAddress: "ac:1f:6b:65:66:46", CIDR: "", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "eno3.8", ParentInterfaceName: "br-eno3-8", InterfaceType: "802.1q", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 9, MACAddress: "00:16:3e:64:f0:6e", MTU: 1500, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "lxdbr0", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, Addresses: []params.Address{{Value: "10.47.242.1", CIDR: "10.47.242.0/24", ConfigType: "dhcp", IsSecondary: true}}, ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 10, MACAddress: "0e:a1:d0:85:e4:95", CIDR: "10.1.12.0/23", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "br-eno3-6", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, ConfigType: "static", Address: "10.1.12.3", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 10, MACAddress: "0e:a1:d0:85:e4:95", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "br-eno3-6", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 11, MACAddress: "ac:1f:6b:65:66:46", CIDR: "10.1.14.0/23", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "br-eno3-7", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, ConfigType: "static", Address: "10.1.14.3", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 11, MACAddress: "ac:1f:6b:65:66:46", CIDR: "", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "br-eno3-7", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 12, MACAddress: "1e:d2:8f:c1:d2:f7", CIDR: "10.0.4.0/22", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "br-eno3", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, ConfigType: "static", Address: "10.0.4.45", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "10.0.4.1", Routes: []params.NetworkRoute(nil), IsDefaultGateway: true, NetworkOrigin: "machine"},
		{DeviceIndex: 12, MACAddress: "1e:d2:8f:c1:d2:f7", CIDR: "2001:41f0:86dd:1000::/64", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "br-eno3", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, ConfigType: "static", Address: "2001:41f0:86dd:1000:1cd2:8fff:fec1:d2f7", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "10.0.4.1", Routes: []params.NetworkRoute(nil), IsDefaultGateway: true, NetworkOrigin: "machine"},
		{DeviceIndex: 12, MACAddress: "1e:d2:8f:c1:d2:f7", CIDR: "2001:41f0:86dd:1000::/64", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "br-eno3", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, ConfigType: "static", Address: "2001:41f0:86dd:1000::15", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "10.0.4.1", Routes: []params.NetworkRoute(nil), IsDefaultGateway: true, NetworkOrigin: "machine"},
		{DeviceIndex: 12, MACAddress: "1e:d2:8f:c1:d2:f7", CIDR: "", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "br-eno3", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "10.0.4.1", Routes: []params.NetworkRoute(nil), IsDefaultGateway: true, NetworkOrigin: "machine"},
		{DeviceIndex: 14, MACAddress: "1e:d2:8f:c1:d2:f7", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth07891da4", ParentInterfaceName: "br-eno3", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 16, MACAddress: "b6:02:ce:d5:a9:8c", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth2ee8ca11", ParentInterfaceName: "br-eno3-6", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 18, MACAddress: "f6:1c:02:49:4f:f8", CIDR: "", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth016acfa3", ParentInterfaceName: "br-eno3-7", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 20, MACAddress: "ea:70:44:9b:f4:c4", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth4df8cc58", ParentInterfaceName: "br-eno3", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 22, MACAddress: "86:12:35:b1:98:5a", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth47a2ce8b", ParentInterfaceName: "br-eno3-6", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 24, MACAddress: "92:d0:c7:c3:a5:28", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth0a1e21e6", ParentInterfaceName: "br-eno3", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 26, MACAddress: "d6:e8:a6:25:ea:6f", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth49e6014a", ParentInterfaceName: "br-eno3-6", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 28, MACAddress: "ae:cb:fb:62:a9:ca", CIDR: "", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "vetha2f459bd", ParentInterfaceName: "br-eno3-7", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 30, MACAddress: "ae:b7:70:51:a8:df", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth4fbab617", ParentInterfaceName: "br-eno3", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 32, MACAddress: "0e:a1:d0:85:e4:95", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "vethf198fca2", ParentInterfaceName: "br-eno3-6", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 34, MACAddress: "72:35:a4:66:e8:bb", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth906f682f", ParentInterfaceName: "br-eno3", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 36, MACAddress: "ea:81:6a:37:50:50", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth9c84c6e5", ParentInterfaceName: "br-eno3-6", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 37, MACAddress: "ac:1f:6b:65:66:46", CIDR: "10.1.16.0/23", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "br-eno3-8", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, ConfigType: "static", Address: "10.1.16.3", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 37, MACAddress: "ac:1f:6b:65:66:46", CIDR: "", MTU: 9214, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "br-eno3-8", ParentInterfaceName: "", InterfaceType: "bridge", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 39, MACAddress: "9a:c5:f8:4a:e4:36", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "veth0a86d9b8", ParentInterfaceName: "br-eno3", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
		{DeviceIndex: 41, MACAddress: "da:77:31:87:5a:15", CIDR: "", MTU: 9000, ProviderId: "", ProviderNetworkId: "", ProviderSubnetId: "", ProviderSpaceId: "", ProviderAddressId: "", ProviderVLANId: "", VLANTag: 0, InterfaceName: "vetha7e8da1b", ParentInterfaceName: "br-eno3-6", InterfaceType: "ethernet", Disabled: false, NoAutoStart: false, ConfigType: "manual", Address: "", Addresses: []params.Address(nil), ShadowAddresses: []params.Address(nil), DNSServers: []string(nil), DNSSearchDomains: []string(nil), GatewayAddress: "", Routes: []params.NetworkRoute(nil), IsDefaultGateway: false, NetworkOrigin: "machine"},
	}

	interfaces := params.InterfaceInfoFromNetworkConfig(cfg)
	breno38 := interfaces.GetByName("br-eno3-8")
	c.Check(breno38, gc.HasLen, 2)

	stateAddr := networkAddressStateArgsForDevice(interfaces, "br-eno3-8")
	c.Check(stateAddr, gc.DeepEquals, []state.LinkLayerDeviceAddress{{
		DeviceName:       "br-eno3-8",
		ConfigMethod:     "static",
		CIDRAddress:      "10.1.16.3/23",
		DNSServers:       []string{},
		IsDefaultGateway: false,
		Origin:           network.OriginMachine,
	}})
}
