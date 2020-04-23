// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

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

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type NetworkSuite struct {
	coretesting.BaseSuite

	stubConfigSource *stubNetworkConfigSource
}

var _ = gc.Suite(&NetworkSuite{})

func (s *NetworkSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stubConfigSource = &stubNetworkConfigSource{
		Stub:                  &testing.Stub{},
		fakeSysClassNetPath:   c.MkDir(),
		interfaces:            exampleObservedInterfaces,
		interfaceAddrs:        exampleObservedInterfaceAddrs,
		defaultRouteGatewayIP: net.ParseIP("1.2.3.4"),
		defaultRouteDevice:    "eth1",
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

func (s *NetworkSuite) TestGetObservedNetworkConfigInterfacesError(c *gc.C) {
	s.stubConfigSource.SetErrors(errors.New("no interfaces"))

	observedConfig, err := common.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, gc.ErrorMatches, "cannot get network interfaces: no interfaces")
	c.Check(observedConfig, gc.IsNil)

	s.stubConfigSource.CheckCallNames(c, "Interfaces")
}

func (s *NetworkSuite) TestGetObservedNetworkConfigInterfaceAddressesError(c *gc.C) {
	s.stubConfigSource.SetErrors(
		nil,                        // Interfaces
		nil,                        // DefaultRoute
		errors.New("no addresses"), // InterfaceAddressses
	)

	observedConfig, err := common.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, gc.ErrorMatches, `cannot get interface "lo" addresses: no addresses`)
	c.Check(observedConfig, gc.IsNil)

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "DefaultRoute", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 3, "InterfaceAddresses", "lo")
}

func (s *NetworkSuite) TestGetObservedNetworkConfigNoInterfaceAddresses(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[3:4] // only br-eth1
	s.stubConfigSource.interfaceAddrs = make(map[string][]net.Addr)
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "br-eth1", "bridge")

	observedConfig, err := common.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   11,
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           1500,
		InterfaceName: "br-eth1",
		InterfaceType: "bridge",
		ConfigType:    "manual",
		NetworkOrigin: params.NetworkOrigin("machine"),
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "DefaultRoute", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 3, "InterfaceAddresses", "br-eth1")
}

func (s *NetworkSuite) TestGetObservedNetworkConfigLoopbackInferred(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[0:1] // only lo
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "lo", "")

	observedConfig, err := common.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   1,
		CIDR:          "127.0.0.0/8",
		Address:       "127.0.0.1",
		MTU:           65536,
		InterfaceName: "lo",
		InterfaceType: "loopback", // inferred from the flags.
		ConfigType:    "loopback", // since it is a loopback
		NetworkOrigin: params.NetworkOrigin("machine"),
	}, {
		DeviceIndex:   1,
		CIDR:          "::1/128",
		Address:       "::1",
		MTU:           65536,
		InterfaceName: "lo",
		InterfaceType: "loopback",
		ConfigType:    "loopback",
		NetworkOrigin: params.NetworkOrigin("machine"),
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "DefaultRoute", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 3, "InterfaceAddresses", "lo")
}

func (s *NetworkSuite) TestGetObservedNetworkConfigVLANInferred(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[6:7] // only eth0.100
	s.stubConfigSource.interfaceAddrs = map[string][]net.Addr{
		"eth0.100": {
			fakeAddr("fe80::5054:ff:fedd:eef0:100/64"),
			fakeAddr("10.100.19.123/24"),
		},
	}
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "eth0.100", "vlan")

	observedConfig, err := common.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   13,
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "eth0.100",
		InterfaceType: "802.1q",
		ConfigType:    "manual", // the IPv6 address treated as empty.
		NetworkOrigin: params.NetworkOrigin("machine"),
	}, {
		DeviceIndex:   13,
		CIDR:          "10.100.19.0/24",
		Address:       "10.100.19.123",
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "eth0.100",
		InterfaceType: "802.1q",
		ConfigType:    "static",
		NetworkOrigin: params.NetworkOrigin("machine"),
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "DefaultRoute", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 3, "InterfaceAddresses", "eth0.100")
}

func (s *NetworkSuite) TestGetObservedNetworkConfigEthernetInfrerred(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[1:2] // only eth0
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "eth0", "")

	observedConfig, err := common.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   2,
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "eth0",
		InterfaceType: "ethernet",
		ConfigType:    "manual", // the IPv6 address treated as empty.
		NetworkOrigin: params.NetworkOrigin("machine"),
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "DefaultRoute", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 3, "InterfaceAddresses", "eth0")
}

func (s *NetworkSuite) TestGetObservedNetworkConfigBridgePortsHaveParentSet(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[1:5] // eth0, br-eth0, br-eth1, eth1
	br0Path := s.stubConfigSource.makeSysClassNetInterfacePath(c, "br-eth0", "bridge")
	// "extra" added below to verify bridge ports which are discovered, but not
	// found as interfaces from the source will be ignored.
	s.stubConfigSource.makeSysClassNetBridgePorts(c, br0Path, "eth0", "extra")
	br1Path := s.stubConfigSource.makeSysClassNetInterfacePath(c, "br-eth1", "bridge")
	s.stubConfigSource.makeSysClassNetBridgePorts(c, br1Path, "eth1")

	observedConfig, err := common.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:         2,
		MACAddress:          "aa:bb:cc:dd:ee:f0",
		MTU:                 1500,
		InterfaceName:       "eth0",
		InterfaceType:       "ethernet",
		ParentInterfaceName: "br-eth0",
		ConfigType:          "manual",
		NetworkOrigin:       params.NetworkOrigin("machine"),
	}, {
		DeviceIndex:   10,
		CIDR:          "10.20.19.0/24",
		Address:       "10.20.19.100",
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "br-eth0",
		InterfaceType: "bridge",
		ConfigType:    "static",
		NetworkOrigin: params.NetworkOrigin("machine"),
	}, {
		DeviceIndex:   10,
		CIDR:          "10.20.19.0/24",
		Address:       "10.20.19.123",
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "br-eth0",
		InterfaceType: "bridge",
		ConfigType:    "static",
		NetworkOrigin: params.NetworkOrigin("machine"),
	}, {
		DeviceIndex:   10,
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "br-eth0",
		InterfaceType: "bridge",
		ConfigType:    "manual",
		NetworkOrigin: params.NetworkOrigin("machine"),
	}, {
		DeviceIndex:   11,
		CIDR:          "10.20.19.0/24",
		Address:       "10.20.19.105",
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           1500,
		InterfaceName: "br-eth1",
		InterfaceType: "bridge",
		ConfigType:    "static",
		NetworkOrigin: params.NetworkOrigin("machine"),
	}, {
		DeviceIndex:   11,
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           1500,
		InterfaceName: "br-eth1",
		InterfaceType: "bridge",
		ConfigType:    "manual",
		NetworkOrigin: params.NetworkOrigin("machine"),
	}, {
		DeviceIndex:         3,
		MACAddress:          "aa:bb:cc:dd:ee:f1",
		MTU:                 1500,
		InterfaceName:       "eth1",
		InterfaceType:       "ethernet",
		ParentInterfaceName: "br-eth1",
		ConfigType:          "manual",
		GatewayAddress:      "1.2.3.4",
		IsDefaultGateway:    true,
		NetworkOrigin:       params.NetworkOrigin("machine"),
	}})

	s.stubConfigSource.CheckCallNames(c,
		"Interfaces",
		"DefaultRoute",
		"SysClassNetPath",
		"InterfaceAddresses", // eth0
		"InterfaceAddresses", // br-eth0
		"InterfaceAddresses", // br-eth1
		"InterfaceAddresses", // eth1
	)
	s.stubConfigSource.CheckCall(c, 3, "InterfaceAddresses", "eth0")
	s.stubConfigSource.CheckCall(c, 4, "InterfaceAddresses", "br-eth0")
	s.stubConfigSource.CheckCall(c, 5, "InterfaceAddresses", "br-eth1")
	s.stubConfigSource.CheckCall(c, 6, "InterfaceAddresses", "eth1")
}

func (s *NetworkSuite) TestGetObservedNetworkConfigAddressNotInCIDRFormat(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[1:2] // only eth0
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "eth0", "")
	// Simulate running on Windows, where net.InterfaceAddrs() returns
	// non-CIDR-formatted addresses.
	s.stubConfigSource.interfaceAddrs = map[string][]net.Addr{
		"eth0": {fakeAddr("10.20.19.42")},
	}

	observedConfig, err := common.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   2,
		Address:       "10.20.19.42", // just Address, no CIDR as netmask cannot be inferred.
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "eth0",
		InterfaceType: "ethernet",
		ConfigType:    "static",
		NetworkOrigin: params.NetworkOrigin("machine"),
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "DefaultRoute", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 3, "InterfaceAddresses", "eth0")
}

func (s *NetworkSuite) TestGetObservedNetworkConfigEmptyAddressValue(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[1:2] // only eth0
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "eth0", "")
	s.stubConfigSource.interfaceAddrs = map[string][]net.Addr{
		"eth0": {fakeAddr("")},
	}

	observedConfig, err := common.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(observedConfig, jc.DeepEquals, []params.NetworkConfig{{
		DeviceIndex:   2,
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		MTU:           1500,
		InterfaceName: "eth0",
		InterfaceType: "ethernet",
		ConfigType:    "manual",
		NetworkOrigin: params.NetworkOrigin("machine"),
	}})

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "DefaultRoute", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 3, "InterfaceAddresses", "eth0")
}

func (s *NetworkSuite) TestGetObservedNetworkConfigInvalidAddressValue(c *gc.C) {
	s.stubConfigSource.interfaces = exampleObservedInterfaces[1:2] // only eth0
	s.stubConfigSource.makeSysClassNetInterfacePath(c, "eth0", "")
	s.stubConfigSource.interfaceAddrs = map[string][]net.Addr{
		"eth0": {fakeAddr("invalid")},
	}

	observedConfig, err := common.GetObservedNetworkConfig(s.stubConfigSource)
	c.Check(err, gc.ErrorMatches, `cannot parse IP address "invalid" on interface "eth0"`)
	c.Check(observedConfig, gc.IsNil)

	s.stubConfigSource.CheckCallNames(c, "Interfaces", "DefaultRoute", "SysClassNetPath", "InterfaceAddresses")
	s.stubConfigSource.CheckCall(c, 3, "InterfaceAddresses", "eth0")
}

type stubNetworkConfigSource struct {
	*testing.Stub

	fakeSysClassNetPath   string
	interfaces            []net.Interface
	interfaceAddrs        map[string][]net.Addr
	defaultRouteGatewayIP net.IP
	defaultRouteDevice    string
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

// DefaultRoute implements NetworkConfigSource.
func (s *stubNetworkConfigSource) DefaultRoute() (net.IP, string, error) {
	s.AddCall("DefaultRoute")
	if err := s.NextErr(); err != nil {
		return nil, "", err
	}
	return s.defaultRouteGatewayIP, s.defaultRouteDevice, nil
}
