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

func (s *bridgeConfigSuite) assertScript(c *gc.C, initialConfig, expectedConfig, addrFamily, nic, bridge string) {
	err := ioutil.WriteFile(s.testConfigPath, []byte(initialConfig), 0644)
	c.Check(err, jc.ErrorIsNil)
	// Run the script and verify the modified config.
	output, code := s.runScript(c, addrFamily, nic, s.testConfigPath, bridge)
	c.Check(code, gc.Equals, 0)
	c.Check(output, gc.Equals, "")
	data, err := ioutil.ReadFile(s.testConfigPath)
	c.Check(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, expectedConfig)
}

func (s *bridgeConfigSuite) TestBridgeScriptWithInvalidParams(c *gc.C) {
	var tests = []struct {
		about  string
		params []string
	}{{
		about:  "argument 1 is zero length",
		params: []string{"", "2", "3", "4"},
	}, {
		about:  "argument 2 is zero length",
		params: []string{"1", "", "3", "4"},
	}, {
		about:  "argument 3 is zero length",
		params: []string{"1", "2", "", "4"},
	}, {
		about:  "argument 4 is zero length",
		params: []string{"1", "2", "3", ""},
	}, {
		about:  "both addr_family and primary_nic arguments empty",
		params: []string{"", "", s.testBridgeName, s.testConfigPath},
	}, {
		about:  "invalid address family, empty primary NIC",
		params: []string{"foo", "", s.testBridgeName, s.testConfigPath},
	}, {
		about:  "empty address family, invalid primary NIC",
		params: []string{"", "bar", s.testBridgeName, s.testConfigPath},
	}, {
		about:  "valid address family, empty primary NIC",
		params: []string{"inet", "", s.testBridgeName, s.testConfigPath},
	}, {
		about:  "valid address family, invalid primary NIC",
		params: []string{"inet", "foo", s.testBridgeName, s.testConfigPath},
	}, {
		about:  "valid, but mismatched address family, valid primary NIC",
		params: []string{"inet6", "eth0", s.testBridgeName, s.testConfigPath},
	}}

	for i, test := range tests {
		c.Logf("test #%d: %s", i, test.about)

		// Simple initial config.
		err := ioutil.WriteFile(s.testConfigPath, []byte(networkDHCPInitial), 0644)
		c.Check(err, jc.ErrorIsNil)

		// Run and check it fails.
		output, code := s.runScript(c, test.params[0], test.params[1], test.params[2], test.params[3])
		c.Check(code, gc.Equals, 1)
		c.Check(output, gc.Equals, "")

		// Verify the config was not modified.
		data, err := ioutil.ReadFile(s.testConfigPath)
		c.Check(err, jc.ErrorIsNil)
		c.Check(string(data), gc.Equals, networkDHCPInitial)
	}
}

func (s *bridgeConfigSuite) TestBridgeScriptWithZeroArgs(c *gc.C) {
	_, code := s.runScript(c, "", "", "", "")
	c.Check(code, gc.Equals, 1)
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCP(c *gc.C) {
	s.assertScript(c, networkDHCPInitial, networkDHCPExpected, "inet", "eth0", "juju-br0")
}

func (s *bridgeConfigSuite) TestBridgeScriptStatic(c *gc.C) {
	s.assertScript(c, networkStaticInitial, networkStaticExpected, "inet", "eth0", "juju-br0")
}

func (s *bridgeConfigSuite) TestBridgeScriptMultiple(c *gc.C) {
	s.assertScript(c, networkMultipleInitial, networkMultipleExpected, "inet", "eth0", "juju-br0")
}

func (s *bridgeConfigSuite) TestBridgeScriptWithAlias(c *gc.C) {
	s.assertScript(c, networkWithAliasInitial, networkWithAliasExpected, "inet", "eth0", "juju-br0")
}

func (s *bridgeConfigSuite) TestBridgeScriptDHCPWithAlias(c *gc.C) {
	s.assertScript(c, networkDHCPWithAliasInitial, networkDHCPWithAliasExpected, "inet", "eth0", "juju-br0")
}

func (s *bridgeConfigSuite) TestBridgeScriptMultipleStaticWithAliases(c *gc.C) {
	s.assertScript(c, networkMultipleStaticWithAliasesInitial, networkMultipleStaticWithAliasesExpected, "inet", "eth0", "juju-br0")
}

func (s *bridgeConfigSuite) runScript(c *gc.C, addressFamily, nic, configFile, bridgeName string) (output string, exitCode int) {
	script := fmt.Sprintf("%s\n%s %q %q %q %q\n",
		bridgeScriptBase,
		"modify_network_config",
		addressFamily,
		nic,
		configFile,
		bridgeName)

	result, err := exec.RunCommands(exec.RunParams{Commands: script})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("script failed unexpectedly"))
	// To simplify most cases, trim any trailing new lines, but still separate
	// the stdout and stderr (in that order) with a new line, if both are
	// non-empty.
	stdout := strings.TrimSuffix(string(result.Stdout), "\n")
	stderr := strings.TrimSuffix(string(result.Stderr), "\n")
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
    bridge_ports eth0
`

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

const networkMultipleStaticWithAliasesExpected = `
iface eth0 inet manual

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
