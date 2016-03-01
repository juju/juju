// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"io/ioutil"
	"os"
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
	pythonVersions   []string
}

var _ = gc.Suite(&bridgeConfigSuite{})

func (s *bridgeConfigSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping bridge config tests on windows")
	}
	s.BaseSuite.SetUpSuite(c)

	for _, version := range []string{
		"/usr/bin/python2",
		"/usr/bin/python3",
		"/usr/bin/python",
	} {
		if _, err := os.Stat(version); err == nil {
			s.pythonVersions = append(s.pythonVersions, version)
		}
	}
}

func (s *bridgeConfigSuite) SetUpTest(c *gc.C) {
	// We need at least one Python package installed.
	c.Assert(s.pythonVersions, gc.Not(gc.HasLen), 0)

	s.testConfigPath = filepath.Join(c.MkDir(), "network-config")
	s.testPythonScript = filepath.Join(c.MkDir(), bridgeScriptName)
	s.testConfig = "# test network config\n"
	err := ioutil.WriteFile(s.testConfigPath, []byte(s.testConfig), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(s.testPythonScript, []byte(bridgeScriptPython), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bridgeConfigSuite) assertScript(c *gc.C, initialConfig, expectedConfig, bridgePrefix, bridgeName, interfaceToBridge string) {
	for i, python := range s.pythonVersions {
		c.Logf("test #%v using %s", i, python)
		// To simplify most cases, trim trailing new lines.
		initialConfig = strings.TrimSuffix(initialConfig, "\n")
		expectedConfig = strings.TrimSuffix(expectedConfig, "\n")
		err := ioutil.WriteFile(s.testConfigPath, []byte(initialConfig), 0644)
		c.Check(err, jc.ErrorIsNil)
		// Run the script and verify the modified config.
		output, retcode := s.runScript(c, python, s.testConfigPath, bridgePrefix, bridgeName, interfaceToBridge)
		c.Check(retcode, gc.Equals, 0)
		c.Check(strings.Trim(output, "\n"), gc.Equals, expectedConfig)
	}
}

func (s *bridgeConfigSuite) assertScriptWithPrefix(c *gc.C, initial, expected, prefix string) {
	s.assertScript(c, initial, expected, prefix, "", "")
}

func (s *bridgeConfigSuite) assertScriptWithDefaultPrefix(c *gc.C, initial, expected string) {
	s.assertScript(c, initial, expected, "", "", "")
}

func (s *bridgeConfigSuite) assertScriptWithoutPrefix(c *gc.C, initial, expected, bridgeName, interfaceToBridge string) {
	s.assertScript(c, initial, expected, "", bridgeName, interfaceToBridge)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithUndefinedArgs(c *gc.C) {
	for i, python := range s.pythonVersions {
		c.Logf("test #%v using %s", i, python)
		_, code := s.runScript(c, python, "", "", "", "")
		c.Check(code, gc.Equals, 1)
	}
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCP(c *gc.C) {
	s.assertScriptWithPrefix(c, networkDHCPInitial, networkDHCPExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptStatic(c *gc.C) {
	s.assertScriptWithPrefix(c, networkStaticInitial, networkStaticExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptDualNIC(c *gc.C) {
	s.assertScriptWithPrefix(c, networkDualNICInitial, networkDualNICExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptWithAlias(c *gc.C) {
	s.assertScriptWithPrefix(c, networkWithAliasInitial, networkWithAliasExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCPWithAlias(c *gc.C) {
	s.assertScriptWithPrefix(c, networkDHCPWithAliasInitial, networkDHCPWithAliasExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptMultipleStaticWithAliases(c *gc.C) {
	s.assertScriptWithPrefix(c, networkMultipleStaticWithAliasesInitial, networkMultipleStaticWithAliasesExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCPWithBond(c *gc.C) {
	s.assertScriptWithPrefix(c, networkDHCPWithBondInitial, networkDHCPWithBondExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptMultipleAliases(c *gc.C) {
	s.assertScriptWithPrefix(c, networkMultipleAliasesInitial, networkMultipleAliasesExpected, "test-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptSmorgasboard(c *gc.C) {
	s.assertScriptWithPrefix(c, networkSmorgasboardInitial, networkSmorgasboardExpected, "juju-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptWithVLANs(c *gc.C) {
	s.assertScriptWithPrefix(c, networkVLANInitial, networkVLANExpected, "vlan-br-")
}

func (s *bridgeConfigSuite) TestBridgeScriptWithMultipleNameservers(c *gc.C) {
	s.assertScriptWithDefaultPrefix(c, networkVLANWithMultipleNameserversInitial, networkVLANWithMultipleNameserversExpected)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithLoopbackOnly(c *gc.C) {
	s.assertScriptWithDefaultPrefix(c, networkLoopbackOnlyInitial, networkLoopbackOnlyExpected)
}

func (s *bridgeConfigSuite) TestBridgeScriptBondWithVLANs(c *gc.C) {
	s.assertScriptWithDefaultPrefix(c, networkStaticBondWithVLANsInitial, networkStaticBondWithVLANsExpected)
}

func (s *bridgeConfigSuite) TestBridgeScriptVLANWithInactive(c *gc.C) {
	s.assertScriptWithDefaultPrefix(c, networkVLANWithInactiveDeviceInitial, networkVLANWithInactiveDeviceExpected)
}

func (s *bridgeConfigSuite) TestBridgeScriptVLANWithActiveDHCPDevice(c *gc.C) {
	s.assertScriptWithDefaultPrefix(c, networkVLANWithActiveDHCPDeviceInitial, networkVLANWithActiveDHCPDeviceExpected)
}

func (s *bridgeConfigSuite) TestBridgeScriptMultipleDNSValues(c *gc.C) {
	s.assertScriptWithDefaultPrefix(c, networkWithMultipleDNSValuesInitial, networkWithMultipleDNSValuesExpected)
}

func (s *bridgeConfigSuite) TestBridgeScriptEmptyDNSValues(c *gc.C) {
	s.assertScriptWithDefaultPrefix(c, networkWithEmptyDNSValuesInitial, networkWithEmptyDNSValuesExpected)
}

func (s *bridgeConfigSuite) TestBridgeScriptMismatchedBridgeNameAndInterfaceArgs(c *gc.C) {
	s.assertScriptWithDefaultPrefix(c, networkWithEmptyDNSValuesInitial, networkWithEmptyDNSValuesExpected)
}

func (s *bridgeConfigSuite) TestBridgeScriptInterfaceNameArgumentRequired(c *gc.C) {
	for i, python := range s.pythonVersions {
		c.Logf("test #%v using %s", i, python)
		output, code := s.runScript(c, python, "# no content", "", "juju-br0", "")
		c.Check(code, gc.Equals, 1)
		c.Check(strings.Trim(output, "\n"), gc.Equals, "error: --interface-to-bridge required when using --bridge-name")
	}
}

func (s *bridgeConfigSuite) TestBridgeScriptBridgeNameArgumentRequired(c *gc.C) {
	for i, python := range s.pythonVersions {
		c.Logf("test #%v using %s", i, python)
		output, code := s.runScript(c, python, "# no content", "", "", "eth0")
		c.Check(code, gc.Equals, 1)
		c.Check(strings.Trim(output, "\n"), gc.Equals, "error: --bridge-name required when using --interface-to-bridge")
	}
}

func (s *bridgeConfigSuite) TestBridgeScriptMatchingNonExistentSpecificIface(c *gc.C) {
	s.assertScriptWithoutPrefix(c, networkStaticInitial, networkStaticInitial, "juju-br0", "eth1234567890")
}

func (s *bridgeConfigSuite) TestBridgeScriptMatchingExistingSpecificIfaceButMissingAutoStanza(c *gc.C) {
	s.assertScriptWithoutPrefix(c, networkWithExistingSpecificIfaceInitial, networkWithExistingSpecificIfaceExpected, "juju-br0", "eth1")
}

func (s *bridgeConfigSuite) TestBridgeScriptMatchingExistingSpecificIface2(c *gc.C) {
	s.assertScriptWithoutPrefix(c, networkLP1532167Initial, networkLP1532167Expected, "juju-br0", "bond0")
}

func (s *bridgeConfigSuite) runScript(c *gc.C, pythonBinary, configFile, bridgePrefix, bridgeName, interfaceToBridge string) (output string, exitCode int) {
	if bridgePrefix != "" {
		bridgePrefix = fmt.Sprintf("--bridge-prefix=%q", bridgePrefix)
	}

	if bridgeName != "" {
		bridgeName = fmt.Sprintf("--bridge-name=%q", bridgeName)
	}

	if interfaceToBridge != "" {
		interfaceToBridge = fmt.Sprintf("--interface-to-bridge=%q", interfaceToBridge)
	}

	script := fmt.Sprintf("%q %q %s %s %s %q\n", pythonBinary, s.testPythonScript, bridgePrefix, bridgeName, interfaceToBridge, configFile)
	c.Log(script)
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
    bridge_ports bond0
    dns-nameservers 10.17.20.200
    dns-search maas19`

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
    bridge_ports bond1
    dns-nameservers 10.17.20.200
    dns-search maas19`

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

iface eth1.3 inet manual
    address 192.168.3.3/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 3
    dns-nameservers 10.17.20.200
    dns-search maas19

auto vlan-br-eth1.3
iface vlan-br-eth1.3 inet static
    address 192.168.3.3/24
    mtu 1500
    bridge_ports eth1.3
    dns-nameservers 10.17.20.200
    dns-search maas19`

const networkVLANWithMultipleNameserversInitial = `auto eth0
iface eth0 inet static
    dns-nameservers 10.245.168.2
    gateway 10.245.168.1
    address 10.245.168.11/21
    mtu 1500

auto eth1
iface eth1 inet manual
    mtu 1500

auto eth2
iface eth2 inet manual
    mtu 1500

auto eth3
iface eth3 inet manual
    mtu 1500

auto eth1.2667
iface eth1.2667 inet static
    dns-nameservers 10.245.168.2
    address 10.245.184.2/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2667

auto eth1.2668
iface eth1.2668 inet static
    dns-nameservers 10.245.168.2
    address 10.245.185.1/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2668

auto eth1.2669
iface eth1.2669 inet static
    dns-nameservers 10.245.168.2
    address 10.245.186.1/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2669

auto eth1.2670
iface eth1.2670 inet static
    dns-nameservers 10.245.168.2
    address 10.245.187.2/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2670

dns-nameservers 10.245.168.2
dns-search dellstack`

const networkVLANWithMultipleNameserversExpected = `iface eth0 inet manual

auto br-eth0
iface br-eth0 inet static
    gateway 10.245.168.1
    address 10.245.168.11/21
    mtu 1500
    bridge_ports eth0
    dns-nameservers 10.245.168.2

auto eth1
iface eth1 inet manual
    mtu 1500

auto eth2
iface eth2 inet manual
    mtu 1500

auto eth3
iface eth3 inet manual
    mtu 1500

iface eth1.2667 inet manual
    address 10.245.184.2/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2667
    dns-nameservers 10.245.168.2

auto br-eth1.2667
iface br-eth1.2667 inet static
    address 10.245.184.2/24
    mtu 1500
    bridge_ports eth1.2667
    dns-nameservers 10.245.168.2

iface eth1.2668 inet manual
    address 10.245.185.1/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2668
    dns-nameservers 10.245.168.2

auto br-eth1.2668
iface br-eth1.2668 inet static
    address 10.245.185.1/24
    mtu 1500
    bridge_ports eth1.2668
    dns-nameservers 10.245.168.2

iface eth1.2669 inet manual
    address 10.245.186.1/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2669
    dns-nameservers 10.245.168.2

auto br-eth1.2669
iface br-eth1.2669 inet static
    address 10.245.186.1/24
    mtu 1500
    bridge_ports eth1.2669
    dns-nameservers 10.245.168.2

iface eth1.2670 inet manual
    address 10.245.187.2/24
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2670
    dns-nameservers 10.245.168.2
    dns-search dellstack

auto br-eth1.2670
iface br-eth1.2670 inet static
    address 10.245.187.2/24
    mtu 1500
    bridge_ports eth1.2670
    dns-nameservers 10.245.168.2
    dns-search dellstack`

const networkLoopbackOnlyInitial = `auto lo
iface lo inet loopback`

const networkLoopbackOnlyExpected = `auto lo
iface lo inet loopback`

const networkStaticBondWithVLANsInitial = `auto eth0
iface eth0 inet manual
    bond-master bond0
    bond-mode active-backup
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-lacp_rate slow

auto eth1
iface eth1 inet manual
    bond-master bond0
    bond-mode active-backup
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-lacp_rate slow

auto bond0
iface bond0 inet static
    address 10.17.20.211/24
    gateway 10.17.20.1
    dns-nameservers 10.17.20.200
    bond-slaves none
    bond-mode active-backup
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    hwaddress 52:54:00:1c:f1:5b
    bond-lacp_rate slow

auto bond0.2
iface bond0.2 inet static
    address 192.168.2.102/24
    vlan-raw-device bond0
    mtu 1500
    vlan_id 2

auto bond0.3
iface bond0.3 inet static
    address 192.168.3.101/24
    vlan-raw-device bond0
    mtu 1500
    vlan_id 3

dns-nameservers 10.17.20.200
dns-search maas19`

const networkStaticBondWithVLANsExpected = `auto eth0
iface eth0 inet manual
    bond-master bond0
    bond-mode active-backup
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-lacp_rate slow

auto eth1
iface eth1 inet manual
    bond-master bond0
    bond-mode active-backup
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-lacp_rate slow

auto bond0
iface bond0 inet manual
    address 10.17.20.211/24
    gateway 10.17.20.1
    bond-slaves none
    bond-mode active-backup
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    hwaddress 52:54:00:1c:f1:5b
    bond-lacp_rate slow
    dns-nameservers 10.17.20.200

auto br-bond0
iface br-bond0 inet static
    address 10.17.20.211/24
    gateway 10.17.20.1
    mtu 1500
    hwaddress 52:54:00:1c:f1:5b
    bridge_ports bond0
    dns-nameservers 10.17.20.200

iface bond0.2 inet manual
    address 192.168.2.102/24
    vlan-raw-device bond0
    mtu 1500
    vlan_id 2

auto br-bond0.2
iface br-bond0.2 inet static
    address 192.168.2.102/24
    mtu 1500
    bridge_ports bond0.2

iface bond0.3 inet manual
    address 192.168.3.101/24
    vlan-raw-device bond0
    mtu 1500
    vlan_id 3
    dns-nameservers 10.17.20.200
    dns-search maas19

auto br-bond0.3
iface br-bond0.3 inet static
    address 192.168.3.101/24
    mtu 1500
    bridge_ports bond0.3
    dns-nameservers 10.17.20.200
    dns-search maas19`

const networkVLANWithInactiveDeviceInitial = `auto eth0
iface eth0 inet static
    dns-nameservers 10.17.20.200
    gateway 10.17.20.1
    address 10.17.20.211/24
    mtu 1500

auto eth1
iface eth1 inet manual
    mtu 1500

auto eth1.2
iface eth1.2 inet dhcp
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2

dns-nameservers 10.17.20.200
dns-search maas19
`

const networkVLANWithInactiveDeviceExpected = `iface eth0 inet manual

auto br-eth0
iface br-eth0 inet static
    gateway 10.17.20.1
    address 10.17.20.211/24
    mtu 1500
    bridge_ports eth0
    dns-nameservers 10.17.20.200

auto eth1
iface eth1 inet manual
    mtu 1500

iface eth1.2 inet manual
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2
    dns-nameservers 10.17.20.200
    dns-search maas19

auto br-eth1.2
iface br-eth1.2 inet dhcp
    mtu 1500
    bridge_ports eth1.2
    dns-nameservers 10.17.20.200
    dns-search maas19`

const networkVLANWithActiveDHCPDeviceInitial = `auto eth0
iface eth0 inet static
    dns-nameservers 10.17.20.200
    gateway 10.17.20.1
    address 10.17.20.211/24
    mtu 1500

auto eth1
iface eth1 inet dhcp
    mtu 1500

auto eth1.2
iface eth1.2 inet dhcp
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2

dns-nameservers 10.17.20.200
dns-search maas19
`

const networkVLANWithActiveDHCPDeviceExpected = `iface eth0 inet manual

auto br-eth0
iface br-eth0 inet static
    gateway 10.17.20.1
    address 10.17.20.211/24
    mtu 1500
    bridge_ports eth0
    dns-nameservers 10.17.20.200

iface eth1 inet manual

auto br-eth1
iface br-eth1 inet dhcp
    mtu 1500
    bridge_ports eth1

iface eth1.2 inet manual
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2
    dns-nameservers 10.17.20.200
    dns-search maas19

auto br-eth1.2
iface br-eth1.2 inet dhcp
    mtu 1500
    bridge_ports eth1.2
    dns-nameservers 10.17.20.200
    dns-search maas19`

const networkWithMultipleDNSValuesInitial = `auto eth0
iface eth0 inet static
    dns-nameservers 10.245.168.2
    gateway 10.245.168.1
    address 10.245.168.11/21
    mtu 1500
    dns-nameservers 192.168.1.1
    dns-nameservers 10.245.168.2 192.168.1.1 10.245.168.2

auto eth1
iface eth1 inet static
    gateway 10.245.168.1
    address 10.245.168.12/21
    mtu 1500
    dns-sortlist 192.168.1.0/24 10.245.168.0/21 192.168.1.0/24
    dns-sortlist 10.245.168.0/21 192.168.1.0/24

auto eth2
iface eth2 inet static
    gateway 10.245.168.1
    address 10.245.168.13/21
    mtu 1500
    dns-search juju ubuntu
    dns-search dellstack ubuntu dellstack

auto eth3
iface eth3 inet static
    gateway 10.245.168.1
    address 10.245.168.14/21
    mtu 1500
    dns-nameservers 192.168.1.1
    dns-nameservers 10.245.168.2 192.168.1.1 10.245.168.2
    dns-sortlist 192.168.1.0/24 10.245.168.0/21 192.168.1.0/24
    dns-sortlist 10.245.168.0/21 192.168.1.0/24
    dns-search juju ubuntu
    dns-search dellstack ubuntu dellstack

dns-search ubuntu juju
dns-search dellstack ubuntu dellstack`

const networkWithMultipleDNSValuesExpected = `iface eth0 inet manual

auto br-eth0
iface br-eth0 inet static
    gateway 10.245.168.1
    address 10.245.168.11/21
    mtu 1500
    bridge_ports eth0
    dns-nameservers 10.245.168.2 192.168.1.1

iface eth1 inet manual

auto br-eth1
iface br-eth1 inet static
    gateway 10.245.168.1
    address 10.245.168.12/21
    mtu 1500
    bridge_ports eth1
    dns-sortlist 192.168.1.0/24 10.245.168.0/21

iface eth2 inet manual

auto br-eth2
iface br-eth2 inet static
    gateway 10.245.168.1
    address 10.245.168.13/21
    mtu 1500
    bridge_ports eth2
    dns-search juju ubuntu dellstack

iface eth3 inet manual

auto br-eth3
iface br-eth3 inet static
    gateway 10.245.168.1
    address 10.245.168.14/21
    mtu 1500
    bridge_ports eth3
    dns-nameservers 192.168.1.1 10.245.168.2
    dns-search juju ubuntu dellstack
    dns-sortlist 192.168.1.0/24 10.245.168.0/21`

const networkWithEmptyDNSValuesInitial = `auto eth0
iface eth0 inet static
    dns-nameservers
    dns-search
    dns-sortlist
    gateway 10.245.168.1
    address 10.245.168.11/21
    mtu 1500

auto eth1
iface eth1 inet static
    dns-nameservers
    dns-search
    dns-sortlist
    gateway 10.245.168.1
    address 10.245.168.12/21
    mtu 1500

dns-nameservers
dns-search
dns-sortlist`

const networkWithEmptyDNSValuesExpected = `iface eth0 inet manual

auto br-eth0
iface br-eth0 inet static
    gateway 10.245.168.1
    address 10.245.168.11/21
    mtu 1500
    bridge_ports eth0

iface eth1 inet manual

auto br-eth1
iface br-eth1 inet static
    gateway 10.245.168.1
    address 10.245.168.12/21
    mtu 1500
    bridge_ports eth1`

const networkLP1532167Initial = `auto eth0
iface eth0 inet manual
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode 802.3ad

auto eth1
iface eth1 inet manual
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode 802.3ad

auto eth2
iface eth2 inet manual
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    bond-master bond1
    mtu 1500
    bond-mode 802.3ad

auto eth3
iface eth3 inet manual
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    bond-master bond1
    mtu 1500
    bond-mode 802.3ad

auto bond0
iface bond0 inet static
    gateway 10.38.160.1
    address 10.38.160.24/24
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    mtu 1500
    bond-mode 802.3ad
    hwaddress 44:a8:42:41:ab:37
    bond-slaves none

auto bond1
iface bond1 inet manual
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    mtu 1500
    bond-mode 802.3ad
    hwaddress 00:0e:1e:b7:b5:50
    bond-slaves none

auto bond0.1016
iface bond0.1016 inet static
    address 172.16.0.21/16
    vlan-raw-device bond0
    mtu 1500
    vlan_id 1016

auto bond0.161
iface bond0.161 inet static
    address 10.38.161.21/24
    vlan-raw-device bond0
    mtu 1500
    vlan_id 161

auto bond0.162
iface bond0.162 inet static
    address 10.38.162.21/24
    vlan-raw-device bond0
    mtu 1500
    vlan_id 162

auto bond0.163
iface bond0.163 inet static
    address 10.38.163.21/24
    vlan-raw-device bond0
    mtu 1500
    vlan_id 163

auto bond1.1017
iface bond1.1017 inet static
    address 172.17.0.21/16
    vlan-raw-device bond1
    mtu 1500
    vlan_id 1017

auto bond1.1018
iface bond1.1018 inet static
    address 172.18.0.21/16
    vlan-raw-device bond1
    mtu 1500
    vlan_id 1018

auto bond1.1019
iface bond1.1019 inet static
    address 172.19.0.21/16
    vlan-raw-device bond1
    mtu 1500
    vlan_id 1019

dns-nameservers 10.38.160.10
dns-search maas`

const networkLP1532167Expected = `auto eth0
iface eth0 inet manual
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode 802.3ad

auto eth1
iface eth1 inet manual
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode 802.3ad

auto eth2
iface eth2 inet manual
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    bond-master bond1
    mtu 1500
    bond-mode 802.3ad

auto eth3
iface eth3 inet manual
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    bond-master bond1
    mtu 1500
    bond-mode 802.3ad

auto bond0
iface bond0 inet manual
    gateway 10.38.160.1
    address 10.38.160.24/24
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    mtu 1500
    bond-mode 802.3ad
    hwaddress 44:a8:42:41:ab:37
    bond-slaves none

auto juju-br0
iface juju-br0 inet static
    gateway 10.38.160.1
    address 10.38.160.24/24
    mtu 1500
    hwaddress 44:a8:42:41:ab:37
    bridge_ports bond0

auto bond1
iface bond1 inet manual
    bond-lacp_rate fast
    bond-xmit_hash_policy layer2+3
    bond-miimon 100
    mtu 1500
    bond-mode 802.3ad
    hwaddress 00:0e:1e:b7:b5:50
    bond-slaves none

auto bond0.1016
iface bond0.1016 inet static
    address 172.16.0.21/16
    vlan-raw-device bond0
    mtu 1500
    vlan_id 1016

auto bond0.161
iface bond0.161 inet static
    address 10.38.161.21/24
    vlan-raw-device bond0
    mtu 1500
    vlan_id 161

auto bond0.162
iface bond0.162 inet static
    address 10.38.162.21/24
    vlan-raw-device bond0
    mtu 1500
    vlan_id 162

auto bond0.163
iface bond0.163 inet static
    address 10.38.163.21/24
    vlan-raw-device bond0
    mtu 1500
    vlan_id 163

auto bond1.1017
iface bond1.1017 inet static
    address 172.17.0.21/16
    vlan-raw-device bond1
    mtu 1500
    vlan_id 1017

auto bond1.1018
iface bond1.1018 inet static
    address 172.18.0.21/16
    vlan-raw-device bond1
    mtu 1500
    vlan_id 1018

auto bond1.1019
iface bond1.1019 inet static
    address 172.19.0.21/16
    vlan-raw-device bond1
    mtu 1500
    vlan_id 1019
    dns-nameservers 10.38.160.10
    dns-search maas`

const networkWithExistingSpecificIfaceInitial = `auto lo
iface lo inet loopback

# Note this has no auto stanza
iface eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1

iface eth1 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1`

const networkWithExistingSpecificIfaceExpected = `auto lo
iface lo inet loopback

iface eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1

iface eth1 inet manual

auto juju-br0
iface juju-br0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
    bridge_ports eth1`
