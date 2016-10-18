// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/juju/testing"
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

	stubConfigSource *stubNetworkConfigSource
}

var _ = gc.Suite(&TypesSuite{})

func (s *TypesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stubConfigSource = &stubNetworkConfigSource{
		Stub:                &testing.Stub{},
		fakeSysClassNetPath: c.MkDir(),
		interfaces:          exampleObservedInterfaces,
		interfaceAddrs:      exampleObservedInterfaceAddrs,
	}
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
	Index:        3,
	MTU:          1500,
	Name:         "eth1",
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
	"eth0":        {fakeAddr("fe80::5054:ff:fedd:eef0/64")},
	"eth1":        {fakeAddr("fe80::5054:ff:fedd:eef1/64")},
	"eth0.50":     {fakeAddr("fe80::5054:ff:fedd:eef0:50/64")},
	"eth0.100":    {fakeAddr("fe80::5054:ff:fedd:eef0:100/64")},
	"eth0.25":     {fakeAddr("fe80::5054:ff:fedd:eef0:25/64")},
	"eth1.11":     {fakeAddr("fe80::5054:ff:fedd:eef1:11/64")},
	"eth1.12":     {fakeAddr("fe80::5054:ff:fedd:eef1:12/64")},
	"eth1.13":     {fakeAddr("fe80::5054:ff:fedd:eef1:13/64")},
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

var expectedObservedNetworkConfigs = []params.NetworkConfig{{
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
	InterfaceType: string(network.BridgeInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.20.19.0/24",
	Address:       "10.20.19.100",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
}, {
	DeviceIndex:   10,
	InterfaceName: "br-eth0",
	InterfaceType: string(network.BridgeInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.20.19.0/24",
	Address:       "10.20.19.123",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
}, {
	DeviceIndex:   12,
	InterfaceName: "br-eth0.100",
	InterfaceType: string(network.BridgeInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.100.19.0/24",
	Address:       "10.100.19.100",
	MTU:           1500,
}, {
	DeviceIndex:   14,
	InterfaceName: "br-eth0.250",
	InterfaceType: string(network.BridgeInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.250.19.0/24",
	Address:       "10.250.19.100",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
}, {
	DeviceIndex:   16,
	InterfaceName: "br-eth0.50",
	InterfaceType: string(network.BridgeInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f0",
	CIDR:          "10.50.19.0/24",
	Address:       "10.50.19.100",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
}, {
	DeviceIndex:         2,
	InterfaceName:       "eth0",
	ParentInterfaceName: "br-eth0",
	InterfaceType:       string(network.EthernetInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
}, {
	DeviceIndex:         13,
	InterfaceName:       "eth0.100",
	ParentInterfaceName: "br-eth0.100",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
}, {
	DeviceIndex:         15,
	InterfaceName:       "eth0.250",
	ParentInterfaceName: "br-eth0.250",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
}, {
	DeviceIndex:         17,
	InterfaceName:       "eth0.50",
	ParentInterfaceName: "br-eth0.50",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f0",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
}, {
	DeviceIndex:   11,
	InterfaceName: "br-eth1",
	InterfaceType: string(network.BridgeInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.20.19.0/24",
	Address:       "10.20.19.105",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
}, {
	DeviceIndex:   18,
	InterfaceName: "br-eth1.11",
	InterfaceType: string(network.BridgeInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.11.19.0/24",
	Address:       "10.11.19.101",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
}, {
	DeviceIndex:   20,
	InterfaceName: "br-eth1.12",
	InterfaceType: string(network.BridgeInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.12.19.0/24",
	Address:       "10.12.19.101",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
}, {
	DeviceIndex:   22,
	InterfaceName: "br-eth1.13",
	InterfaceType: string(network.BridgeInterface),
	MACAddress:    "aa:bb:cc:dd:ee:f1",
	CIDR:          "10.13.19.0/24",
	Address:       "10.13.19.101",
	MTU:           1500,
	ConfigType:    string(network.ConfigStatic),
}, {
	DeviceIndex:         3,
	InterfaceName:       "eth1",
	ParentInterfaceName: "br-eth1",
	InterfaceType:       string(network.EthernetInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
}, {
	DeviceIndex:         19,
	InterfaceName:       "eth1.11",
	ParentInterfaceName: "br-eth1.11",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
}, {
	DeviceIndex:         21,
	InterfaceName:       "eth1.12",
	ParentInterfaceName: "br-eth1.12",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
}, {
	DeviceIndex:         23,
	InterfaceName:       "eth1.13",
	ParentInterfaceName: "br-eth1.13",
	InterfaceType:       string(network.VLAN_8021QInterface),
	MACAddress:          "aa:bb:cc:dd:ee:f1",
	MTU:                 1500,
	ConfigType:          string(network.ConfigManual),
}}

var expectedProviderNetworkConfigs = []params.NetworkConfig{{
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

var expectedFinalNetworkConfigs = []params.NetworkConfig{{
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
	ParentInterfaceName: "",
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
	ParentInterfaceName: "",
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
	ParentInterfaceName: "",
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
	ParentInterfaceName: "",
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
	ParentInterfaceName: "",
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
	ParentInterfaceName: "",
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

var expectedLinkLayerDeviceArgsWithFinalNetworkConfig = []state.LinkLayerDeviceArgs{{
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
	ParentName:  "",
}, {
	Name:        "br-eth0.250",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "",
}, {
	Name:        "br-eth0.50",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f0",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "",
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
	ParentName:  "",
}, {
	Name:        "br-eth1.12",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "",
}, {
	Name:        "br-eth1.13",
	MTU:         1500,
	Type:        state.BridgeDevice,
	MACAddress:  "aa:bb:cc:dd:ee:f1",
	IsAutoStart: true,
	IsUp:        true,
	ParentName:  "",
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

var expectedLinkLayerDeviceAdressesWithFinalNetworkConfig = []state.LinkLayerDeviceAddress{{
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

func (s *TypesSuite) TestNetworkConfigsToStateArgs(c *gc.C) {
	devicesArgs, devicesAddrs := networkingcommon.NetworkConfigsToStateArgs(expectedFinalNetworkConfigs)

	c.Check(devicesArgs, jc.DeepEquals, expectedLinkLayerDeviceArgsWithFinalNetworkConfig)
	c.Check(devicesAddrs, jc.DeepEquals, expectedLinkLayerDeviceAdressesWithFinalNetworkConfig)
}

func (s *TypesSuite) TestMergeProviderAndObservedNetworkConfigsBothNil(c *gc.C) {
	result := networkingcommon.MergeProviderAndObservedNetworkConfigs(nil, nil)
	c.Check(result, gc.IsNil)
}

func (s *TypesSuite) TestMergeProviderAndObservedNetworkConfigsNilObservedConfigs(c *gc.C) {
	input := expectedProviderNetworkConfigs
	result := networkingcommon.MergeProviderAndObservedNetworkConfigs(input, nil)
	c.Check(result, gc.IsNil)
}

func (s *TypesSuite) TestMergeProviderAndObservedNetworkConfigsNilProviderConfigs(c *gc.C) {
	input := expectedObservedNetworkConfigs
	result := networkingcommon.MergeProviderAndObservedNetworkConfigs(nil, input)
	c.Check(result, jc.DeepEquals, input)
}

func (s *TypesSuite) TestMergeProviderAndObservedNetworkConfigs(c *gc.C) {
	observedConfig := expectedObservedNetworkConfigs
	providerConfig := expectedProviderNetworkConfigs
	result := networkingcommon.MergeProviderAndObservedNetworkConfigs(providerConfig, observedConfig)
	c.Check(result, jc.DeepEquals, expectedFinalNetworkConfigs)
}

func (s *TypesSuite) TestGetObservedNetworkConfigInterfacesError(c *gc.C) {
	s.stubConfigSource.SetErrors(errors.New("no interfaces"))

	observedConfig, err := networkingcommon.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, gc.ErrorMatches, "cannot get network interfaces: no interfaces")
	c.Check(observedConfig, gc.IsNil)

	s.stubConfigSource.CheckCallNames(c, "Interfaces")
}

func (s *TypesSuite) TestGetObservedNetworkConfigInterfaceAddressesError(c *gc.C) {
	s.stubConfigSource.SetErrors(
		nil, // Interfaces() succeeds.
		errors.New("no addresses"), // InterfaceAddressses fails.
	)

	observedConfig, err := networkingcommon.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, gc.ErrorMatches, `cannot get interface "lo" addresses: no addresses`)
	c.Check(observedConfig, gc.IsNil)

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 2, "InterfaceAddresses", "lo")
}

func (s *TypesSuite) TestGetObservedNetworkConfigNoInterfaceAddresses(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[3:4] // only br-eth1
	s.stubConfigSource.interfaceAddrs = make(map[string][]net.Addr)
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "br-eth1", "bridge")

	observedConfig, err := networkingcommon.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   11,
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           1500,
		InterfaceName: "br-eth1",
		InterfaceType: "bridge",
		ConfigType:    "manual",
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 2, "InterfaceAddresses", "br-eth1")
}

func (s *TypesSuite) TestGetObservedNetworkConfigLoopbackInfrerred(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[0:1] // only lo
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "lo", "")

	observedConfig, err := networkingcommon.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   1,
		CIDR:          "127.0.0.0/8",
		Address:       "127.0.0.1",
		MTU:           65536,
		InterfaceName: "lo",
		InterfaceType: "loopback", // inferred from the flags.
		ConfigType:    "loopback", // since it is a loopback
	}, {
		DeviceIndex:   1,
		MTU:           65536,
		InterfaceName: "lo",
		InterfaceType: "loopback",
		ConfigType:    "loopback",
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 2, "InterfaceAddresses", "lo")
}

func (s *TypesSuite) TestGetObservedNetworkConfigVLANInfrerred(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[6:7] // only eth0.100
	s.stubConfigSource.interfaceAddrs = map[string][]net.Addr{
		"eth0.100": []net.Addr{
			fakeAddr("fe80::5054:ff:fedd:eef0:100/64"),
			fakeAddr("10.100.19.123/24"),
		},
	}
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "eth0.100", "vlan")

	observedConfig, err := networkingcommon.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   13,
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "eth0.100",
		InterfaceType: "802.1q",
		ConfigType:    "manual", // the IPv6 address treated as empty.
	}, {
		DeviceIndex:   13,
		CIDR:          "10.100.19.0/24",
		Address:       "10.100.19.123",
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "eth0.100",
		InterfaceType: "802.1q",
		ConfigType:    "static",
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 2, "InterfaceAddresses", "eth0.100")
}

func (s *TypesSuite) TestGetObservedNetworkConfigEthernetInfrerred(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[1:2] // only eth0
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "eth0", "")

	observedConfig, err := networkingcommon.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   2,
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "eth0",
		InterfaceType: "ethernet",
		ConfigType:    "manual", // the IPv6 address treated as empty.
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 2, "InterfaceAddresses", "eth0")
}

func (s *TypesSuite) TestGetObservedNetworkConfigBridgePortsHaveParentSet(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[1:5] // eth0, br-eth0, br-eth1, eth1
	br0Path := s.stubConfigSource.makeSysClassNetInterfacePath(c, "br-eth0", "bridge")
	// "extra" added below to verify bridge ports which are discovered, but not
	// found as interfaces from the source will be ignored.
	s.stubConfigSource.makeSysClassNetBridgePorts(c, br0Path, "eth0", "extra")
	br1Path := s.stubConfigSource.makeSysClassNetInterfacePath(c, "br-eth1", "bridge")
	s.stubConfigSource.makeSysClassNetBridgePorts(c, br1Path, "eth1")

	observedConfig, err := networkingcommon.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:         2,
		MACAddress:          "aa:bb:cc:dd:ee:f0",
		MTU:                 1500,
		InterfaceName:       "eth0",
		InterfaceType:       "ethernet",
		ParentInterfaceName: "br-eth0",
		ConfigType:          "manual",
	}, {
		DeviceIndex:   10,
		CIDR:          "10.20.19.0/24",
		Address:       "10.20.19.100",
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "br-eth0",
		InterfaceType: "bridge",
		ConfigType:    "static",
	}, {
		DeviceIndex:   10,
		CIDR:          "10.20.19.0/24",
		Address:       "10.20.19.123",
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "br-eth0",
		InterfaceType: "bridge",
		ConfigType:    "static",
	}, {
		DeviceIndex:   10,
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "br-eth0",
		InterfaceType: "bridge",
		ConfigType:    "manual",
	}, {
		DeviceIndex:   11,
		CIDR:          "10.20.19.0/24",
		Address:       "10.20.19.105",
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           1500,
		InterfaceName: "br-eth1",
		InterfaceType: "bridge",
		ConfigType:    "static",
	}, {
		DeviceIndex:   11,
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           1500,
		InterfaceName: "br-eth1",
		InterfaceType: "bridge",
		ConfigType:    "manual",
	}, {
		DeviceIndex:         3,
		MACAddress:          "aa:bb:cc:dd:ee:f1",
		MTU:                 1500,
		InterfaceName:       "eth1",
		InterfaceType:       "ethernet",
		ParentInterfaceName: "br-eth1",
		ConfigType:          "manual",
	}})

	s.stubConfigSource.CheckCallNames(c,
		"Interfaces", "SysClassNetPath",
		"InterfaceAddresses", // eth0
		"InterfaceAddresses", // br-eth0
		"InterfaceAddresses", // br-eth1
		"InterfaceAddresses", // eth1
	)
	s.stubConfigSource.CheckCall(c, 2, "InterfaceAddresses", "eth0")
	s.stubConfigSource.CheckCall(c, 3, "InterfaceAddresses", "br-eth0")
	s.stubConfigSource.CheckCall(c, 4, "InterfaceAddresses", "br-eth1")
	s.stubConfigSource.CheckCall(c, 5, "InterfaceAddresses", "eth1")
}

func (s *TypesSuite) TestGetObservedNetworkConfigAddressNotInCIDRFormat(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[1:2] // only eth0
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "eth0", "")
	// Simluate running on Windows, where net.InterfaceAddrs() returns
	// non-CIDR-formatted addresses.
	s.stubConfigSource.interfaceAddrs = map[string][]net.Addr{
		"eth0": []net.Addr{fakeAddr("10.20.19.42")},
	}

	observedConfig, err := networkingcommon.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   2,
		Address:       "10.20.19.42", // just Address, no CIDR as netmask cannot be inferred.
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "eth0",
		InterfaceType: "ethernet",
		ConfigType:    "static",
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 2, "InterfaceAddresses", "eth0")
}

func (s *TypesSuite) TestGetObservedNetworkConfigEmptyAddressValue(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[1:2] // only eth0
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "eth0", "")
	s.stubConfigSource.interfaceAddrs = map[string][]net.Addr{
		"eth0": []net.Addr{fakeAddr("")},
	}

	observedConfig, err := networkingcommon.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   2,
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "eth0",
		InterfaceType: "ethernet",
		ConfigType:    "manual",
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 2, "InterfaceAddresses", "eth0")
}

func (s *TypesSuite) TestGetObservedNetworkConfigInvalidAddressValue(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[1:2] // only eth0
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "eth0", "")
	s.stubConfigSource.interfaceAddrs = map[string][]net.Addr{
		"eth0": []net.Addr{fakeAddr("invalid")},
	}

	observedConfig, err := networkingcommon.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, gc.ErrorMatches, `cannot parse IP address "invalid" on interface "eth0"`)
	c.Check(observedConfig, gc.IsNil)

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 2, "InterfaceAddresses", "eth0")
}

type stubNetworkConfigSource struct {
	*testing.Stub

	fakeSysClassNetPath string
	interfaces          []net.Interface
	interfaceAddrs      map[string][]net.Addr
}

// makeSysClassNetInterfacePath creates a subdir for the given interfaceName,
// and a uevent file there with the given devtype set. Returns the created path.
func (s *stubNetworkConfigSource) makeSysClassNetInterfacePath(c *gc.C, interfaceName, devType string) string {
	interfacePath := filepath.Join(s.fakeSysClassNetPath, interfaceName)
	err := os.Mkdir(interfacePath, 0755)
	c.Assert(err, jc.ErrorIsNil)

	var contents string
	if devType == "" {
		contents = fmt.Sprintf(`
IFINDEX=42
INTERFACE=%s
`, interfaceName)
	} else {
		contents = fmt.Sprintf(`
IFINDEX=42
INTERFACE=%s
DEVTYPE=%s
`, interfaceName, devType)
	}
	ueventPath := filepath.Join(interfacePath, "uevent")
	err = ioutil.WriteFile(ueventPath, []byte(contents), 0644)
	c.Assert(err, jc.ErrorIsNil)

	return interfacePath
}

// makeSysClassNetBridgePorts creates a "brif" subdir in the given
// interfacePath, and one file for each entry in the given ports, named the same
// as the port value. Needed to simulate the FS structure network.GetBridgePorts()
// can handle.
func (s *stubNetworkConfigSource) makeSysClassNetBridgePorts(c *gc.C, interfacePath string, ports ...string) {
	brifPath := filepath.Join(interfacePath, "brif")
	err := os.Mkdir(brifPath, 0755)
	c.Assert(err, jc.ErrorIsNil)

	for _, portName := range ports {
		portPath := filepath.Join(brifPath, portName)
		err = ioutil.WriteFile(portPath, []byte("#empty"), 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
}

// SysClassNetPath implements NetworkConfigSource.
func (s *stubNetworkConfigSource) SysClassNetPath() string {
	s.AddCall("SysClassNetPath")
	return s.fakeSysClassNetPath
}

// Interfaces implements NetworkConfigSource.
func (s *stubNetworkConfigSource) Interfaces() ([]net.Interface, error) {
	s.AddCall("Interfaces")
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return s.interfaces, nil
}

// InterfaceAddresses implements NetworkConfigSource.
func (s *stubNetworkConfigSource) InterfaceAddresses(name string) ([]net.Addr, error) {
	s.AddCall("InterfaceAddresses", name)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return s.interfaceAddrs[name], nil
}
