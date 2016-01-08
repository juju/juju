// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type bridgeConfigSuite struct {
	coretesting.BaseSuite

	testConfig       string
	testConfigPath   string
	testPythonScript string
}

var _ = gc.Suite(&bridgeConfigSuite{})

func (s *bridgeConfigSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping bridge config tests on windows")
	}
	s.BaseSuite.SetUpSuite(c)
}

func (s *bridgeConfigSuite) SetUpTest(c *gc.C) {
	s.testConfigPath = filepath.Join(c.MkDir(), "network-config")
	s.testPythonScript = filepath.Join(c.MkDir(), bridgeScriptName)
	s.testConfig = "# test network config\n"
	err := ioutil.WriteFile(s.testConfigPath, []byte(s.testConfig), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(s.testPythonScript, []byte(bridgeScriptPython), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bridgeConfigSuite) assertScript(c *gc.C, initialConfig, expectedConfig, bridgePrefix string) {
	// To simplify most cases, trim trailing new lines.
	initialConfig = strings.TrimSuffix(initialConfig, "\n")
	expectedConfig = strings.TrimSuffix(expectedConfig, "\n")
	err := ioutil.WriteFile(s.testConfigPath, []byte(initialConfig), 0644)
	c.Check(err, jc.ErrorIsNil)
	// Run the script and verify the modified config.
	output, retcode := s.runScript(c, s.testConfigPath, bridgePrefix)
	c.Check(retcode, gc.Equals, 0)
	c.Check(strings.Trim(output, "\n"), gc.Equals, expectedConfig)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithUndefinedArgs(c *gc.C) {
	_, code := s.runScript(c, "", "")
	c.Check(code, gc.Equals, 1)
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCP(c *gc.C) {
	s.assertScript(c, networkDHCPInitial, networkDHCPExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptStatic(c *gc.C) {
	s.assertScript(c, networkStaticInitial, networkStaticExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptDualNIC(c *gc.C) {
	s.assertScript(c, networkDualNICInitial, networkDualNICExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptWithAlias(c *gc.C) {
	s.assertScript(c, networkWithAliasInitial, networkWithAliasExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCPWithAlias(c *gc.C) {
	s.assertScript(c, networkDHCPWithAliasInitial, networkDHCPWithAliasExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptMultipleStaticWithAliases(c *gc.C) {
	s.assertScript(c, networkMultipleStaticWithAliasesInitial, networkMultipleStaticWithAliasesExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCPWithBond(c *gc.C) {
	s.assertScript(c, networkDHCPWithBondInitial, networkDHCPWithBondExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptMultipleAliases(c *gc.C) {
	s.assertScript(c, networkMultipleAliasesInitial, networkMultipleAliasesExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptSmorgasboard(c *gc.C) {
	s.assertScript(c, networkSmorgasboardInitial, networkSmorgasboardExpected, "juju-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptWithVLANs(c *gc.C) {
	s.assertScript(c, networkVLANInitial, networkVLANExpected, "vlan-br-")
}

func (s *bridgeConfigSuite) runScript(c *gc.C, configFile string, bridgePrefix string) (output string, exitCode int) {
	script := fmt.Sprintf("%q --bridge-prefix=%q %q\n",
		s.testPythonScript,
		bridgePrefix,
		configFile)

	result, err := exec.RunCommands(exec.RunParams{Commands: script})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("script failed unexpectedly"))
	stdout := string(result.Stdout)
	stderr := string(result.Stderr)
	if stderr != "" {
		return stdout + "\n" + stderr, result.Code
	}
	return stdout, result.Code
}

// The rest of the file contains various forms of network config for
// both before and after it has been run through the python script.
// They are used in individual test functions.

const networkStaticInitial = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1`

const networkStaticExpected = `auto lo
iface lo inet loopback

iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
    bridge_ports eth0`

const networkDHCPInitial = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp`

const networkDHCPExpected = `auto lo
iface lo inet loopback

iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet dhcp
    bridge_ports eth0`

const networkDualNICInitial = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1

auto eth1
iface eth1 inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 4.3.2.1`

const networkDualNICExpected = `auto lo
iface lo inet loopback

iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
    bridge_ports eth0

iface eth1 inet manual

auto test-br-eth1
iface test-br-eth1 inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 4.3.2.1
    bridge_ports eth1`

const networkWithAliasInitial = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1

auto eth0:1
iface eth0:1 inet static
    address 1.2.3.5`

const networkWithAliasExpected = `auto lo
iface lo inet loopback

iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
    bridge_ports eth0

auto eth0:1
iface eth0:1 inet static
    address 1.2.3.5`

const networkDHCPWithAliasInitial = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    gateway 10.14.0.1
    address 10.14.0.102/24

auto eth0:1
iface eth0:1 inet static
    address 10.14.0.103/24

auto eth0:2
iface eth0:2 inet static
    address 10.14.0.100/24

dns-nameserver 192.168.1.142`

const networkDHCPWithAliasExpected = `auto lo
iface lo inet loopback

iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet static
    gateway 10.14.0.1
    address 10.14.0.102/24
    bridge_ports eth0

auto eth0:1
iface eth0:1 inet static
    address 10.14.0.103/24

auto eth0:2
iface eth0:2 inet static
    address 10.14.0.100/24
    dns-nameserver 192.168.1.142`

const networkMultipleStaticWithAliasesInitial = `
auto eth0
iface eth0 inet static
    gateway 10.17.20.1
    address 10.17.20.201/24
    mtu 1500

auto eth0:1
iface eth0:1 inet static
    address 10.17.20.202/24
    mtu 1500

auto eth1
iface eth1 inet manual
    mtu 1500

dns-nameservers 10.17.20.200
dns-search maas`

const networkMultipleStaticWithAliasesExpected = `iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet static
    gateway 10.17.20.1
    address 10.17.20.201/24
    mtu 1500
    bridge_ports eth0

auto eth0:1
iface eth0:1 inet static
    address 10.17.20.202/24
    mtu 1500

auto eth1
iface eth1 inet manual
    mtu 1500
    dns-nameservers 10.17.20.200
    dns-search maas`

const networkDHCPWithBondInitial = `auto eth0
iface eth0 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode active-backup

auto eth1
iface eth1 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode active-backup

auto bond0
iface bond0 inet dhcp
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-mode active-backup
    hwaddress 52:54:00:1c:f1:5b
    bond-slaves none

dns-nameservers 10.17.20.200
dns-search maas19`

const networkDHCPWithBondExpected = `auto eth0
iface eth0 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode active-backup

auto eth1
iface eth1 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode active-backup

auto bond0
iface bond0 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-mode active-backup
    hwaddress 52:54:00:1c:f1:5b
    bond-slaves none
    dns-nameservers 10.17.20.200
    dns-search maas19

auto test-br-bond0
iface test-br-bond0 inet dhcp
    mtu 1500
    hwaddress 52:54:00:1c:f1:5b
    dns-nameservers 10.17.20.200
    dns-search maas19
    bridge_ports bond0`

const networkMultipleAliasesInitial = `auto eth0
iface eth0 inet dhcp

auto eth1
iface eth1 inet dhcp

auto eth10
iface eth10 inet static
    gateway 10.17.20.1
    address 10.17.20.201/24
    mtu 1500

auto eth10:1
iface eth10:1 inet static
    address 10.17.20.202/24
    mtu 1500

auto eth10:2
iface eth10:2 inet static
    address 10.17.20.203/24
    mtu 1500

dns-nameservers 10.17.20.200
dns-search maas19`

const networkMultipleAliasesExpected = `iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet dhcp
    bridge_ports eth0

iface eth1 inet manual

auto test-br-eth1
iface test-br-eth1 inet dhcp
    bridge_ports eth1

iface eth10 inet manual

auto test-br-eth10
iface test-br-eth10 inet static
    gateway 10.17.20.1
    address 10.17.20.201/24
    mtu 1500
    bridge_ports eth10

auto eth10:1
iface eth10:1 inet static
    address 10.17.20.202/24
    mtu 1500

auto eth10:2
iface eth10:2 inet static
    address 10.17.20.203/24
    mtu 1500
    dns-nameservers 10.17.20.200
    dns-search maas19`

const networkSmorgasboardInitial = `auto eth0
iface eth0 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode active-backup

auto eth1
iface eth1 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode active-backup

auto eth2
iface eth2 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond1
    mtu 1500
    bond-mode active-backup

auto eth3
iface eth3 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond1
    mtu 1500
    bond-mode active-backup

auto eth4
iface eth4 inet static
    address 10.17.20.202/24
    mtu 1500

auto eth5
iface eth5 inet dhcp
    mtu 1500

auto eth6
iface eth6 inet static
    address 10.17.20.203/24
    mtu 1500

auto eth6:1
iface eth6:1 inet static
    address 10.17.20.205/24
    mtu 1500

auto eth6:2
iface eth6:2 inet static
    address 10.17.20.204/24
    mtu 1500

auto eth6:3
iface eth6:3 inet static
    address 10.17.20.206/24
    mtu 1500

auto eth6:4
iface eth6:4 inet static
    address 10.17.20.207/24
    mtu 1500

auto bond0
iface bond0 inet static
    gateway 10.17.20.1
    address 10.17.20.201/24
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-mode active-backup
    hwaddress 52:54:00:6a:4f:fd
    bond-slaves none

auto bond1
iface bond1 inet dhcp
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-mode active-backup
    hwaddress 52:54:00:8e:6e:b0
    bond-slaves none

dns-nameservers 10.17.20.200
dns-search maas19`

const networkSmorgasboardExpected = `auto eth0
iface eth0 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode active-backup

auto eth1
iface eth1 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode active-backup

auto eth2
iface eth2 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond1
    mtu 1500
    bond-mode active-backup

auto eth3
iface eth3 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond1
    mtu 1500
    bond-mode active-backup

iface eth4 inet manual

auto juju-br-eth4
iface juju-br-eth4 inet static
    address 10.17.20.202/24
    mtu 1500
    bridge_ports eth4

iface eth5 inet manual

auto juju-br-eth5
iface juju-br-eth5 inet dhcp
    mtu 1500
    bridge_ports eth5

iface eth6 inet manual

auto juju-br-eth6
iface juju-br-eth6 inet static
    address 10.17.20.203/24
    mtu 1500
    bridge_ports eth6

auto eth6:1
iface eth6:1 inet static
    address 10.17.20.205/24
    mtu 1500

auto eth6:2
iface eth6:2 inet static
    address 10.17.20.204/24
    mtu 1500

auto eth6:3
iface eth6:3 inet static
    address 10.17.20.206/24
    mtu 1500

auto eth6:4
iface eth6:4 inet static
    address 10.17.20.207/24
    mtu 1500

auto bond0
iface bond0 inet manual
    gateway 10.17.20.1
    address 10.17.20.201/24
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-mode active-backup
    hwaddress 52:54:00:6a:4f:fd
    bond-slaves none

auto juju-br-bond0
iface juju-br-bond0 inet static
    gateway 10.17.20.1
    address 10.17.20.201/24
    mtu 1500
    hwaddress 52:54:00:6a:4f:fd
    bridge_ports bond0

auto bond1
iface bond1 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-mode active-backup
    hwaddress 52:54:00:8e:6e:b0
    bond-slaves none
    dns-nameservers 10.17.20.200
    dns-search maas19

auto juju-br-bond1
iface juju-br-bond1 inet dhcp
    mtu 1500
    hwaddress 52:54:00:8e:6e:b0
    dns-nameservers 10.17.20.200
    dns-search maas19
    bridge_ports bond1`

const networkVLANInitial = `auto eth0
iface eth0 inet static
    gateway 10.17.20.1
    address 10.17.20.212/24
    mtu 1500

auto eth1
iface eth1 inet manual
    mtu 1500

auto eth0.2
iface eth0.2 inet static
    address 192.168.2.3/24
    vlan-raw-device eth0
    mtu 1500
    vlan_id 2

auto eth1.3
iface eth1.3 inet static
    address 192.168.3.3/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 3

dns-nameservers 10.17.20.200
dns-search maas19`

const networkVLANExpected = `iface eth0 inet manual

auto vlan-br-eth0
iface vlan-br-eth0 inet static
    gateway 10.17.20.1
    address 10.17.20.212/24
    mtu 1500
    bridge_ports eth0

auto eth1
iface eth1 inet manual
    mtu 1500

iface eth0.2 inet manual
    address 192.168.2.3/24
    vlan-raw-device eth0
    mtu 1500
    vlan_id 2

auto vlan-br-eth0.2
iface vlan-br-eth0.2 inet static
    address 192.168.2.3/24
    mtu 1500
    bridge_ports eth0.2

auto eth1.3
iface eth1.3 inet static
    address 192.168.3.3/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 3
    dns-nameservers 10.17.20.200
    dns-search maas19`
