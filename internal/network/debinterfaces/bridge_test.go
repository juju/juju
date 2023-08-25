// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/network/debinterfaces"
)

type BridgeSuite struct {
	testing.IsolationSuite

	expander debinterfaces.WordExpander
}

var _ = gc.Suite(&BridgeSuite{})

func (s *BridgeSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
}

func format(stanzas []debinterfaces.Stanza) string {
	return debinterfaces.FormatStanzas(stanzas, 4)
}

func (s *BridgeSuite) SetUpTest(c *gc.C) {
	s.expander = debinterfaces.NewWordExpander()
}

func (s *BridgeSuite) assertParse(c *gc.C, content string) []debinterfaces.Stanza {
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, gc.IsNil)
	return stanzas
}

func (s *BridgeSuite) checkBridge(input, expected string, c *gc.C, devices map[string]string) {
	stanzas := s.assertParse(c, input)
	bridged := debinterfaces.Bridge(stanzas, devices)
	c.Check(format(bridged), gc.Equals, expected)
	s.assertParse(c, format(bridged))
}

func (s *BridgeSuite) checkBridgeUnchanged(input string, c *gc.C, devices map[string]string) {
	stanzas := s.assertParse(c, input)
	bridged := debinterfaces.Bridge(stanzas, devices)
	c.Check(format(bridged), gc.Equals, input[1:])
	s.assertParse(c, format(bridged))
}

func (s *BridgeSuite) TestBridgeDeviceNameNotMatched(c *gc.C) {
	input := `
auto eth0
iface eth0 inet manual`
	s.checkBridgeUnchanged(input, c, map[string]string{"non-existent-interface": "br-non-existent"})
}

func (s *BridgeSuite) TestBridgeDeviceNameAlreadyBridged(c *gc.C) {
	input := `
auto br-eth0
iface br-eth0 inet dhcp
    bridge_ports eth0`
	s.checkBridgeUnchanged(input, c, map[string]string{"br-eth0": "br-eth0-2"})
}

func (s *BridgeSuite) TestBridgeDeviceIsBridgeable(c *gc.C) {
	input := `
auto eth0
iface eth0 inet dhcp`

	expected := `
auto eth0
iface eth0 inet manual

auto br-eth0
iface br-eth0 inet dhcp
    bridge_ports eth0`
	s.checkBridge(input, expected[1:], c, map[string]string{"eth0": "br-eth0"})
}

func (s *BridgeSuite) TestBridgeDeviceIsBridgeableButHasNoAutoStanza(c *gc.C) {
	input := `
iface eth0 inet dhcp`

	expected := `
iface eth0 inet manual

iface br-eth0 inet dhcp
    bridge_ports eth0`
	s.checkBridge(input, expected[1:], c, map[string]string{"eth0": "br-eth0"})
}

func (s *BridgeSuite) TestBridgeDeviceIsNotBridgeable(c *gc.C) {
	input := `
iface work-wireless bootp`
	s.checkBridgeUnchanged(input, c, map[string]string{"work-wireless": "br-work-wireless"})
}

func (s *BridgeSuite) TestBridgeSpecialOptionsGetMoved(c *gc.C) {
	input := `
auto eth0
iface eth0 inet static
    mtu 1500

auto eth1
iface eth1 inet static
    address 192.168.1.254
    gateway 192.168.1.1
    netmask 255.255.255.0
    dns-nameservers 8.8.8.8
    dns-search ubuntu.com
    dns-sortlist 192.168.1.1/24 10.245.168.0/21 192.168.1.0/24
    mtu 1500`

	expected := `
auto eth0
iface eth0 inet manual
    mtu 1500

auto eth1
iface eth1 inet manual
    mtu 1500

auto br-eth0
iface br-eth0 inet static
    bridge_ports eth0

auto br-eth1
iface br-eth1 inet static
    address 192.168.1.254
    gateway 192.168.1.1
    netmask 255.255.255.0
    dns-nameservers 8.8.8.8
    dns-search ubuntu.com
    dns-sortlist 192.168.1.1/24 10.245.168.0/21 192.168.1.0/24
    bridge_ports eth1`
	s.checkBridge(input, expected[1:], c, map[string]string{"eth0": "br-eth0", "eth1": "br-eth1"})
}

func (s *BridgeSuite) TestBridgeVLAN(c *gc.C) {
	input := `
auto eth0.2
iface eth0.2 inet static
    address 192.168.2.3/24
    vlan-raw-device eth0
    mtu 1500
    vlan_id 2`

	expected := `
auto eth0.2
iface eth0.2 inet manual
    vlan-raw-device eth0
    mtu 1500
    vlan_id 2

auto br-eth0.2
iface br-eth0.2 inet static
    address 192.168.2.3/24
    bridge_ports eth0.2`
	s.checkBridge(input, expected[1:], c, map[string]string{"eth0.2": "br-eth0.2"})
}

func (s *BridgeSuite) TestBridgeBond(c *gc.C) {
	input := `
auto eth0
iface eth0 inet manual
    bond-miimon 100
    bond-master bond0
    bond-mode active-backup
    bond-lacp-rate slow
    bond-xmit-hash-policy layer2
    mtu 1500

auto eth1
iface eth1 inet manual
    bond-miimon 100
    bond-master bond0
    bond-mode active-backup
    bond-lacp-rate slow
    bond-xmit-hash-policy layer2
    mtu 1500

auto bond0
iface bond0 inet static
    address 10.17.20.211/24
    gateway 10.17.20.1
    dns-nameservers 10.17.20.200
    bond-slaves eth0 eth1
    bond-mode active-backup
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    hwaddress 52:54:00:1c:f1:5b
    bond-lacp_rate slow`

	expected := `
auto eth0
iface eth0 inet manual
    bond-miimon 100
    bond-master bond0
    bond-mode active-backup
    bond-lacp-rate slow
    bond-xmit-hash-policy layer2
    mtu 1500

auto eth1
iface eth1 inet manual
    bond-miimon 100
    bond-master bond0
    bond-mode active-backup
    bond-lacp-rate slow
    bond-xmit-hash-policy layer2
    mtu 1500

auto bond0
iface bond0 inet manual
    bond-slaves eth0 eth1
    bond-mode active-backup
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    hwaddress 52:54:00:1c:f1:5b
    bond-lacp_rate slow

auto br-bond0
iface br-bond0 inet static
    address 10.17.20.211/24
    gateway 10.17.20.1
    dns-nameservers 10.17.20.200
    hwaddress 52:54:00:1c:f1:5b
    bridge_ports bond0`
	s.checkBridge(input, expected[1:], c, map[string]string{"bond0": "br-bond0"})
}

func (s *BridgeSuite) TestBridgingIdempotent(c *gc.C) {
	input := `
auto eth0
iface eth0 inet manual
    bond-miimon 100
    bond-master bond0
    bond-mode active-backup
    bond-lacp-rate slow
    bond-xmit-hash-policy layer2
    mtu 1500

auto bond0
iface bond0 inet manual
    bond-mode active-backup
    bond-xmit-hash-policy layer2
    hwaddress ether 7c:d3:0a:bb:e2:0a
    bond-slaves eth0
    mtu 1500
    bond-lacp-rate slow
    bond-miimon 100

auto br-bond0
iface br-bond0 inet static
    address 10.20.2.253/22
    gateway 10.20.0.1
    bridge_ports bond0`

	s.checkBridgeUnchanged(input, c, map[string]string{"bond0": "br-bond0", "bond0.1000": "br-bond0.1000", "bond0.1001": "br-bond0.1001", "bond0.1002": "br-bond0.1002"})
}

func (s *BridgeSuite) TestBridgeNoIfacesDefined(c *gc.C) {
	input := `
mapping eth0
    script /path/to/pcmcia-compat.sh
    map home,*,*,*                  home
    map work,*,*,00:11:22:33:44:55  work-wireless
    map work,*,*,01:12:23:34:45:50  work-static`
	s.checkBridgeUnchanged(input, c, map[string]string{"eth0": "br-eth0"})
}

func (s *BridgeSuite) TestBridgeBondMaster(c *gc.C) {
	input := `
auto ens5
iface ens5 inet manual
    bond-lacp_rate slow
    mtu 1500
    bond-master bond0
    bond-xmit_hash_policy layer2
    bond-mode active-backup
    bond-miimon 100`
	s.checkBridgeUnchanged(input, c, map[string]string{"ens5": "br-ens5"})
}

func (s *BridgeSuite) TestBridgeNoIfacesDefinedFromFile(c *gc.C) {
	stanzas, err := debinterfaces.ParseSource("testdata/ifupdown-examples", nil, s.expander)
	c.Assert(err, gc.IsNil)
	input := format(stanzas)
	s.checkBridge(input, input, c, map[string]string{"non-existent-interface": "non-existent-bridge"})
}

func (s *BridgeSuite) TestBridgeAlias(c *gc.C) {
	input := `
auto eth0
iface eth0 inet static
    gateway 10.14.0.1
    address 10.14.0.102/24

auto eth0:1
iface eth0:1 inet static
    address 1.2.3.5`

	expected := `
auto eth0
iface eth0 inet manual

auto br-eth0:1
iface br-eth0:1 inet static
    address 1.2.3.5

auto br-eth0
iface br-eth0 inet static
    gateway 10.14.0.1
    address 10.14.0.102/24
    bridge_ports eth0`
	s.checkBridge(input, expected[1:], c, map[string]string{"eth0": "br-eth0", "eth0:1": "br-eth0:1"})
}

func (s *BridgeSuite) TestBridgeMultipleInterfaces(c *gc.C) {
	input := `
auto enp1s0f3
iface enp1s0f3 inet static
  address 192.168.1.64/24
  gateway 192.168.1.254
  dns-nameservers 192.168.1.254
  dns-search home

iface enp1s0f3 inet6 dhcp`

	expected := `
auto enp1s0f3
iface enp1s0f3 inet manual

iface enp1s0f3 inet6 manual

auto br-enp1s0f3
iface br-enp1s0f3 inet static
    address 192.168.1.64/24
    gateway 192.168.1.254
    dns-nameservers 192.168.1.254
    dns-search home
    bridge_ports enp1s0f3

iface br-enp1s0f3 inet6 dhcp
    bridge_ports enp1s0f3`
	s.checkBridge(input, expected[1:], c, map[string]string{"enp1s0f3": "br-enp1s0f3"})
}

func (s *BridgeSuite) TestSourceStanzaWithRelativeFilenames(c *gc.C) {
	stanzas, err := debinterfaces.Parse("testdata/TestInputSourceStanza/interfaces")
	c.Assert(err, gc.IsNil)
	c.Assert(stanzas, gc.HasLen, 3)
	bridged := debinterfaces.Bridge(stanzas, map[string]string{"eth0": "br-eth0"})

	expected := `
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet manual

auto eth1
iface eth1 inet static
    address 192.168.1.64
    dns-nameservers 192.168.1.254

auto eth2
iface eth2 inet manual

auto br-eth0
iface br-eth0 inet dhcp
    bridge_ports eth0`

	c.Assert(debinterfaces.FormatStanzas(debinterfaces.FlattenStanzas(bridged), 4), gc.Equals, expected[1:])
}

func (s *BridgeSuite) TestSourceDirectoryStanzaWithRelativeFilenames(c *gc.C) {
	stanzas, err := debinterfaces.Parse("testdata/TestInputSourceDirectoryStanza/interfaces")
	c.Assert(err, gc.IsNil)
	c.Assert(stanzas, gc.HasLen, 3)

	bridged := debinterfaces.Bridge(stanzas, map[string]string{"eth3": "br-eth3"})

	expected := `
auto lo
iface lo inet loopback

auto eth3
iface eth3 inet manual

auto br-eth3
iface br-eth3 inet static
    address 192.168.1.128
    dns-nameservers 192.168.1.254
    bridge_ports eth3`

	c.Assert(debinterfaces.FormatStanzas(debinterfaces.FlattenStanzas(bridged), 4), gc.Equals, expected[1:])
}

func (s *BridgeSuite) TestBridgeInet6Only(c *gc.C) {
	input := `
auto enxe0db55e41d5b
iface enxe0db55e41d5b inet6 static
    address 3ffe:1234:5678::1/64
    gateway 3ffe:1234:5678::2
`
	expected := `
auto enxe0db55e41d5b
iface enxe0db55e41d5b inet6 manual

auto br-xe0db55e41d5b
iface br-xe0db55e41d5b inet6 static
    address 3ffe:1234:5678::1/64
    gateway 3ffe:1234:5678::2
    bridge_ports enxe0db55e41d5b`

	s.checkBridge(input, expected[1:], c, map[string]string{"enxe0db55e41d5b": "br-xe0db55e41d5b"})
}
