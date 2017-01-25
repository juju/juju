// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

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

	"github.com/juju/juju/network"
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
	s.testPythonScript = filepath.Join(c.MkDir(), "add-bridge.py")
	s.testConfig = "# test network config\n"
	err := ioutil.WriteFile(s.testConfigPath, []byte(s.testConfig), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(s.testPythonScript, []byte(network.BridgeScriptPythonContent), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bridgeConfigSuite) assertScript(c *gc.C, initialConfig, expectedConfig, bridgePrefix, bridgeName, interfaceToBridge string) {
	for i, python := range s.pythonVersions {
		c.Logf("test #%v using %s", i, python)
		// To simplify most cases, trim trailing new lines.
		expectedConfig = strings.TrimSuffix(expectedConfig, "\n")
		err := ioutil.WriteFile(s.testConfigPath, []byte(strings.TrimSuffix(initialConfig, "\n")), 0644)
		c.Check(err, jc.ErrorIsNil)
		// Run the script and verify the modified config.
		output, retcode := s.runScriptWithoutActivation(c, python, s.testConfigPath, bridgePrefix, bridgeName, interfaceToBridge)
		c.Check(retcode, gc.Equals, 0)
		c.Check(strings.Trim(output, "\n"), gc.Equals, expectedConfig, gc.Commentf("initial was:\n%s", initialConfig))
	}
}

func (s *bridgeConfigSuite) assertScriptWithActivationAndDryRun(c *gc.C, isBonded, isAlreadyBridged bool, initialConfig, bridgePrefix string, interfacesToBridge []string) {
	expectedConfig := s.dryRunExpectedOutputHelper(isBonded, isAlreadyBridged, bridgePrefix, interfacesToBridge)

	for i, python := range s.pythonVersions {
		c.Logf("test #%v using %s", i, python)
		err := ioutil.WriteFile(s.testConfigPath, []byte(strings.TrimSuffix(initialConfig, "\n")), 0644)
		c.Check(err, jc.ErrorIsNil)
		output, retcode := s.runScriptWithActivationAndDryRun(c, python, bridgePrefix, interfacesToBridge)
		c.Check(retcode, gc.Equals, 0)
		c.Check(len(output), gc.Equals, len(expectedConfig))
		c.Check(output, gc.DeepEquals, expectedConfig)
	}
}

func (s *bridgeConfigSuite) assertScriptWithPrefix(c *gc.C, initial, expected, prefix, interfaceToBridge string) {
	s.assertScript(c, initial, expected, prefix, "", interfaceToBridge)
}

func (s *bridgeConfigSuite) assertScriptWithDefaultPrefix(c *gc.C, initial, expected, interfaceToBridge string) {
	s.assertScript(c, initial, expected, "", "", interfaceToBridge)
}

func (s *bridgeConfigSuite) assertScriptWithoutPrefix(c *gc.C, initial, expected, bridgeName, interfaceToBridge string) {
	s.assertScript(c, initial, expected, "", bridgeName, interfaceToBridge)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithUndefinedArgs(c *gc.C) {
	for i, python := range s.pythonVersions {
		c.Logf("test #%v using %s", i, python)
		_, code := s.runScriptWithoutActivation(c, python, "", "", "", "")
		c.Check(code, gc.Equals, 2)
	}
}

func (s *bridgeConfigSuite) TestBridgeScriptWithPrefixTransformation(c *gc.C) {
	for i, v := range []struct {
		interfaceToBridge string
		initial           string
		expected          string
		prefix            string
	}{
		{networkDHCPInterfacesToBridge, networkDHCPInitial, networkDHCPExpected, "test-br-"},
		{networkStaticWithAliasInterfacesToBridge, networkStaticWithAliasInitial, networkStaticWithAliasExpected, "test-br-"},
		{networkDHCPWithBondInterfacesToBridge, networkDHCPWithBondInitial, networkDHCPWithBondExpected, "test-br-"},
		{networkDualNICInterfacesToBridge, networkDualNICInitial, networkDualNICExpected, "test-br-"},
		{networkMultipleAliasesInterfacesToBridge, networkMultipleAliasesInitial, networkMultipleAliasesExpected, "test-br-"},
		{networkMultipleStaticWithAliasesInterfacesToBridge, networkMultipleStaticWithAliasesInitial, networkMultipleStaticWithAliasesExpected, "test-br-"},
		{networkSmorgasboardInterfacesToBridge, networkSmorgasboardInitial, networkSmorgasboardExpected, "juju-br-"},
		{networkStaticInterfacesToBridge, networkStaticInitial, networkStaticExpected, "test-br-"},
		{networkVLANInterfacesToBridge, networkVLANInitial, networkVLANExpected, "vlan-br-"},
		{networkWithAliasInterfacesToBridge, networkWithAliasInitial, networkWithAliasExpected, "test-br-"},
	} {
		c.Logf("test #%v - expected transformation", i)
		s.assertScriptWithPrefix(c, v.initial, v.expected, v.prefix, v.interfaceToBridge)
		c.Logf("test #%v - idempotent transformation", i)
		s.assertScriptWithPrefix(c, v.expected, v.expected, v.prefix, v.interfaceToBridge)
	}
}

func (s *bridgeConfigSuite) TestBridgeScriptWithDefaultPrefixTransformation(c *gc.C) {
	for i, v := range []struct {
		interfaceToBridge string
		initial           string
		expected          string
	}{
		{networkLoopbackOnlyInterfacesToBridge, networkLoopbackOnlyInitial, networkLoopbackOnlyExpected},
		{networkStaticBondWithVLANsInterfacesToBridge, networkStaticBondWithVLANsInitial, networkStaticBondWithVLANsExpected},
		{networkVLANWithActiveDHCPDeviceInterfacesToBridge, networkVLANWithActiveDHCPDeviceInitial, networkVLANWithActiveDHCPDeviceExpected},
		{networkVLANWithInactiveDeviceInterfacesToBridge, networkVLANWithInactiveDeviceInitial, networkVLANWithInactiveDeviceExpected},
		{networkVLANWithMultipleNameserversInterfacesToBridge, networkVLANWithMultipleNameserversInitial, networkVLANWithMultipleNameserversExpected},
		{networkWithEmptyDNSValuesInterfacesToBridge, networkWithEmptyDNSValuesInitial, networkWithEmptyDNSValuesExpected},
		{networkWithMultipleDNSValuesInterfacesToBridge, networkWithMultipleDNSValuesInitial, networkWithMultipleDNSValuesExpected},
		{networkPartiallyBridgedInterfacesToBridge, networkPartiallyBridgedInitial, networkPartiallyBridgedExpected},
	} {
		c.Logf("test #%v - expected transformation", i)
		s.assertScriptWithDefaultPrefix(c, v.initial, v.expected, v.interfaceToBridge)
		c.Logf("test #%v - idempotent transformation", i)
		s.assertScriptWithDefaultPrefix(c, v.expected, v.expected, v.interfaceToBridge)
	}
}

func (s *bridgeConfigSuite) TestBridgeScriptInterfaceNameArgumentRequired(c *gc.C) {
	for i, python := range s.pythonVersions {
		c.Logf("test #%v using %s", i, python)
		output, code := s.runScriptWithoutActivation(c, python, "# no content", "", "juju-br0", "")
		c.Check(code, gc.Equals, 2)
		// We match very lazily here to isolate ourselves from
		// the different formatting of argparse error messages
		// that has occured between Python 2 and Python 3.
		c.Check(strings.Trim(output, "\n"), gc.Matches, "(\n|.)*error:.*--interfaces-to-bridge.*")
	}
}

func (s *bridgeConfigSuite) TestBridgeScriptMatchingExistingSpecificIfaceButMissingAutoStanza(c *gc.C) {
	s.assertScriptWithoutPrefix(c, networkWithExistingSpecificIfaceInitial, networkWithExistingSpecificIfaceExpected, "juju-br0", "eth1")
}

func (s *bridgeConfigSuite) TestBridgeScriptMatchingExistingSpecificIface2(c *gc.C) {
	s.assertScriptWithoutPrefix(c, networkLP1532167Initial, networkLP1532167Expected, "juju-br0", "bond0")
}

func (s *bridgeConfigSuite) runScriptWithoutActivation(c *gc.C, pythonBinary, configFile, bridgePrefix, bridgeName, interfaceToBridge string) (output string, exitCode int) {
	if bridgePrefix != "" {
		bridgePrefix = fmt.Sprintf("--bridge-prefix=%q", bridgePrefix)
	}

	if bridgeName != "" {
		bridgeName = fmt.Sprintf("--bridge-name=%q", bridgeName)
	}

	if interfaceToBridge != "" {
		interfaceToBridge = fmt.Sprintf("--interfaces-to-bridge=%q", interfaceToBridge)
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

func (s *bridgeConfigSuite) runScriptWithActivationAndDryRun(c *gc.C, pythonBinary, bridgePrefix string, interfacesToBridge []string) (output []string, exitCode int) {
	if bridgePrefix != "" {
		bridgePrefix = fmt.Sprintf("--bridge-prefix=%q", bridgePrefix)
	}

	script := fmt.Sprintf("%q %q --activate --dry-run %s %s --interfaces-to-bridge=%q",
		pythonBinary, s.testPythonScript, bridgePrefix, s.testConfigPath, strings.Join(interfacesToBridge, " "))
	c.Log(script)
	result, err := exec.RunCommands(exec.RunParams{Commands: script})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("script failed unexpectedly"))
	stdout := strings.Split(string(result.Stdout), "\n")
	stderr := strings.Split(string(result.Stderr), "\n")
	output = make([]string, 0, len(stdout)+len(stderr))
	for _, s := range stdout {
		s = strings.Trim(s, "\n")
		if s != "" {
			output = append(output, s)
		}
	}
	for _, s := range stderr {
		s = strings.Trim(s, "\n")
		if s != "" {
			output = append(output, s)
		}
	}
	return output, result.Code
}

func (s *bridgeConfigSuite) dryRunExpectedOutputHelper(isBonded, isAlreadyBridged bool, bridgePrefix string, interfacesToBridge []string) []string {
	output := make([]string, 0)
	bridgedNames := make([]string, len(interfacesToBridge))
	for i, s := range interfacesToBridge {
		bridgedNames[i] = bridgePrefix + s
	}
	if isAlreadyBridged {
		output = append(output, "already bridged, or nothing to do.")
	} else {
		output = append(output, "**** Original configuration")
		output = append(output, fmt.Sprintf("cat %s", s.testConfigPath))
		output = append(output, "ip -d link show")
		output = append(output, "ip route show")
		output = append(output, "brctl show")
		output = append(output, fmt.Sprintf("ifdown --exclude=lo --interfaces=%[1]s %s", s.testConfigPath, strings.Join(interfacesToBridge, " ")))
		output = append(output, "**** Activating new configuration")
		if isBonded {
			output = append(output, "working around https://bugs.launchpad.net/ubuntu/+source/ifenslave/+bug/1269921")
			output = append(output, "working around https://bugs.launchpad.net/juju-core/+bug/1594855")
			output = append(output, "sleep 3")
		}
		output = append(output, fmt.Sprintf("cat %s", s.testConfigPath))
		output = append(output, fmt.Sprintf("ifup --exclude=lo --interfaces=%[1]s -a", s.testConfigPath))
		output = append(output, "ip -d link show")
		output = append(output, "ip route show")
		output = append(output, "brctl show")
	}
	return output
}

func (s *bridgeConfigSuite) TestBridgeScriptWithoutBondedInterfaceSingle(c *gc.C) {
	bridgePrefix := "TestBridgeScriptWithoutBondedInterfaceSingle"
	interfacesToBridge := []string{"eth0"}
	s.assertScriptWithActivationAndDryRun(c, false, false, networkDHCPInitial, bridgePrefix, interfacesToBridge)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithoutBondedInterfaceMultiple(c *gc.C) {
	bridgePrefix := "TestBridgeScriptWithoutBondedInterfaceMultiple"
	interfacesToBridge := []string{"eth0", "eth1"}
	s.assertScriptWithActivationAndDryRun(c, false, false, networkDHCPInitial, bridgePrefix, interfacesToBridge)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithBondedInterfaceSingle(c *gc.C) {
	bridgePrefix := "TestBridgeScriptWithBondedInterfaceSingle"
	interfacesToBridge := []string{"bond0"}
	s.assertScriptWithActivationAndDryRun(c, true, false, networkDHCPWithBondInitial, bridgePrefix, interfacesToBridge)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithBondedInterfaceMultiple(c *gc.C) {
	bridgePrefix := "TestBridgeScriptWithBondedInterfaceMultiple"
	interfacesToBridge := []string{"bond0", "bond1"}
	s.assertScriptWithActivationAndDryRun(c, true, false, networkDHCPWithBondInitial, bridgePrefix, interfacesToBridge)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithBondedInterfaceAlreadyBridged(c *gc.C) {
	bridgePrefix := "TestBridgeScriptWithBondedInterfaceAlreadyBridged"
	interfacesToBridge := []string{"br-eth1"}
	s.assertScriptWithActivationAndDryRun(c, false, true, networkPartiallyBridgedInitial, bridgePrefix, interfacesToBridge)
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

const networkStaticInterfacesToBridge = "eth0"

const networkStaticExpected = `auto lo
iface lo inet loopback

auto eth0
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

const networkDHCPInterfacesToBridge = "eth0"

const networkDHCPExpected = `auto lo
iface lo inet loopback

auto eth0
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

const networkDualNICInterfacesToBridge = "eth0 eth1"

const networkDualNICExpected = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
    bridge_ports eth0

auto eth1
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

const networkWithAliasInterfacesToBridge = "eth0 eth0:1"

const networkWithAliasExpected = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
    bridge_ports eth0

auto test-br-eth0:1
iface test-br-eth0:1 inet static
    address 1.2.3.5`

const networkStaticWithAliasInitial = `auto lo
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

const networkStaticWithAliasInterfacesToBridge = "eth0 eth0:1 eth0:2"

const networkStaticWithAliasExpected = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet static
    gateway 10.14.0.1
    address 10.14.0.102/24
    bridge_ports eth0

auto test-br-eth0:1
iface test-br-eth0:1 inet static
    address 10.14.0.103/24

auto test-br-eth0:2
iface test-br-eth0:2 inet static
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

const networkMultipleStaticWithAliasesInterfacesToBridge = "eth0 eth0:1"

const networkMultipleStaticWithAliasesExpected = `auto eth0
iface eth0 inet manual
    mtu 1500

auto test-br-eth0
iface test-br-eth0 inet static
    gateway 10.17.20.1
    address 10.17.20.201/24
    bridge_ports eth0

auto test-br-eth0:1
iface test-br-eth0:1 inet static
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

const networkDHCPWithBondInterfacesToBridge = "bond0"

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

auto test-br-bond0
iface test-br-bond0 inet dhcp
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

const networkMultipleAliasesInterfacesToBridge = "eth0 eth1 eth10 eth10:1 eth10:2"

const networkMultipleAliasesExpected = `auto eth0
iface eth0 inet manual

auto test-br-eth0
iface test-br-eth0 inet dhcp
    bridge_ports eth0

auto eth1
iface eth1 inet manual

auto test-br-eth1
iface test-br-eth1 inet dhcp
    bridge_ports eth1

auto eth10
iface eth10 inet manual
    mtu 1500

auto test-br-eth10
iface test-br-eth10 inet static
    gateway 10.17.20.1
    address 10.17.20.201/24
    bridge_ports eth10

auto test-br-eth10:1
iface test-br-eth10:1 inet static
    address 10.17.20.202/24
    mtu 1500

auto test-br-eth10:2
iface test-br-eth10:2 inet static
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

const networkSmorgasboardInterfacesToBridge = "eth4 eth5 eth6 eth6:1 eth6:2 eth6:3 eth6:4 bond0 bond1"

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

auto eth4
iface eth4 inet manual
    mtu 1500

auto juju-br-eth4
iface juju-br-eth4 inet static
    address 10.17.20.202/24
    bridge_ports eth4

auto eth5
iface eth5 inet manual
    mtu 1500

auto juju-br-eth5
iface juju-br-eth5 inet dhcp
    bridge_ports eth5

auto eth6
iface eth6 inet manual
    mtu 1500

auto juju-br-eth6
iface juju-br-eth6 inet static
    address 10.17.20.203/24
    bridge_ports eth6

auto juju-br-eth6:1
iface juju-br-eth6:1 inet static
    address 10.17.20.205/24
    mtu 1500

auto juju-br-eth6:2
iface juju-br-eth6:2 inet static
    address 10.17.20.204/24
    mtu 1500

auto juju-br-eth6:3
iface juju-br-eth6:3 inet static
    address 10.17.20.206/24
    mtu 1500

auto juju-br-eth6:4
iface juju-br-eth6:4 inet static
    address 10.17.20.207/24
    mtu 1500

auto bond0
iface bond0 inet manual
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

auto juju-br-bond1
iface juju-br-bond1 inet dhcp
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

const networkVLANInterfacesToBridge = "eth0 eth0.2 eth1.3"

const networkVLANExpected = `auto eth0
iface eth0 inet manual
    mtu 1500

auto vlan-br-eth0
iface vlan-br-eth0 inet static
    gateway 10.17.20.1
    address 10.17.20.212/24
    bridge_ports eth0

auto eth1
iface eth1 inet manual
    mtu 1500

auto eth0.2
iface eth0.2 inet manual
    vlan-raw-device eth0
    mtu 1500
    vlan_id 2

auto vlan-br-eth0.2
iface vlan-br-eth0.2 inet static
    address 192.168.2.3/24
    bridge_ports eth0.2

auto eth1.3
iface eth1.3 inet manual
    vlan-raw-device eth1
    mtu 1500
    vlan_id 3

auto vlan-br-eth1.3
iface vlan-br-eth1.3 inet static
    address 192.168.3.3/24
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

const networkVLANWithMultipleNameserversInterfacesToBridge = "eth0 eth1.2667 eth1.2668 eth1.2669 eth1.2670"

const networkVLANWithMultipleNameserversExpected = `auto eth0
iface eth0 inet manual
    mtu 1500

auto br-eth0
iface br-eth0 inet static
    gateway 10.245.168.1
    address 10.245.168.11/21
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

auto eth1.2667
iface eth1.2667 inet manual
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2667

auto br-eth1.2667
iface br-eth1.2667 inet static
    address 10.245.184.2/24
    bridge_ports eth1.2667
    dns-nameservers 10.245.168.2

auto eth1.2668
iface eth1.2668 inet manual
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2668

auto br-eth1.2668
iface br-eth1.2668 inet static
    address 10.245.185.1/24
    bridge_ports eth1.2668
    dns-nameservers 10.245.168.2

auto eth1.2669
iface eth1.2669 inet manual
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2669

auto br-eth1.2669
iface br-eth1.2669 inet static
    address 10.245.186.1/24
    bridge_ports eth1.2669
    dns-nameservers 10.245.168.2

auto eth1.2670
iface eth1.2670 inet manual
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2670

auto br-eth1.2670
iface br-eth1.2670 inet static
    address 10.245.187.2/24
    bridge_ports eth1.2670
    dns-nameservers 10.245.168.2
    dns-search dellstack`

const networkLoopbackOnlyInitial = `auto lo
iface lo inet loopback`

const networkLoopbackOnlyInterfacesToBridge = "lo"

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

const networkStaticBondWithVLANsInterfacesToBridge = "bond0 bond0.2 bond0.3"

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
    bond-slaves none
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
    hwaddress 52:54:00:1c:f1:5b
    bridge_ports bond0
    dns-nameservers 10.17.20.200

auto bond0.2
iface bond0.2 inet manual
    vlan-raw-device bond0
    mtu 1500
    vlan_id 2

auto br-bond0.2
iface br-bond0.2 inet static
    address 192.168.2.102/24
    bridge_ports bond0.2

auto bond0.3
iface bond0.3 inet manual
    vlan-raw-device bond0
    mtu 1500
    vlan_id 3

auto br-bond0.3
iface br-bond0.3 inet static
    address 192.168.3.101/24
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

const networkVLANWithInactiveDeviceInterfacesToBridge = "eth0 eth1.2"

const networkVLANWithInactiveDeviceExpected = `auto eth0
iface eth0 inet manual
    mtu 1500

auto br-eth0
iface br-eth0 inet static
    gateway 10.17.20.1
    address 10.17.20.211/24
    bridge_ports eth0
    dns-nameservers 10.17.20.200

auto eth1
iface eth1 inet manual
    mtu 1500

auto eth1.2
iface eth1.2 inet manual
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2

auto br-eth1.2
iface br-eth1.2 inet dhcp
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

const networkVLANWithActiveDHCPDeviceInterfacesToBridge = "eth0 eth1 eth1.2"

const networkVLANWithActiveDHCPDeviceExpected = `auto eth0
iface eth0 inet manual
    mtu 1500

auto br-eth0
iface br-eth0 inet static
    gateway 10.17.20.1
    address 10.17.20.211/24
    bridge_ports eth0
    dns-nameservers 10.17.20.200

auto eth1
iface eth1 inet manual
    mtu 1500

auto br-eth1
iface br-eth1 inet dhcp
    bridge_ports eth1

auto eth1.2
iface eth1.2 inet manual
    vlan-raw-device eth1
    mtu 1500
    vlan_id 2

auto br-eth1.2
iface br-eth1.2 inet dhcp
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

const networkWithMultipleDNSValuesInterfacesToBridge = "eth0 eth1 eth2 eth3"

const networkWithMultipleDNSValuesExpected = `auto eth0
iface eth0 inet manual
    mtu 1500

auto br-eth0
iface br-eth0 inet static
    gateway 10.245.168.1
    address 10.245.168.11/21
    bridge_ports eth0
    dns-nameservers 10.245.168.2 192.168.1.1

auto eth1
iface eth1 inet manual
    mtu 1500

auto br-eth1
iface br-eth1 inet static
    gateway 10.245.168.1
    address 10.245.168.12/21
    bridge_ports eth1
    dns-sortlist 192.168.1.0/24 10.245.168.0/21

auto eth2
iface eth2 inet manual
    mtu 1500

auto br-eth2
iface br-eth2 inet static
    gateway 10.245.168.1
    address 10.245.168.13/21
    bridge_ports eth2
    dns-search juju ubuntu dellstack

auto eth3
iface eth3 inet manual
    mtu 1500

auto br-eth3
iface br-eth3 inet static
    gateway 10.245.168.1
    address 10.245.168.14/21
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

const networkWithEmptyDNSValuesInterfacesToBridge = "eth0 eth1"

const networkWithEmptyDNSValuesExpected = `auto eth0
iface eth0 inet manual
    mtu 1500

auto br-eth0
iface br-eth0 inet static
    gateway 10.245.168.1
    address 10.245.168.11/21
    bridge_ports eth0

auto eth1
iface eth1 inet manual
    mtu 1500

auto br-eth1
iface br-eth1 inet static
    gateway 10.245.168.1
    address 10.245.168.12/21
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

const networkLP1532167InterfacesToBridge = "bond0 bond0.1016 bond0.161 bond0.162 bond0.163 bond1.1017 bond1.1018 bond1.1019"

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

const networkWithExistingSpecificIfaceInterfacesToBridge = "eth0 eth1"

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

const networkPartiallyBridgedInitial = `auto lo
iface lo inet loopback

iface eth0 inet manual

auto br-eth0
iface br-eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
    bridge_ports eth0

auto eth1
iface eth1 inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 4.3.2.1`

const networkPartiallyBridgedInterfacesToBridge = "br-eth0 eth1"

const networkPartiallyBridgedExpected = `auto lo
iface lo inet loopback

iface eth0 inet manual

auto br-eth0
iface br-eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
    bridge_ports eth0

auto eth1
iface eth1 inet manual

auto br-eth1
iface br-eth1 inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 4.3.2.1
    bridge_ports eth1`
