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

	testConfig     string
	testConfigPath string
	testBridgeName string
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
	s.testConfig = "# test network config\n"
	s.testBridgeName = "test-bridge"
	err := ioutil.WriteFile(s.testConfigPath, []byte(s.testConfig), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bridgeConfigSuite) assertScript(c *gc.C, initialConfig, expectedConfig, nic, bridge string, isBond bool) {
	// To simplify most cases, trim trailing new lines.
	initialConfig = strings.TrimSuffix(initialConfig, "\n")
	expectedConfig = strings.TrimSuffix(expectedConfig, "\n")
	err := ioutil.WriteFile(s.testConfigPath, []byte(initialConfig), 0644)
	c.Check(err, jc.ErrorIsNil)
	// Run the script and verify the modified config.
	output, retcode := s.runScript(c, s.testConfigPath, nic, bridge, isBond)
	c.Check(retcode, gc.Equals, 0)
	c.Check(strings.Trim(output, "\n"), gc.Equals, expectedConfig)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithUndefinedArgs(c *gc.C) {
	_, code := s.runScript(c, "", "", "", false)
	c.Check(code, gc.Equals, 1)
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCP(c *gc.C) {
	s.assertScript(c, networkDHCPInitial, networkDHCPExpected, "eth0", "juju-br0", false)
}

func (s *bridgeConfigSuite) TestBridgeScriptStatic(c *gc.C) {
	s.assertScript(c, networkStaticInitial, networkStaticExpected, "eth0", "juju-br0", false)
}

func (s *bridgeConfigSuite) TestBridgeScriptMultiple(c *gc.C) {
	s.assertScript(c, networkMultipleInitial, networkMultipleExpected, "eth0", "juju-br0", false)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithAlias(c *gc.C) {
	s.assertScript(c, networkWithAliasInitial, networkWithAliasExpected, "eth0", "juju-br0", false)
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCPWithAlias(c *gc.C) {
	s.assertScript(c, networkDHCPWithAliasInitial, networkDHCPWithAliasExpected, "eth0", "juju-br0", false)
}

func (s *bridgeConfigSuite) TestBridgeScriptMultipleStaticWithAliases(c *gc.C) {
	s.assertScript(c, networkMultipleStaticWithAliasesInitial, networkMultipleStaticWithAliasesExpected, "eth0", "juju-br0", false)
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCPWithBond(c *gc.C) {
	s.assertScript(c, networkDHCPWithBondInitial, networkDHCPWithBondExpected, "bond0", "juju-br0", true)
}

func (s *bridgeConfigSuite) TestBridgeScriptMultipleAliases(c *gc.C) {
	s.assertScript(c, networkMultipleAliasesInitial, networkMultipleAliasesExpected, "eth10", "juju-br10", false)
}

func (s *bridgeConfigSuite) TestBridgeScriptPopEmptyStanza(c *gc.C) {
	s.assertScript(c, networkMinimalInitial, networkMinimalExpected, "eth0", "juju-br0", false)
}

func (s *bridgeConfigSuite) runScript(c *gc.C, configFile string, nic string, bridge string, isBond bool) (output string, exitCode int) {
	var primaryNicIsBonded = ""

	if isBond {
		primaryNicIsBonded = "--primary-nic-is-bonded"
	}

	script := fmt.Sprintf("%s\npython -c %q --render-only --filename=%q --primary-nic=%q --bridge-name=%q %s\n",
		bridgeScriptPythonBashDef,
		"$python_script",
		configFile,
		nic,
		bridge,
		primaryNicIsBonded)

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
// both before and after it has been run through the
// modify_network_config bash function. They are used in individual
// test functions.

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

auto juju-br0
iface juju-br0 inet static
    bridge_ports eth0
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1`

const networkDHCPInitial = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp`

const networkDHCPExpected = `auto lo
iface lo inet loopback

iface eth0 inet manual

auto juju-br0
iface juju-br0 inet dhcp
    bridge_ports eth0`

const networkMultipleInitial = networkStaticInitial + `
auto eth1
iface eth1 inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 4.3.2.1`

const networkMultipleExpected = `auto lo
iface lo inet loopback

iface eth0 inet manual

auto juju-br0
iface juju-br0 inet static
    bridge_ports eth0
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1

auto eth1
iface eth1 inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 4.3.2.1`

const networkWithAliasInitial = networkStaticInitial + `
auto eth0:1
iface eth0:1 inet static
    address 1.2.3.5`

const networkWithAliasExpected = `auto lo
iface lo inet loopback

iface eth0 inet manual

auto juju-br0
iface juju-br0 inet static
    bridge_ports eth0
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1

auto juju-br0:1
iface juju-br0:1 inet static
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

auto juju-br0
iface juju-br0 inet static
    bridge_ports eth0
    gateway 10.14.0.1
    address 10.14.0.102/24

auto juju-br0:1
iface juju-br0:1 inet static
    address 10.14.0.103/24

auto juju-br0:2
iface juju-br0:2 inet static
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

auto juju-br0
iface juju-br0 inet static
    bridge_ports eth0
    gateway 10.17.20.1
    address 10.17.20.201/24
    mtu 1500

auto juju-br0:1
iface juju-br0:1 inet static
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

auto juju-br0
iface juju-br0 inet dhcp
    bridge_ports bond0
    mtu 1500
    hwaddress 52:54:00:1c:f1:5b
    pre-up ip link add dev bond0 name juju-br0 type bridge || true

dns-nameservers 10.17.20.200
dns-search maas19`

const networkMultipleAliasesInitial = `auto eth0
iface eth0 inet dhcp

auto eth1
iface eth1 inet dhcp

auto eth2
iface eth2 inet dhcp

auto eth3
iface eth3 inet dhcp

auto eth4
iface eth4 inet dhcp

auto eth5
iface eth5 inet dhcp

auto eth5
iface eth5 inet dhcp

auto eth6
iface eth6 inet dhcp

auto eth7
iface eth7 inet dhcp

auto eth8
iface eth8 inet dhcp

auto eth9
iface eth9 inet dhcp

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

auto eth10:3
iface eth10:3 inet static
    address 10.17.20.204/24
    mtu 1500

auto eth10:4
iface eth10:4 inet static
    address 10.17.20.205/24
    mtu 1500

auto eth10:5
iface eth10:5 inet static
    address 10.17.20.206/24
    mtu 1500

auto eth10:6
iface eth10:6 inet static
    address 10.17.20.207/24
    mtu 1500

auto eth10:7
iface eth10:7 inet static
    address 10.17.20.208/24
    mtu 1500

auto eth10:8
iface eth10:8 inet static
    address 10.17.20.209/24
    mtu 1500

auto eth10:9
iface eth10:9 inet static
    address 10.17.20.210/24
    mtu 1500

auto eth10:10
iface eth10:10 inet static
    address 10.17.20.211/24
    mtu 1500

auto eth10:11
iface eth10:11 inet static
    address 10.17.20.212/24
    mtu 1500

auto eth10:12
iface eth10:12 inet static
    address 10.17.20.213/24
    mtu 1500

dns-nameservers 10.17.20.200
dns-search maas19`

const networkMultipleAliasesExpected = `auto eth0
iface eth0 inet dhcp

auto eth1
iface eth1 inet dhcp

auto eth2
iface eth2 inet dhcp

auto eth3
iface eth3 inet dhcp

auto eth4
iface eth4 inet dhcp

auto eth5
iface eth5 inet dhcp

auto eth5
iface eth5 inet dhcp

auto eth6
iface eth6 inet dhcp

auto eth7
iface eth7 inet dhcp

auto eth8
iface eth8 inet dhcp

auto eth9
iface eth9 inet dhcp

iface eth10 inet manual

auto juju-br10
iface juju-br10 inet static
    bridge_ports eth10
    gateway 10.17.20.1
    address 10.17.20.201/24
    mtu 1500

auto juju-br10:1
iface juju-br10:1 inet static
    address 10.17.20.202/24
    mtu 1500

auto juju-br10:2
iface juju-br10:2 inet static
    address 10.17.20.203/24
    mtu 1500

auto juju-br10:3
iface juju-br10:3 inet static
    address 10.17.20.204/24
    mtu 1500

auto juju-br10:4
iface juju-br10:4 inet static
    address 10.17.20.205/24
    mtu 1500

auto juju-br10:5
iface juju-br10:5 inet static
    address 10.17.20.206/24
    mtu 1500

auto juju-br10:6
iface juju-br10:6 inet static
    address 10.17.20.207/24
    mtu 1500

auto juju-br10:7
iface juju-br10:7 inet static
    address 10.17.20.208/24
    mtu 1500

auto juju-br10:8
iface juju-br10:8 inet static
    address 10.17.20.209/24
    mtu 1500

auto juju-br10:9
iface juju-br10:9 inet static
    address 10.17.20.210/24
    mtu 1500

auto juju-br10:10
iface juju-br10:10 inet static
    address 10.17.20.211/24
    mtu 1500

auto juju-br10:11
iface juju-br10:11 inet static
    address 10.17.20.212/24
    mtu 1500

auto juju-br10:12
iface juju-br10:12 inet static
    address 10.17.20.213/24
    mtu 1500

dns-nameservers 10.17.20.200
dns-search maas19`

const networkMinimalInitial = `auto eth0
iface eth0 inet dhcp`

const networkMinimalExpected = `iface eth0 inet manual

auto juju-br0
iface juju-br0 inet dhcp
    bridge_ports eth0`
