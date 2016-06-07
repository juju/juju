// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon_test

import (
	"fmt"
	"math/rand"
	"net"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type TypesSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&TypesSuite{})

func (s *TypesSuite) TestCopyNetworkConfig(c *gc.C) {
	inputAndExpectedOutput := []params.NetworkConfig{{
		InterfaceName: "foo",
		DNSServers:    []string{"bar", "baz"},
		Address:       "0.1.2.3",
	}, {
		DeviceIndex:         124,
		ParentInterfaceName: "parent",
		ProviderId:          "nic-id",
	}}

	output := networkingcommon.CopyNetworkConfigs(inputAndExpectedOutput)
	c.Assert(output, jc.DeepEquals, inputAndExpectedOutput)
}

func mustParseMAC(value string) net.HardwareAddr {
	parsedMAC, err := net.ParseMAC(value)
	if err != nil {
		panic(fmt.Sprintf("cannot parse MAC %q: %v", value, err))
	}
	return parsedMAC
}

var exampleObservedInterfaces = []net.Interface{{
	Index: 1,
	MTU:   65536,
	Name:  "lo",
	Flags: net.FlagUp | net.FlagLoopback,
}, {
	Index:        2,
	MTU:          1500,
	Name:         "eth0",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f0"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        3,
	MTU:          1500,
	Name:         "eth1",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f1"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        10,
	MTU:          1500,
	Name:         "br-eth0",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f0"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        11,
	MTU:          1500,
	Name:         "br-eth1",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f1"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        12,
	MTU:          1500,
	Name:         "br-eth0.100",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f0"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        13,
	MTU:          1500,
	Name:         "eth0.100",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f0"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        14,
	MTU:          1500,
	Name:         "br-eth0.250",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f0"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        15,
	MTU:          1500,
	Name:         "eth0.250",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f0"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        16,
	MTU:          1500,
	Name:         "br-eth0.50",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f0"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        17,
	MTU:          1500,
	Name:         "eth0.50",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f0"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        18,
	MTU:          1500,
	Name:         "br-eth1.11",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f1"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        19,
	MTU:          1500,
	Name:         "eth1.11",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f1"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        20,
	MTU:          1500,
	Name:         "br-eth1.12",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f1"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        21,
	MTU:          1500,
	Name:         "eth1.12",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f1"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        22,
	MTU:          1500,
	Name:         "br-eth1.13",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f1"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}, {
	Index:        23,
	MTU:          1500,
	Name:         "eth1.13",
	HardwareAddr: mustParseMAC("aa:bb:cc:dd:ee:f1"),
	Flags:        net.FlagUp | net.FlagBroadcast | net.FlagMulticast,
}}

type fakeAddr string

func (f fakeAddr) Network() string { return "" }
func (f fakeAddr) String() string  { return string(f) }

var _ net.Addr = (*fakeAddr)(nil)

var exampleObservedInterfaceAddrs = map[string][]net.Addr{
	"eth0":        nil,
	"eth1":        nil,
	"eth0.50":     nil,
	"eth0.100":    nil,
	"eth0.25":     nil,
	"eth1.11":     nil,
	"eth1.12":     nil,
	"eth1.13":     nil,
	"lo":          {fakeAddr("127.0.0.1/8"), fakeAddr("::1/128")},
	"br-eth0":     {fakeAddr("10.20.19.100/24"), fakeAddr("10.20.19.123/24"), fakeAddr("fe80::5054:ff:fedd:eef0/64")},
	"br-eth1":     {fakeAddr("10.20.19.105/24"), fakeAddr("fe80::5054:ff:fedd:eef1/64")},
	"br-eth0.50":  {fakeAddr("10.50.19.100/24"), fakeAddr("fe80::5054:ff:fedd:eef0/64")},
	"br-eth0.100": {fakeAddr("10.100.19.100/24"), fakeAddr("fe80::5054:ff:fedd:eef0/64")},
	"br-eth0.250": {fakeAddr("10.250.19.100/24"), fakeAddr("fe80::5054:ff:fedd:eef0/64")},
	"br-eth1.11":  {fakeAddr("10.11.19.101/24"), fakeAddr("fe80::5054:ff:fedd:eef1/64")},
	"br-eth1.12":  {fakeAddr("10.12.19.101/24"), fakeAddr("fe80::5054:ff:fedd:eef1/64")},
	"br-eth1.13":  {fakeAddr("10.13.19.101/24"), fakeAddr("fe80::5054:ff:fedd:eef1/64")},
}

var expectedSortedObservedNetworkConfigs = []params.NetworkConfig{{
	DeviceIndex:   1,
	InterfaceName: "lo",
	InterfaceType: string(network.LoopbackInterface),
	MACAddress:    "",
	CIDR:          "127.0.0.0/8",
	Address:       "127.0.0.1",
	MTU:           65536,
	ConfigType:    string(network.ConfigLoopback),
}, {
	DeviceIndex:   10,
	InterfaceName: "br-eth0",
	InterfaceType: string(network.EthernetInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.20.19.0/24",
	Address:       "10.20.19.100",
	MTU:           1500,
}, {
	DeviceIndex:   10,
	InterfaceName: "br-eth0",
	InterfaceType: string(network.EthernetInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.20.19.0/24",
	Address:       "10.20.19.123",
	MTU:           1500,
}, {
	DeviceIndex:   12,
	InterfaceName: "br-eth0.100",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.100.19.0/24",
	Address:       "10.100.19.100",
	MTU:           1500,
}, {
	DeviceIndex:   14,
	InterfaceName: "br-eth0.250",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.250.19.0/24",
	Address:       "10.250.19.100",
	MTU:           1500,
}, {
	DeviceIndex:   16,
	InterfaceName: "br-eth0.50",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.50.19.0/24",
	Address:       "10.50.19.100",
	MTU:           1500,
}, {
	DeviceIndex:   2,
	InterfaceName: "eth0",
	InterfaceType: string(network.EthernetInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	MTU:           1500,
}, {
	DeviceIndex:   13,
	InterfaceName: "eth0.100",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	MTU:           1500,
}, {
	DeviceIndex:   15,
	InterfaceName: "eth0.250",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	MTU:           1500,
}, {
	DeviceIndex:   17,
	InterfaceName: "eth0.50",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	MTU:           1500,
}, {
	DeviceIndex:   11,
	InterfaceName: "br-eth1",
	InterfaceType: string(network.EthernetInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.20.19.0/24",
	Address:       "10.20.19.105",
	MTU:           1500,
}, {
	DeviceIndex:   18,
	InterfaceName: "br-eth1.11",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.11.19.0/24",
	Address:       "10.11.19.101",
	MTU:           1500,
}, {
	DeviceIndex:   20,
	InterfaceName: "br-eth1.12",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.12.19.0/24",
	Address:       "10.12.19.101",
	MTU:           1500,
}, {
	DeviceIndex:   22,
	InterfaceName: "br-eth1.13",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.13.19.0/24",
	Address:       "10.13.19.101",
	MTU:           1500,
}, {
	DeviceIndex:   3,
	InterfaceName: "eth1",
	InterfaceType: string(network.EthernetInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	MTU:           1500,
}, {
	DeviceIndex:   19,
	InterfaceName: "eth1.11",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	MTU:           1500,
}, {
	DeviceIndex:   21,
	InterfaceName: "eth1.12",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	MTU:           1500,
}, {
	DeviceIndex:   23,
	InterfaceName: "eth1.13",
	InterfaceType: string(network.VLAN_8021QInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	MTU:           1500,
}}

var expectedSortedProviderNetworkConfigs = []params.NetworkConfig{{
	InterfaceName:       "eth0",
	InterfaceType:       string(network.EthernetInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	CIDR:                "10.20.19.0/24",
	Address:             "10.20.19.100",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "",
	ProviderId:          "3",
	ProviderSubnetId:    "3",
	ProviderVLANId:      "5001",
	VLANTag:             0,
	ProviderAddressId:   "1287",
}, {
	InterfaceName:       "eth0",
	InterfaceType:       string(network.EthernetInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	CIDR:                "10.20.19.0/24",
	Address:             "10.20.19.123",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "",
	ProviderId:          "3",
	ProviderSubnetId:    "3",
	ProviderVLANId:      "5001",
	VLANTag:             0,
	ProviderAddressId:   "1288",
}, {
	InterfaceName:       "eth0.100",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	CIDR:                "10.100.19.0/24",
	Address:             "10.100.19.100",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "eth0",
	ProviderId:          "516",
	ProviderSubnetId:    "6",
	ProviderVLANId:      "5005",
	VLANTag:             100,
	ProviderAddressId:   "1292",
}, {
	InterfaceName:       "eth0.250",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	CIDR:                "10.250.19.0/24",
	Address:             "10.250.19.100",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "eth0",
	ProviderId:          "517",
	ProviderSubnetId:    "8",
	ProviderVLANId:      "5008",
	VLANTag:             250,
	ProviderAddressId:   "1294",
}, {
	InterfaceName:       "eth0.50",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	CIDR:                "10.50.19.0/24",
	Address:             "10.50.19.100",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "eth0",
	ProviderId:          "515",
	ProviderSubnetId:    "5",
	ProviderVLANId:      "5004",
	VLANTag:             50,
	ProviderAddressId:   "1290",
}, {
	InterfaceName:       "eth1",
	InterfaceType:       string(network.EthernetInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	CIDR:                "10.20.19.0/24",
	Address:             "10.20.19.105",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "",
	ProviderId:          "245",
	ProviderSubnetId:    "3",
	ProviderVLANId:      "5001",
	VLANTag:             0,
	ProviderAddressId:   "1295",
}, {
	InterfaceName:       "eth1.11",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	CIDR:                "10.11.19.0/24",
	Address:             "10.11.19.101",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "eth1",
	ProviderId:          "518",
	ProviderSubnetId:    "9",
	ProviderVLANId:      "5013",
	VLANTag:             11,
	ProviderAddressId:   "1298",
}, {
	InterfaceName:       "eth1.12",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	CIDR:                "10.12.19.0/24",
	Address:             "10.12.19.101",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "eth1",
	ProviderId:          "519",
	ProviderSubnetId:    "10",
	ProviderVLANId:      "5014",
	VLANTag:             12,
	ProviderAddressId:   "1300",
}, {
	InterfaceName:       "eth1.13",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	CIDR:                "10.13.19.0/24",
	Address:             "10.13.19.101",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "eth1",
	ProviderId:          "520",
	ProviderSubnetId:    "11",
	ProviderVLANId:      "5015",
	VLANTag:             13,
	ProviderAddressId:   "1302",
}}

var expectedSortedMergedNetworkConfigs = []params.NetworkConfig{{
	DeviceIndex:         1,
	InterfaceName:       "lo",
	InterfaceType:       string(network.LoopbackInterface),
	CIDR:                "127.0.0.0/8",
	Address:             "127.0.0.1",
	MTU:                 65536,
	ConfigType:          string(network.ConfigLoopback),
	ParentInterfaceName: "",
}, {
	DeviceIndex:         10,
	InterfaceName:       "br-eth0",
	InterfaceType:       string(network.BridgeInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	CIDR:                "10.20.19.0/24",
	Address:             "10.20.19.100",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "",
	ProviderSubnetId:    "3",
	ProviderVLANId:      "5001",
	VLANTag:             0,
	ProviderAddressId:   "1287",
}, {
	DeviceIndex:         10,
	InterfaceName:       "br-eth0",
	InterfaceType:       string(network.BridgeInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	CIDR:                "10.20.19.0/24",
	Address:             "10.20.19.123",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "",
	ProviderSubnetId:    "3",
	ProviderVLANId:      "5001",
	VLANTag:             0,
	ProviderAddressId:   "1288",
}, {
	DeviceIndex:         12,
	InterfaceName:       "br-eth0.100",
	InterfaceType:       string(network.BridgeInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	CIDR:                "10.100.19.0/24",
	Address:             "10.100.19.100",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "br-eth0",
	ProviderSubnetId:    "6",
	ProviderVLANId:      "5005",
	VLANTag:             100,
	ProviderAddressId:   "1292",
}, {
	DeviceIndex:         14,
	InterfaceName:       "br-eth0.250",
	InterfaceType:       string(network.BridgeInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	CIDR:                "10.250.19.0/24",
	Address:             "10.250.19.100",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "br-eth0",
	ProviderSubnetId:    "8",
	ProviderVLANId:      "5008",
	VLANTag:             250,
	ProviderAddressId:   "1294",
}, {
	DeviceIndex:         16,
	InterfaceName:       "br-eth0.50",
	InterfaceType:       string(network.BridgeInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	CIDR:                "10.50.19.0/24",
	Address:             "10.50.19.100",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "br-eth0",
	ProviderSubnetId:    "5",
	ProviderVLANId:      "5004",
	VLANTag:             50,
	ProviderAddressId:   "1290",
}, {
	DeviceIndex:         2,
	InterfaceName:       "eth0",
	InterfaceType:       string(network.EthernetInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	ParentInterfaceName: "br-eth0",
	ProviderId:          "3",
	ProviderSubnetId:    "3",
	ProviderVLANId:      "5001",
	VLANTag:             0,
}, {
	DeviceIndex:         13,
	InterfaceName:       "eth0.100",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	ParentInterfaceName: "br-eth0.100",
	ProviderId:          "516",
	ProviderSubnetId:    "6",
	ProviderVLANId:      "5005",
	VLANTag:             100,
}, {
	DeviceIndex:         15,
	InterfaceName:       "eth0.250",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	ParentInterfaceName: "br-eth0.250",
	ProviderId:          "517",
	ProviderSubnetId:    "8",
	ProviderVLANId:      "5008",
	VLANTag:             250,
}, {
	DeviceIndex:         17,
	InterfaceName:       "eth0.50",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	ParentInterfaceName: "br-eth0.50",
	ProviderId:          "515",
	ProviderSubnetId:    "5",
	ProviderVLANId:      "5004",
	VLANTag:             50,
}, {
	DeviceIndex:         11,
	InterfaceName:       "br-eth1",
	InterfaceType:       string(network.BridgeInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	CIDR:                "10.20.19.0/24",
	Address:             "10.20.19.105",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "",
	ProviderSubnetId:    "3",
	ProviderVLANId:      "5001",
	VLANTag:             0,
	ProviderAddressId:   "1295",
}, {
	DeviceIndex:         18,
	InterfaceName:       "br-eth1.11",
	InterfaceType:       string(network.BridgeInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	CIDR:                "10.11.19.0/24",
	Address:             "10.11.19.101",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "br-eth1",
	ProviderSubnetId:    "9",
	ProviderVLANId:      "5013",
	VLANTag:             11,
	ProviderAddressId:   "1298",
}, {
	DeviceIndex:         20,
	InterfaceName:       "br-eth1.12",
	InterfaceType:       string(network.BridgeInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	CIDR:                "10.12.19.0/24",
	Address:             "10.12.19.101",
	MTU:                 1500,
	ConfigType:          string(network.ConfigStatic),
	ParentInterfaceName: "br-eth1",
	ProviderSubnetId:    "10",
	ProviderVLANId:      "5014",
	VLANTag:             12,
	ProviderAddressId:   "1300",
}, {
	DeviceIndex:         22,
	InterfaceName:       "br-eth1.13",
	InterfaceType:       string(network.BridgeInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	CIDR:                "10.13.19.0/24",
	Address:             "10.13.19.101",
	MTU:                 1500,
	ParentInterfaceName: "br-eth1",
	ConfigType:          string(network.ConfigStatic),
	ProviderSubnetId:    "11",
	ProviderVLANId:      "5015",
	VLANTag:             13,
	ProviderAddressId:   "1302",
}, {
	DeviceIndex:         3,
	InterfaceName:       "eth1",
	InterfaceType:       string(network.EthernetInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	ParentInterfaceName: "br-eth1",
	ProviderId:          "245",
	ProviderSubnetId:    "3",
	ProviderVLANId:      "5001",
	VLANTag:             0,
}, {
	DeviceIndex:         19,
	InterfaceName:       "eth1.11",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	ParentInterfaceName: "br-eth1.11",
	ProviderId:          "518",
	ProviderSubnetId:    "9",
	ProviderVLANId:      "5013",
	VLANTag:             11,
}, {
	DeviceIndex:         21,
	InterfaceName:       "eth1.12",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	ParentInterfaceName: "br-eth1.12",
	ProviderId:          "519",
	ProviderSubnetId:    "10",
	ProviderVLANId:      "5014",
	VLANTag:             12,
}, {
	DeviceIndex:         23,
	InterfaceName:       "eth1.13",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
	ParentInterfaceName: "br-eth1.13",
	ProviderId:          "520",
	ProviderSubnetId:    "11",
	ProviderVLANId:      "5015",
	VLANTag:             13,
}}

var expectedSortedNetworkConfigsByInterfaceName = []params.NetworkConfig{
	{InterfaceName: "br-eth0"},
	{InterfaceName: "br-eth0.12"},
	{InterfaceName: "br-eth0.34"},
	{InterfaceName: "br-eth1"},
	{InterfaceName: "br-eth1.100"},
	{InterfaceName: "br-eth1.250"},
	{InterfaceName: "br-eth1.50"},
	{InterfaceName: "eth0"},
	{InterfaceName: "eth0.12"},
	{InterfaceName: "eth0.34"},
	{InterfaceName: "eth1"},
	{InterfaceName: "eth1.100"},
	{InterfaceName: "eth1.250"},
	{InterfaceName: "eth1.50"},
}

var expectedLinkLayerDeviceArgsWithMergedNetworkConfig = []state.LinkLayerDeviceArgs{{
	Name:        "lo",
	MTU:         65536,
	Type:        state.LoopbackDevice,
	IsAutoStart: true,
	IsUp:        true,
}, {
	Name:        "br-eth0",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
}, {
	Name:        "br-eth0.100",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0",
}, {
	Name:        "br-eth0.250",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0",
}, {
	Name:        "br-eth0.50",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0",
}, {
	Name:        "eth0",
	MTU:         1500,
	ProviderID:  "3",
	Type:        state.EthernetDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0",
}, {
	Name:        "eth0.100",
	MTU:         1500,
	ProviderID:  "516",
	Type:        state.VLAN_8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0.100",
}, {
	Name:        "eth0.250",
	MTU:         1500,
	ProviderID:  "517",
	Type:        state.VLAN_8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0.250",
}, {
	Name:        "eth0.50",
	MTU:         1500,
	ProviderID:  "515",
	Type:        state.VLAN_8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth0.50",
}, {
	Name:        "br-eth1",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
}, {
	Name:        "br-eth1.11",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1",
}, {
	Name:        "br-eth1.12",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1",
}, {
	Name:        "br-eth1.13",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1",
}, {
	Name:        "eth1",
	MTU:         1500,
	ProviderID:  "245",
	Type:        state.EthernetDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1",
}, {
	Name:        "eth1.11",
	MTU:         1500,
	ProviderID:  "518",
	Type:        state.VLAN_8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1.11",
}, {
	Name:        "eth1.12",
	MTU:         1500,
	ProviderID:  "519",
	Type:        state.VLAN_8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1.12",
}, {
	Name:        "eth1.13",
	MTU:         1500,
	ProviderID:  "520",
	Type:        state.VLAN_8021QDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "br-eth1.13",
}}

var expectedLinkLayerDeviceAdressesWithMergedNetworkConfig = []state.LinkLayerDeviceAddress{{
	DeviceName:   "lo",
	ConfigMethod: state.LoopbackAddress,
	CIDRAddress:  "127.0.0.1/8",
}, {
	DeviceName:   "br-eth0",
	ConfigMethod: state.StaticAddress,
	CIDRAddress:  "10.20.19.100/24",
	ProviderID:   "1287",
}, {
	DeviceName:   "br-eth0",
	ConfigMethod: state.StaticAddress,
	CIDRAddress:  "10.20.19.123/24",
	ProviderID:   "1288",
}, {
	DeviceName:   "br-eth0.100",
	ConfigMethod: state.StaticAddress,
	CIDRAddress:  "10.100.19.100/24",
	ProviderID:   "1292",
}, {
	DeviceName:   "br-eth0.250",
	ConfigMethod: state.StaticAddress,
	CIDRAddress:  "10.250.19.100/24",
	ProviderID:   "1294",
}, {
	DeviceName:   "br-eth0.50",
	ConfigMethod: state.StaticAddress,
	CIDRAddress:  "10.50.19.100/24",
	ProviderID:   "1290",
}, {
	DeviceName:   "br-eth1",
	ConfigMethod: state.StaticAddress,
	CIDRAddress:  "10.20.19.105/24",
	ProviderID:   "1295",
}, {
	DeviceName:   "br-eth1.11",
	ConfigMethod: state.StaticAddress,
	CIDRAddress:  "10.11.19.101/24",
	ProviderID:   "1298",
}, {
	DeviceName:   "br-eth1.12",
	ConfigMethod: state.StaticAddress,
	CIDRAddress:  "10.12.19.101/24",
	ProviderID:   "1300",
}, {
	DeviceName:   "br-eth1.13",
	ConfigMethod: state.StaticAddress,
	CIDRAddress:  "10.13.19.101/24",
	ProviderID:   "1302",
}}

func (s *TypesSuite) TestSortNetworkConfigsByParentsWithObservedConfigs(c *gc.C) {
	s.checkSortNetworkConfigsByParentsWithAllInputPremutationsMatches(c, expectedSortedObservedNetworkConfigs)
}

func (s *TypesSuite) checkSortNetworkConfigsByParentsWithAllInputPremutationsMatches(c *gc.C, expectedOutput []params.NetworkConfig) {
	expectedLength := len(expectedOutput)
	jsonExpected := s.networkConfigsAsJSON(c, expectedOutput)
	for i := 0; i < expectedLength; i++ {
		shuffledInput := shuffleNetworkConfigs(expectedOutput)
		result := networkingcommon.SortNetworkConfigsByParents(shuffledInput)
		c.Assert(result, gc.HasLen, expectedLength)
		jsonResult := s.networkConfigsAsJSON(c, result)
		c.Check(jsonResult, gc.Equals, jsonExpected)
	}
}

func (s *TypesSuite) networkConfigsAsJSON(c *gc.C, input []params.NetworkConfig) string {
	asJSON, err := networkingcommon.NetworkConfigsToIndentedJSON(input)
	c.Assert(err, jc.ErrorIsNil)
	return asJSON
}

func shuffleNetworkConfigs(input []params.NetworkConfig) []params.NetworkConfig {
	inputLength := len(input)
	output := make([]params.NetworkConfig, inputLength)
	shuffled := rand.Perm(inputLength)
	for i, j := range shuffled {
		output[i] = input[j]
	}
	return output
}

func (s *TypesSuite) TestSortNetworkConfigsByParentsWithProviderConfigs(c *gc.C) {
	s.checkSortNetworkConfigsByParentsWithAllInputPremutationsMatches(c, expectedSortedProviderNetworkConfigs)
}

func (s *TypesSuite) TestSortNetworkConfigsByParentsWithMergedConfigs(c *gc.C) {
	s.checkSortNetworkConfigsByParentsWithAllInputPremutationsMatches(c, expectedSortedMergedNetworkConfigs)
}

func (s *TypesSuite) TestSortNetworkConfigsByInterfaceName(c *gc.C) {
	expectedLength := len(expectedSortedNetworkConfigsByInterfaceName)
	jsonExpected := s.networkConfigsAsJSON(c, expectedSortedNetworkConfigsByInterfaceName)
	for i := 0; i < expectedLength; i++ {
		shuffledInput := shuffleNetworkConfigs(expectedSortedNetworkConfigsByInterfaceName)
		result := networkingcommon.SortNetworkConfigsByInterfaceName(shuffledInput)
		c.Assert(result, gc.HasLen, expectedLength)
		jsonResult := s.networkConfigsAsJSON(c, result)
		c.Check(jsonResult, gc.Equals, jsonExpected)
	}
}

func (s *TypesSuite) TestMergeProviderAndObservedNetworkConfigs(c *gc.C) {
	observedConfigsLength := len(expectedSortedObservedNetworkConfigs)
	providerConfigsLength := len(expectedSortedProviderNetworkConfigs)
	jsonExpected := s.networkConfigsAsJSON(c, expectedSortedMergedNetworkConfigs)
	for i := 0; i < observedConfigsLength; i++ {
		shuffledObservedConfigs := shuffleNetworkConfigs(expectedSortedObservedNetworkConfigs)
		for j := 0; j < providerConfigsLength; j++ {
			shuffledProviderConfigs := shuffleNetworkConfigs(expectedSortedProviderNetworkConfigs)

			mergedConfigs, err := networkingcommon.MergeProviderAndObservedNetworkConfigs(shuffledProviderConfigs, shuffledObservedConfigs)
			c.Assert(err, jc.ErrorIsNil)
			jsonResult := s.networkConfigsAsJSON(c, mergedConfigs)
			c.Check(jsonResult, gc.Equals, jsonExpected)
		}
	}
}

func (s *TypesSuite) TestGetObservedNetworkConfig(c *gc.C) {
	s.PatchValue(networkingcommon.NetInterfaces, func() ([]net.Interface, error) {
		return exampleObservedInterfaces, nil
	})
	s.PatchValue(networkingcommon.InterfaceAddrs, func(i *net.Interface) ([]net.Addr, error) {
		c.Assert(i, gc.NotNil)
		if addrs, found := exampleObservedInterfaceAddrs[i.Name]; found {
			return addrs, nil
		}
		return nil, nil
	})

	observedConfig, err := networkingcommon.GetObservedNetworkConfig()
	c.Assert(err, jc.ErrorIsNil)
	jsonResult := s.networkConfigsAsJSON(c, observedConfig)
	jsonExpected := s.networkConfigsAsJSON(c, expectedSortedObservedNetworkConfigs)
	c.Check(jsonResult, gc.Equals, jsonExpected)
}

func (s *TypesSuite) TestNetworkConfigsToStateArgs(c *gc.C) {
	devicesArgs, devicesAddrs := networkingcommon.NetworkConfigsToStateArgs(expectedSortedMergedNetworkConfigs)

	c.Check(devicesArgs, jc.DeepEquals, expectedLinkLayerDeviceArgsWithMergedNetworkConfig)
	c.Check(devicesAddrs, jc.DeepEquals, expectedLinkLayerDeviceAdressesWithMergedNetworkConfig)
}
