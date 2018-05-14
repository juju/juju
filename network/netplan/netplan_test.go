package netplan_test

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network/netplan"
	coretesting "github.com/juju/juju/testing"
)

type NetplanSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&NetplanSuite{})

func checkNetplanRoundTrips(c *gc.C, input string) {
	if strings.HasPrefix(input, "\n") {
		input = input[1:]
	}
	var np netplan.Netplan
	err := netplan.Unmarshal([]byte(input), &np)
	c.Assert(err, jc.ErrorIsNil)
	out, err := netplan.Marshal(np)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(out), gc.Equals, input)
}

func (s *NetplanSuite) TestStructures(c *gc.C) {
	checkNetplanRoundTrips(c, `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
      wakeonlan: true
      addresses:
      - 192.168.14.2/24
      - 2001:1::1/64
      critical: true
      dhcp4: true
      dhcp-identifier: mac
      gateway4: 192.168.14.1
      gateway6: 2001:1::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
      routes:
      - to: 0.0.0.0/0
        via: 11.0.0.1
        metric: 3
    lom:
      match:
        driver: ixgbe
      set-name: lom1
      dhcp6: true
    switchports:
      match:
        name: enp2*
      mtu: 1280
  wifis:
    all-wlans:
      access-points:
        Joe's home:
          password: s3kr1t
    wlp1s0:
      access-points:
        guest:
          mode: ap
          channel: 11
  bridges:
    br0:
      interfaces: [wlp1s0, switchports]
      dhcp4: false
  routes:
  - to: 0.0.0.0/0
    via: 11.0.0.1
    metric: 3
`)
}

func (s *NetplanSuite) TestBasicBond(c *gc.C) {
	checkNetplanRoundTrips(c, `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
        set-name: id0
    id1:
      match:
        macaddress: de:ad:be:ef:01:02
        set-name: id1
  bridges:
    br-bond0:
      interfaces: [bond0]
      dhcp4: true
  bonds:
    bond0:
      interfaces:
      - id0
      - id1
      parameters:
        mode: 802.3ad
        lacp-rate: fast
        mii-monitor-interval: 100
        transmit-hash-policy: layer2
        up-delay: 0
        down-delay: 0
`)
}

func (s *NetplanSuite) TestParseBridgedBond(c *gc.C) {
	checkNetplanRoundTrips(c, `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
        set-name: id0
    id1:
      match:
        macaddress: de:ad:be:ef:01:02
        set-name: id1
  bridges:
    br-bond0:
      interfaces: [bond0]
      dhcp4: true
  bonds:
    bond0:
      interfaces:
      - id0
      - id1
      parameters:
        mode: 802.3ad
        lacp-rate: fast
        mii-monitor-interval: 100
        transmit-hash-policy: layer2
        up-delay: 0
        down-delay: 0
`)
}

func (s *NetplanSuite) TestBondIntValues(c *gc.C) {
	// several parameters can be specified as an integer or a string
	// such as 'mode: 0' is the same as 'balance-rr'
	checkNetplanRoundTrips(c, `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
        set-name: id0
    id1:
      match:
        macaddress: de:ad:be:ef:01:02
        set-name: id1
  bonds:
    bond0:
      interfaces:
      - id0
      - id1
      parameters:
        mode: 0
        lacp-rate: 1
        ad-select: 1
        arp-validate: 0
        arp-all-targets: 0
        fail-over-mac-policy: 1
        primary-reselect-policy: 1
`)
	checkNetplanRoundTrips(c, `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
        set-name: id0
    id1:
      match:
        macaddress: de:ad:be:ef:01:02
        set-name: id1
  bonds:
    bond0:
      interfaces:
      - id0
      - id1
      parameters:
        mode: balance-rr
        lacp-rate: fast
        ad-select: bandwidth
        arp-validate: filter
        arp-all-targets: all
        fail-over-mac-policy: follow
        primary-reselect-policy: always
`)
}

func (s *NetplanSuite) TestBondWithVLAN(c *gc.C) {
	checkNetplanRoundTrips(c, `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
        set-name: id0
    id1:
      match:
        macaddress: de:ad:be:ef:01:02
        set-name: id1
  bonds:
    bond0:
      interfaces:
      - id0
      - id1
      parameters:
        mode: 802.3ad
        lacp-rate: fast
        mii-monitor-interval: 100
        transmit-hash-policy: layer2
        up-delay: 0
        down-delay: 0
  vlans:
    bond0.209:
      id: 209
      link: bond0
      addresses:
      - 123.123.123.123/24
      nameservers:
        addresses: [8.8.8.8]
`)
}

func (s *NetplanSuite) TestBondsAllParameters(c *gc.C) {
	// All parameters don't inherently make sense at the same time, but we should be able to parse all of them.
	checkNetplanRoundTrips(c, `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
        set-name: id0
    id1:
      match:
        macaddress: de:ad:be:ef:01:02
        set-name: id1
  bonds:
    bond0:
      interfaces:
      - id0
      - id1
      parameters:
        mode: 802.3ad
        lacp-rate: fast
        mii-monitor-interval: 100
        min-links: 0
        transmit-hash-policy: layer2
        ad-select: 1
        all-slaves-active: true
        arp-interval: 100
        arp-ip-targets:
        - 192.168.0.1
        - 192.168.10.20
        arp-validate: none
        arp-all-targets: boo
        up-delay: 0
        down-delay: 0
        fail-over-mac-policy: follow
        gratuitious-arp: 0
        packets-per-slave: 0
        primary-reselect-policy: better
        resend-igmp: 0
        learn-packet-interval: 4660
        primary: id1
`)
}

func (s *NetplanSuite) TestAllEthernetParams(c *gc.C) {
	// Make sure we can handle any fields in Ethernet stanzas
}

func (s *NetplanSuite) TestAllRoutesParams(c *gc.C) {
	checkNetplanRoundTrips(c, `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
        set-name: id0
      routes:
      - from: 192.168.0.0/24
        on-link: true
        scope: global
        table: 1234
        to: 192.168.3.1/24
        type: unicast
        via: 192.168.3.1
        metric: 1234567
      - on-link: false
        to: 192.168.5.1/24
        via: 192.168.5.1
        metric: 0
      - to: 192.168.5.1/24
        type: unreachable
        via: 192.168.5.1
      routing-policy:
      - from: 192.168.10.0/24
        mark: 123
        priority: 10
        table: 1234
        to: 192.168.3.1/24
        type-of-service: 0
      - from: 192.168.12.0/24
        mark: 0
        priority: 0
        table: 0
        to: 192.168.3.1/24
        type-of-service: 255
`)
}

func (s *NetplanSuite) TestAllVLANParams(c *gc.C) {
	checkNetplanRoundTrips(c, `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
        set-name: id0
  vlans:
    id0.123:
      id: 123
      link: id0
      accept-ra: true
      addresses:
      - 123.123.123.123/24
      critical: true
      dhcp4: false
      dhcp6: false
      dhcp-identifier: duid
      gateway4: 123.123.123.123
      gateway6: dead::beef
      nameservers:
        addresses: [8.8.8.8]
      macaddress: de:ad:be:ef:12:34
      mtu: 9000
      renderer: NetworkManager
      routes:
      - table: 102
        to: 100.0.0.0/8
        via: 1.2.3.10
        metric: 5
      routing-policy:
      - from: 192.168.5.0/24
        table: 103
      optional: true
    id0.456:
      id: 456
      link: id0
      accept-ra: false
`)
}

func (s *NetplanSuite) TestSimpleBridger(c *gc.C) {
	input := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
      routes:
      - to: 100.0.0.0/8
        via: 1.2.3.10
        metric: 5
`[1:]
	expected := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
  bridges:
    juju-bridge:
      interfaces: [id0]
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
      routes:
      - to: 100.0.0.0/8
        via: 1.2.3.10
        metric: 5
`[1:]
	var np netplan.Netplan

	err := netplan.Unmarshal([]byte(input), &np)
	c.Assert(err, jc.ErrorIsNil)

	err = np.BridgeEthernetById("id0", "juju-bridge")
	c.Assert(err, jc.ErrorIsNil)

	out, err := netplan.Marshal(np)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(out), gc.Equals, expected)
}

func (s *NetplanSuite) TestBridgerIdempotent(c *gc.C) {
	input := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
  bridges:
    juju-bridge:
      interfaces: [id0]
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
      routes:
      - to: 100.0.0.0/8
        via: 1.2.3.10
        metric: 5
`[1:]
	var np netplan.Netplan
	err := netplan.Unmarshal([]byte(input), &np)
	c.Assert(err, jc.ErrorIsNil)
	err = np.BridgeEthernetById("id0", "juju-bridge")
	c.Assert(err, jc.ErrorIsNil)
	out, err := netplan.Marshal(np)
	c.Check(string(out), gc.Equals, input)
}

func (s *NetplanSuite) TestBridgerBridgeExists(c *gc.C) {
	input := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
    id1:
      match:
        driver: ixgbe
  bridges:
    juju-bridge:
      interfaces: [id1]
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
`[1:]
	var np netplan.Netplan
	err := netplan.Unmarshal([]byte(input), &np)
	c.Assert(err, jc.ErrorIsNil)
	err = np.BridgeEthernetById("id0", "juju-bridge")
	c.Check(err, gc.ErrorMatches, `Cannot bridge device "id0" on bridge "juju-bridge" - bridge named "juju-bridge" already exists`)
}

func (s *NetplanSuite) TestBridgerDeviceBridged(c *gc.C) {
	input := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
  bridges:
    not-juju-bridge:
      interfaces: [id0]
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
`[1:]
	var np netplan.Netplan
	err := netplan.Unmarshal([]byte(input), &np)
	c.Assert(err, jc.ErrorIsNil)
	err = np.BridgeEthernetById("id0", "juju-bridge")
	c.Check(err, gc.ErrorMatches, `.*Device "id0" is already bridged in bridge "not-juju-bridge" instead of "juju-bridge".*`)
}

func (s *NetplanSuite) TestBridgerDeviceMissing(c *gc.C) {
	input := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
  bridges:
    not-juju-bridge:
      interfaces: [id0]
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
`[1:]
	var np netplan.Netplan
	err := netplan.Unmarshal([]byte(input), &np)
	c.Assert(err, jc.ErrorIsNil)
	err = np.BridgeEthernetById("id7", "juju-bridge")
	c.Check(err, gc.ErrorMatches, `Device with id "id7" for bridge "juju-bridge" not found`)
}

func (s *NetplanSuite) TestFindEthernetBySetName(c *gc.C) {
	input := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      set-name: eno1
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
    id1:
      match:
        macaddress: "00:11:22:33:44:66"
        name: en*3
      addresses:
      - 1.2.4.4/24
      - 2001::1/64
      gateway4: 1.2.4.5
      gateway6: 2001::2
      nameservers:
        search: [baz.local]
        addresses: [8.8.4.4]
`[1:]
	var np netplan.Netplan
	err := netplan.Unmarshal([]byte(input), &np)
	c.Assert(err, jc.ErrorIsNil)

	device, err := np.FindEthernetByName("eno1")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(device, gc.Equals, "id0")

	device, err = np.FindEthernetByName("eno3")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(device, gc.Equals, "id1")

	device, err = np.FindEthernetByName("eno5")
	c.Check(err, gc.ErrorMatches, "Ethernet device with name \"eno5\" not found")
}

func (s *NetplanSuite) TestFindEthernetByMAC(c *gc.C) {
	input := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      set-name: eno1
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
    id1:
      match:
        macaddress: "00:11:22:33:44:66"
      addresses:
      - 1.2.4.4/24
      - 2001::1/64
      gateway4: 1.2.4.5
      gateway6: 2001::2
      nameservers:
        search: [baz.local]
        addresses: [8.8.4.4]
`[1:]
	var np netplan.Netplan
	err := netplan.Unmarshal([]byte(input), &np)
	c.Assert(err, jc.ErrorIsNil)

	device, err := np.FindEthernetByMAC("00:11:22:33:44:66")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(device, gc.Equals, "id1")

	device, err = np.FindEthernetByMAC("00:11:22:33:44:88")
	c.Check(err, gc.ErrorMatches, "Ethernet device with mac \"00:11:22:33:44:88\" not found")
}

func (s *NetplanSuite) TestReadDirectory(c *gc.C) {
	c.Skip("Full netplan merge not supported yet, see https://bugs.launchpad.net/juju/+bug/1701429")
	expected := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
      set-name: eno1
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.8
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
    id1:
      match:
        macaddress: "00:11:22:33:44:66"
      addresses:
      - 1.2.4.4/24
      - 2001::1/64
      gateway4: 1.2.4.5
      gateway6: 2001::2
      nameservers:
        search: [baz.local]
        addresses: [8.8.4.4]
    id2:
      match:
        driver: iwldvm
  bridges:
    some-bridge:
      interfaces: [id2]
      addresses:
      - 1.5.6.7/24
`[1:]
	np, err := netplan.ReadDirectory("testdata/TestReadDirectory")
	c.Assert(err, jc.ErrorIsNil)

	out, err := netplan.Marshal(np)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(out), gc.Equals, expected)
}

// TODO(wpk) 2017-06-14 This test checks broken behaviour, it should be removed when TestReadDirectory passes.
// see https://bugs.launchpad.net/juju/+bug/1701429
func (s *NetplanSuite) TestReadDirectoryWithoutProperMerge(c *gc.C) {
	expected := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      gateway4: 1.2.3.8
    id1:
      match:
        macaddress: 00:11:22:33:44:66
      addresses:
      - 1.2.4.4/24
      - 2001::1/64
      gateway4: 1.2.4.5
      gateway6: 2001::2
      nameservers:
        search: [baz.local]
        addresses: [8.8.4.4]
    id2:
      match:
        driver: iwldvm
      set-name: eno3
  bridges:
    some-bridge:
      interfaces: [id2]
      addresses:
      - 1.5.6.7/24
`[1:]
	np, err := netplan.ReadDirectory("testdata/TestReadDirectory")
	c.Assert(err, jc.ErrorIsNil)

	out, err := netplan.Marshal(np)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(out), gc.Equals, expected)
}

func (s *NetplanSuite) TestReadWriteBackupRollback(c *gc.C) {
	expected := `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
    id0:
      match:
        macaddress: "00:11:22:33:44:55"
      set-name: eno1
    id1:
      match:
        macaddress: 00:11:22:33:44:66
      addresses:
      - 1.2.4.4/24
      - 2001::1/64
      gateway4: 1.2.4.5
      gateway6: 2001::2
      nameservers:
        search: [baz.local]
        addresses: [8.8.4.4]
    id2:
      match:
        driver: iwldvm
  bridges:
    juju-bridge:
      interfaces: [id0]
      addresses:
      - 1.2.3.4/24
      - 2000::1/64
      gateway4: 1.2.3.5
      gateway6: 2000::2
      nameservers:
        search: [foo.local, bar.local]
        addresses: [8.8.8.8]
    some-bridge:
      interfaces: [id2]
      addresses:
      - 1.5.6.7/24
`[1:]
	tempDir := c.MkDir()
	files := []string{"00.yaml", "01.yaml"}
	contents := make([][]byte, len(files))
	for i, file := range files {
		var err error
		contents[i], err = ioutil.ReadFile(path.Join("testdata/TestReadWriteBackup", file))
		c.Assert(err, jc.ErrorIsNil)
		err = ioutil.WriteFile(path.Join(tempDir, file), contents[i], 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	np, err := netplan.ReadDirectory(tempDir)
	c.Assert(err, jc.ErrorIsNil)

	err = np.BridgeEthernetById("id0", "juju-bridge")
	c.Assert(err, jc.ErrorIsNil)

	generatedFile, err := np.Write("")
	c.Assert(err, jc.ErrorIsNil)

	_, err = np.Write("")
	c.Check(err, gc.ErrorMatches, "Cannot write the same netplan twice")

	err = np.MoveYamlsToBak()
	c.Assert(err, jc.ErrorIsNil)

	err = np.MoveYamlsToBak()
	c.Check(err, gc.ErrorMatches, "Cannot backup netplan yamls twice")

	fileInfos, err := ioutil.ReadDir(tempDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(fileInfos, gc.HasLen, len(files)+1)
	for _, fileInfo := range fileInfos {
		for i, fileName := range files {
			// original file is moved to backup
			c.Check(fileInfo.Name(), gc.Not(gc.Equals), fileName)
			// backup file has the proper content
			if strings.HasPrefix(fileInfo.Name(), fmt.Sprintf("%s.bak.", fileName)) {
				data, err := ioutil.ReadFile(path.Join(tempDir, fileInfo.Name()))
				c.Assert(err, jc.ErrorIsNil)
				c.Check(data, gc.DeepEquals, contents[i])
			}
		}
	}

	data, err := ioutil.ReadFile(generatedFile)
	c.Check(string(data), gc.Equals, expected)

	err = np.Rollback()
	c.Assert(err, jc.ErrorIsNil)

	fileInfos, err = ioutil.ReadDir(tempDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(fileInfos, gc.HasLen, len(files))
	foundFiles := 0
	for _, fileInfo := range fileInfos {
		for i, fileName := range files {
			if fileInfo.Name() == fileName {
				data, err := ioutil.ReadFile(path.Join(tempDir, fileInfo.Name()))
				c.Assert(err, jc.ErrorIsNil)
				c.Check(data, gc.DeepEquals, contents[i])
				foundFiles++
			}
		}
	}
	c.Check(foundFiles, gc.Equals, len(files))

	// After rollback we should be able to write and move yamls to backup again
	// We also check if writing to an explicit file works
	myPath := path.Join(tempDir, "my-own-path.yaml")
	outPath, err := np.Write(myPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(outPath, gc.Equals, myPath)
	data, err = ioutil.ReadFile(outPath)
	c.Check(string(data), gc.Equals, expected)

	err = np.MoveYamlsToBak()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NetplanSuite) TestReadDirectoryMissing(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1771077")
	// On Windows the error is something like: "The system cannot find the file specified"
	tempDir := c.MkDir()
	os.RemoveAll(tempDir)
	_, err := netplan.ReadDirectory(tempDir)
	c.Check(err, gc.ErrorMatches, "open .* no such file or directory")
}

func (s *NetplanSuite) TestReadDirectoryAccessDenied(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1771077")
	tempDir := c.MkDir()
	err := ioutil.WriteFile(path.Join(tempDir, "00-file.yaml"), []byte("network:\n"), 00000)
	_, err = netplan.ReadDirectory(tempDir)
	c.Check(err, gc.ErrorMatches, "open .*/00-file.yaml: permission denied")
}

func (s *NetplanSuite) TestReadDirectoryBrokenYaml(c *gc.C) {
	tempDir := c.MkDir()
	err := ioutil.WriteFile(path.Join(tempDir, "00-file.yaml"), []byte("I am not a yaml file!\nreally!\n"), 0644)
	_, err = netplan.ReadDirectory(tempDir)
	c.Check(err, gc.ErrorMatches, "yaml: unmarshal errors:\n.*")
}

func (s *NetplanSuite) TestWritePermissionDenied(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1771077")
	tempDir := c.MkDir()
	np, err := netplan.ReadDirectory(tempDir)
	c.Assert(err, jc.ErrorIsNil)
	os.Chmod(tempDir, 00000)
	_, err = np.Write(path.Join(tempDir, "99-juju-netplan.yaml"))
	c.Check(err, gc.ErrorMatches, "open .* permission denied")
}

func (s *NetplanSuite) TestWriteCantGenerateName(c *gc.C) {
	tempDir := c.MkDir()
	for i := 0; i < 100; i++ {
		filePath := path.Join(tempDir, fmt.Sprintf("%0.2d-juju.yaml", i))
		ioutil.WriteFile(filePath, []byte{}, 0644)
	}
	np, err := netplan.ReadDirectory(tempDir)
	c.Assert(err, jc.ErrorIsNil)
	_, err = np.Write("")
	c.Check(err, gc.ErrorMatches, "Can't generate a filename for netplan YAML")
}

func (s *NetplanSuite) TestProperReadingOrder(c *gc.C) {
	var header = `
network:
  version: 2
  renderer: NetworkManager
  ethernets:
`[1:]
	var template = `
    id%d:
      set-name: foo.%d.%d
`[1:]
	tempDir := c.MkDir()

	for _, n := range rand.Perm(100) {
		content := header
		for i := 0; i < (100 - n); i++ {
			content += fmt.Sprintf(template, i, i, n)
		}
		ioutil.WriteFile(path.Join(tempDir, fmt.Sprintf("%0.2d-test.yaml", n)), []byte(content), 0644)
	}

	np, err := netplan.ReadDirectory(tempDir)
	c.Assert(err, jc.ErrorIsNil)

	fileName, err := np.Write("")

	writtenContent, err := ioutil.ReadFile(fileName)
	c.Assert(err, jc.ErrorIsNil)

	content := header
	for n := 0; n < 100; n++ {
		content += fmt.Sprintf(template, n, n, 100-n-1)
	}
	c.Check(string(writtenContent), gc.Equals, content)
}

type Example struct {
	filename string
	content  string
}

func readExampleStrings(c *gc.C) []Example {
	fileInfos, err := ioutil.ReadDir("testdata/examples")
	c.Assert(err, jc.ErrorIsNil)
	var examples []Example
	for _, finfo := range fileInfos {
		if finfo.IsDir() {
			continue
		}
		if strings.HasSuffix(finfo.Name(), ".yaml") {
			f, err := os.Open("testdata/examples/" + finfo.Name())
			c.Assert(err, jc.ErrorIsNil)
			content, err := ioutil.ReadAll(f)
			f.Close()
			c.Assert(err, jc.ErrorIsNil)
			examples = append(examples, Example{
				filename: finfo.Name(),
				content:  string(content),
			})
		}
	}
	// Make sure we find all the example files, if we change the count, update this number, but we don't allow the test
	// suite to find the wrong number of files.
	c.Assert(len(examples), gc.Equals, 13)
	return examples
}

func (s *NetplanSuite) TestNetplanExamples(c *gc.C) {
	// these are the examples shipped by netplan, we should be able to read all of them
	examples := readExampleStrings(c)
	for _, example := range examples {
		c.Logf("example: %s", example.filename)
		var orig map[interface{}]interface{}
		err := netplan.Unmarshal([]byte(example.content), &orig)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("failed to unmarshal as map %s", example.filename))
		var np netplan.Netplan
		err = netplan.Unmarshal([]byte(example.content), &np)
		c.Check(err, jc.ErrorIsNil, gc.Commentf("failed to unmarshal %s", example.filename))
		// We don't assert that we exactly match the serialized form (we may output fields in a different order),
		// but we do check that if we Marshal and then Unmarshal again, we get the same map contents.
		// (We might also change boolean 'no' to 'false', etc.
		out, err := netplan.Marshal(np)
		c.Check(err, jc.ErrorIsNil, gc.Commentf("failed to marshal %s", example.filename))
		var roundtripped map[interface{}]interface{}
		err = netplan.Unmarshal(out, &roundtripped)
		if !reflect.DeepEqual(orig, roundtripped) {
			pretty.Ldiff(c, orig, roundtripped)
			c.Errorf("marshalling and unmarshalling %s did not contain the same content", example.filename)
		}
	}
}
