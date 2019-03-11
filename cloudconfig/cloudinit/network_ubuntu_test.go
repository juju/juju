// Copyright 2018 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/container"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type NetworkUbuntuSuite struct {
	testing.BaseSuite

	networkInterfacesPythonFile string
	systemNetworkInterfacesFile string
	jujuNetplanFile             string

	fakeInterfaces []network.InterfaceInfo

	expectedSampleConfigHeader      string
	expectedSampleConfigTemplate    string
	expectedSampleConfigWriting     string
	expectedSampleUserData          string
	expectedFallbackConfig          string
	expectedBaseConfig              string
	expectedFullNetplanYaml         string
	expectedBaseNetplanYaml         string
	expectedFullNetplan             string
	expectedBaseNetplan             string
	expectedFallbackUserData        string
	tempFolder                      string
	pythonVersions                  []string
	originalSystemNetworkInterfaces string
}

var _ = gc.Suite(&NetworkUbuntuSuite{})

func (s *NetworkUbuntuSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	if runtime.GOOS == "windows" {
		c.Skip("This test is for Linux only")
	}

	s.tempFolder = c.MkDir()
	networkFolder := c.MkDir()
	netplanFolder := c.MkDir()
	s.systemNetworkInterfacesFile = filepath.Join(networkFolder, "system-interfaces")
	s.networkInterfacesPythonFile = filepath.Join(networkFolder, "system-interfaces.py")
	s.jujuNetplanFile = filepath.Join(netplanFolder, "79-juju.yaml")

	s.fakeInterfaces = []network.InterfaceInfo{{
		InterfaceName:    "any0",
		CIDR:             "0.1.2.0/24",
		ConfigType:       network.ConfigStatic,
		NoAutoStart:      false,
		Address:          network.NewAddress("0.1.2.3"),
		DNSServers:       network.NewAddresses("ns1.invalid", "ns2.invalid"),
		DNSSearchDomains: []string{"foo", "bar"},
		GatewayAddress:   network.NewAddress("0.1.2.1"),
		MACAddress:       "aa:bb:cc:dd:ee:f0",
		MTU:              8317,
	}, {
		InterfaceName:    "any1",
		CIDR:             "0.2.2.0/24",
		ConfigType:       network.ConfigStatic,
		NoAutoStart:      false,
		Address:          network.NewAddress("0.2.2.4"),
		DNSServers:       network.NewAddresses("ns1.invalid", "ns2.invalid"),
		DNSSearchDomains: []string{"foo", "bar"},
		GatewayAddress:   network.NewAddress("0.2.2.1"),
		MACAddress:       "aa:bb:cc:dd:ee:f1",
		Routes: []network.Route{{
			DestinationCIDR: "0.5.6.0/24",
			GatewayIP:       "0.2.2.1",
			Metric:          50,
		}},
	}, {
		InterfaceName: "any2",
		ConfigType:    network.ConfigDHCP,
		MACAddress:    "aa:bb:cc:dd:ee:f2",
		NoAutoStart:   true,
	}, {
		InterfaceName: "any3",
		ConfigType:    network.ConfigDHCP,
		MACAddress:    "aa:bb:cc:dd:ee:f3",
		NoAutoStart:   false,
	}, {
		InterfaceName: "any4",
		ConfigType:    network.ConfigManual,
		MACAddress:    "aa:bb:cc:dd:ee:f4",
		NoAutoStart:   true,
	}, {
		InterfaceName:  "any5",
		ConfigType:     network.ConfigStatic,
		MACAddress:     "aa:bb:cc:dd:ee:f5",
		NoAutoStart:    false,
		CIDR:           "2001:db8::/64",
		Address:        network.NewAddress("2001:db8::dead:beef"),
		GatewayAddress: network.NewAddress("2001:db8::dead:f00"),
	}}

	for _, version := range []string{
		"/usr/bin/python2",
		"/usr/bin/python3",
		"/usr/bin/python",
	} {
		if _, err := os.Stat(version); err == nil {
			s.pythonVersions = append(s.pythonVersions, version)
		}
	}
	c.Assert(s.pythonVersions, gc.Not(gc.HasLen), 0)

	s.expectedSampleConfigHeader = `#cloud-config
bootcmd:
`
	s.expectedSampleConfigWriting = `- install -D -m 644 /dev/null '%[1]s.templ'
- |-
  printf '%%s\n' '
  auto lo {ethaa_bb_cc_dd_ee_f0} {ethaa_bb_cc_dd_ee_f1} {ethaa_bb_cc_dd_ee_f3} {ethaa_bb_cc_dd_ee_f5}

  iface lo inet loopback
    dns-nameservers ns1.invalid ns2.invalid
    dns-search bar foo

  iface {ethaa_bb_cc_dd_ee_f0} inet static
    address 0.1.2.3/24
    gateway 0.1.2.1
    mtu 8317

  iface {ethaa_bb_cc_dd_ee_f1} inet static
    address 0.2.2.4/24
    post-up ip route add 0.5.6.0/24 via 0.2.2.1 metric 50
    pre-down ip route del 0.5.6.0/24 via 0.2.2.1 metric 50

  iface {ethaa_bb_cc_dd_ee_f2} inet dhcp

  iface {ethaa_bb_cc_dd_ee_f3} inet dhcp

  iface {ethaa_bb_cc_dd_ee_f4} inet manual

  iface {ethaa_bb_cc_dd_ee_f5} inet6 static
    address 2001:db8::dead:beef/64
    gateway 2001:db8::dead:f00
  ' > '%[1]s.templ'
`
	s.expectedSampleConfigTemplate = `
auto lo {ethaa_bb_cc_dd_ee_f0} {ethaa_bb_cc_dd_ee_f1} {ethaa_bb_cc_dd_ee_f3} {ethaa_bb_cc_dd_ee_f5}

iface lo inet loopback
  dns-nameservers ns1.invalid ns2.invalid
  dns-search bar foo

iface {ethaa_bb_cc_dd_ee_f0} inet static
  address 0.1.2.3/24
  gateway 0.1.2.1
  mtu 8317

iface {ethaa_bb_cc_dd_ee_f1} inet static
  address 0.2.2.4/24
  post-up ip route add 0.5.6.0/24 via 0.2.2.1 metric 50
  pre-down ip route del 0.5.6.0/24 via 0.2.2.1 metric 50

iface {ethaa_bb_cc_dd_ee_f2} inet dhcp

iface {ethaa_bb_cc_dd_ee_f3} inet dhcp

iface {ethaa_bb_cc_dd_ee_f4} inet manual

iface {ethaa_bb_cc_dd_ee_f5} inet6 static
  address 2001:db8::dead:beef/64
  gateway 2001:db8::dead:f00
`

	networkInterfacesScriptYamled := strings.Replace(cloudinit.NetworkInterfacesScript, "\n", "\n  ", -1)
	networkInterfacesScriptYamled = strings.Replace(networkInterfacesScriptYamled, "\n  \n", "\n\n", -1)
	networkInterfacesScriptYamled = strings.Replace(networkInterfacesScriptYamled, "%", "%%", -1)
	networkInterfacesScriptYamled = strings.Replace(networkInterfacesScriptYamled, "'", "'\"'\"'", -1)

	s.expectedSampleUserData = `- install -D -m 744 /dev/null '%[2]s'
- |-
  printf '%%s\n' '` + networkInterfacesScriptYamled + ` ' > '%[2]s'
- |2

  if [ ! -f /sbin/ifup ]; then
      echo "No /sbin/ifup, applying netplan configuration."
      netplan generate
      netplan apply
  else
    if [ -f /usr/bin/python ]; then
        python %[2]s --interfaces-file %[1]s --output-file %[1]s.out
    else
        python3 %[2]s --interfaces-file %[1]s --output-file %[1]s.out
    fi
    ifdown -a
    sleep 1.5
    mv %[1]s.out %[1]s
    ifup -a
  fi
`[1:]
	s.expectedFullNetplanYaml = `
- install -D -m 644 /dev/null '%[1]s'
- |-
  printf '%%s\n' 'network:
    version: 2
    ethernets:
      any0:
        match:
          macaddress: aa:bb:cc:dd:ee:f0
        addresses:
        - 0.1.2.3/24
        gateway4: 0.1.2.1
        nameservers:
          search: [foo, bar]
          addresses: [ns1.invalid, ns2.invalid]
        mtu: 8317
      any1:
        match:
          macaddress: aa:bb:cc:dd:ee:f1
        addresses:
        - 0.2.2.4/24
        gateway4: 0.2.2.1
        nameservers:
          search: [foo, bar]
          addresses: [ns1.invalid, ns2.invalid]
        routes:
        - to: 0.5.6.0/24
          via: 0.2.2.1
          metric: 50
      any2:
        match:
          macaddress: aa:bb:cc:dd:ee:f2
        dhcp4: true
      any3:
        match:
          macaddress: aa:bb:cc:dd:ee:f3
        dhcp4: true
      any4:
        match:
          macaddress: aa:bb:cc:dd:ee:f4
      any5:
        match:
          macaddress: aa:bb:cc:dd:ee:f5
        addresses:
        - 2001:db8::dead:beef/64
        gateway6: 2001:db8::dead:f00
  ' > '%[1]s'
`[1:]

	s.expectedFullNetplan = `
network:
  version: 2
  ethernets:
    any0:
      match:
        macaddress: aa:bb:cc:dd:ee:f0
      addresses:
      - 0.1.2.3/24
      gateway4: 0.1.2.1
      nameservers:
        search: [foo, bar]
        addresses: [ns1.invalid, ns2.invalid]
      mtu: 8317
    any1:
      match:
        macaddress: aa:bb:cc:dd:ee:f1
      addresses:
      - 0.2.2.4/24
      gateway4: 0.2.2.1
      nameservers:
        search: [foo, bar]
        addresses: [ns1.invalid, ns2.invalid]
      routes:
      - to: 0.5.6.0/24
        via: 0.2.2.1
        metric: 50
    any2:
      match:
        macaddress: aa:bb:cc:dd:ee:f2
      dhcp4: true
    any3:
      match:
        macaddress: aa:bb:cc:dd:ee:f3
      dhcp4: true
    any4:
      match:
        macaddress: aa:bb:cc:dd:ee:f4
    any5:
      match:
        macaddress: aa:bb:cc:dd:ee:f5
      addresses:
      - 2001:db8::dead:beef/64
      gateway6: 2001:db8::dead:f00
`[1:]

	s.expectedBaseNetplanYaml = `
- install -D -m 644 /dev/null '%[1]s'
- |-
  printf '%%s\n' 'network:
    version: 2
    ethernets:
      eth0:
        match:
          name: eth0
        dhcp4: true
  ' > '%[1]s'
`[1:]

	s.expectedFallbackConfig = `- install -D -m 644 /dev/null '%[1]s.templ'
- |-
  printf '%%s\n' '
  auto lo {eth}

  iface lo inet loopback

  iface {eth} inet dhcp
  ' > '%[1]s.templ'
`

	s.expectedBaseConfig = `
auto lo {eth}

iface lo inet loopback

iface {eth} inet dhcp
`

	s.expectedBaseNetplan = `
network:
  version: 2
  ethernets:
    eth0:
      match:
        name: eth0
      dhcp4: true
`[1:]

	s.expectedFallbackUserData = `
#cloud-config
bootcmd:
- install -D -m 644 /dev/null '%[1]s'
- |-
  printf '%%s\n' '
  auto eth0 lo

  iface lo inet loopback

  iface eth0 inet dhcp
  ' > '%[1]s'
runcmd:
- |-
  if [ -f %[1]s ]; then
      echo "stopping all interfaces"
      ifdown -a
      sleep 1.5
      if ifup -a --interfaces=%[1]s; then
          echo "ifup with %[1]s succeeded, renaming to %[2]s"
          cp %[2]s %[2]s-orig
          cp %[1]s %[2]s
      else
          echo "ifup with %[1]s failed, leaving old %[2]s alone"
          ifup -a
      fi
  else
      echo "did not find %[1]s, not reconfiguring networking"
  fi
`[1:]

	s.originalSystemNetworkInterfaces = `
# This file describes the network interfaces available on your system
# and how to activate them. For more information, see interfaces(5).

# The loopback network interface
auto lo
iface lo inet loopback

# Source interfaces
# Please check /etc/network/interfaces.d before changing this file
# as interfaces may have been defined in /etc/network/interfaces.d
# See LP: #1262951
source /etc/network/interfaces.d/*.cfg
`[1:]

	s.PatchValue(cloudinit.NetworkInterfacesFile, s.systemNetworkInterfacesFile)
	s.PatchValue(cloudinit.SystemNetworkInterfacesFile, s.systemNetworkInterfacesFile)
	s.PatchValue(cloudinit.JujuNetplanFile, s.jujuNetplanFile)
}

func (s *NetworkUbuntuSuite) TestGenerateENIConfig(c *gc.C) {
	data, err := cloudinit.GenerateENITemplate(nil)
	c.Assert(err, gc.ErrorMatches, "missing container network config")
	c.Assert(data, gc.Equals, "")

	netConfig := container.BridgeNetworkConfig("foo", 0, nil)
	data, err = cloudinit.GenerateENITemplate(netConfig.Interfaces)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data, gc.Equals, s.expectedBaseConfig)

	// Test with all interface types.
	netConfig = container.BridgeNetworkConfig("foo", 0, s.fakeInterfaces)
	data, err = cloudinit.GenerateENITemplate(netConfig.Interfaces)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data, gc.Equals, s.expectedSampleConfigTemplate)
}

func (s *NetworkUbuntuSuite) TestGenerateNetplan(c *gc.C) {
	data, err := cloudinit.GenerateNetplan(nil)
	c.Assert(err, gc.ErrorMatches, "missing container network config")
	c.Assert(data, gc.Equals, "")

	netConfig := container.BridgeNetworkConfig("foo", 0, nil)
	data, err = cloudinit.GenerateNetplan(netConfig.Interfaces)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data, gc.Equals, s.expectedBaseNetplan)

	// Test with all interface types.
	netConfig = container.BridgeNetworkConfig("foo", 0, s.fakeInterfaces)
	data, err = cloudinit.GenerateNetplan(netConfig.Interfaces)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data, gc.Equals, s.expectedFullNetplan)
}

func (s *NetworkUbuntuSuite) TestAddNetworkConfigSampleConfig(c *gc.C) {
	netConfig := container.BridgeNetworkConfig("foo", 0, s.fakeInterfaces)
	cloudConf, err := cloudinit.New("xenial")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudConf, gc.NotNil)
	err = cloudConf.AddNetworkConfig(netConfig.Interfaces)
	c.Assert(err, jc.ErrorIsNil)

	expected := s.expectedSampleConfigHeader
	expected += fmt.Sprintf(s.expectedFullNetplanYaml, s.jujuNetplanFile)
	expected += fmt.Sprintf(s.expectedSampleConfigWriting, s.systemNetworkInterfacesFile)
	expected += fmt.Sprintf(s.expectedSampleUserData, s.systemNetworkInterfacesFile, s.networkInterfacesPythonFile, s.systemNetworkInterfacesFile)
	assertUserData(c, cloudConf, expected)
}

func (s *NetworkUbuntuSuite) TestAddNetworkConfigFallbackConfig(c *gc.C) {
	netConfig := container.BridgeNetworkConfig("foo", 0, nil)
	cloudConf, err := cloudinit.New("xenial")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudConf, gc.NotNil)
	err = cloudConf.AddNetworkConfig(netConfig.Interfaces)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudConf, gc.NotNil)
	expected := s.expectedSampleConfigHeader
	expected += fmt.Sprintf(s.expectedBaseNetplanYaml, s.jujuNetplanFile)
	expected += fmt.Sprintf(s.expectedFallbackConfig, s.systemNetworkInterfacesFile, s.systemNetworkInterfacesFile)
	expected += fmt.Sprintf(s.expectedSampleUserData, s.systemNetworkInterfacesFile, s.networkInterfacesPythonFile)
	assertUserData(c, cloudConf, expected)
}

func CloudInitDataExcludingOutputSection(data string) []string {
	// Extract the "#cloud-config" header and all lines between
	// from the "bootcmd" section up to (but not including) the
	// "output" sections to match against expected. But we cannot
	// possibly handle all the /other/ output that may be added by
	// CloudInitUserData() in the future, so we also truncate at
	// the first runcmd which now happens to include the runcmd's
	// added for raising the network interfaces captured in
	// expectedFallbackUserData. However, the other tests above do
	// check for that output.

	var linesToMatch []string
	seenBootcmd := false
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, "#cloud-config") {
			linesToMatch = append(linesToMatch, line)
			continue
		}

		if strings.HasPrefix(line, "bootcmd:") {
			seenBootcmd = true
		}

		if strings.HasPrefix(line, "output:") && seenBootcmd {
			break
		}

		if seenBootcmd {
			linesToMatch = append(linesToMatch, line)
		}
	}

	return linesToMatch
}

func assertUserData(c *gc.C, cloudConf cloudinit.CloudConfig, expected string) {
	data, err := cloudConf.RenderYAML()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, expected)

	// Make sure it's valid YAML as well.
	out := make(map[string]interface{})
	err = yaml.Unmarshal(data, &out)
	c.Assert(err, jc.ErrorIsNil)
	if len(cloudConf.BootCmds()) > 0 {
		outcmds := out["bootcmd"].([]interface{})
		confcmds := cloudConf.BootCmds()
		c.Assert(len(outcmds), gc.Equals, len(confcmds))
		for i := range outcmds {
			c.Assert(outcmds[i].(string), gc.Equals, confcmds[i])
		}
	} else {
		c.Assert(out["bootcmd"], gc.IsNil)
	}
}

func (s *NetworkUbuntuSuite) TestENIScriptSimple(c *gc.C) {
	cmd := s.createMockCommand(c, []string{simpleENIFileIPOutput})
	s.runENIScriptWithAllPythons(c, cmd, simpleENIFile, simpleENIFileExpected, 0, 1)
}

func (s *NetworkUbuntuSuite) TestENIScriptUnknownMAC(c *gc.C) {
	cmd := s.createMockCommand(c, []string{unknownMACENIFileIPOutput})
	s.runENIScriptWithAllPythons(c, cmd, unknownMACENIFile, unknownMACENIFileExpected, 0, 1)
}

func (s *NetworkUbuntuSuite) TestENIScriptHotplugFail(c *gc.C) {
	cmd := s.createMockCommand(c, []string{hotplugENIFileIPOutputPre})
	s.runENIScriptWithAllPythons(c, cmd, hotplugENIFile, hotplugENIFileExpectedFail, 0, 3)
}

func (s *NetworkUbuntuSuite) TestENIScriptHotplugTooLate(c *gc.C) {
	for _, python := range s.pythonVersions {
		c.Logf("test using %s", python)
		ipCommand := s.createMockCommand(c, []string{hotplugENIFileIPOutputPre, hotplugENIFileIPOutputPre, hotplugENIFileIPOutputPre, hotplugENIFileIPOutputPost})
		s.runENIScript(c, python, ipCommand, hotplugENIFile, hotplugENIFileExpectedFail, 0, 3)
	}
}

func (s *NetworkUbuntuSuite) TestENIScriptHotplug(c *gc.C) {
	for _, python := range s.pythonVersions {
		c.Logf("test using %s", python)
		ipCommand := s.createMockCommand(c, []string{hotplugENIFileIPOutputPre, hotplugENIFileIPOutputPre, hotplugENIFileIPOutputPost})
		s.runENIScript(c, python, ipCommand, hotplugENIFile, hotplugENIFileExpected, 0, 3)
	}
}

func (s *NetworkUbuntuSuite) runENIScriptWithAllPythons(c *gc.C, ipCommand, input, expectedOutput string, wait, retries int) {
	for _, python := range s.pythonVersions {
		c.Logf("test using %s", python)
		s.runENIScript(c, python, ipCommand, input, expectedOutput, wait, retries)
	}

}

func (s *NetworkUbuntuSuite) runENIScript(c *gc.C, pythonBinary, ipCommand, input, expectedOutput string, wait, retries int) {
	dataFile := filepath.Join(s.tempFolder, "interfaces")
	dataOutFile := filepath.Join(s.tempFolder, "interfaces.out")
	dataBakFile := filepath.Join(s.tempFolder, "interfaces.bak")
	templFile := filepath.Join(s.tempFolder, "interfaces.templ")
	scriptFile := filepath.Join(s.tempFolder, "script.py")

	err := ioutil.WriteFile(dataFile, []byte(s.originalSystemNetworkInterfaces), 0644)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Can't write interfaces file"))

	err = ioutil.WriteFile(templFile, []byte(input), 0644)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Can't write interfaces.templ file"))

	err = ioutil.WriteFile(scriptFile, []byte(cloudinit.NetworkInterfacesScript), 0755)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Can't write script file"))

	script := fmt.Sprintf("%q %q --interfaces-file %q --output-file %q --command %q --wait %d --retries %d",
		pythonBinary, scriptFile, dataFile, dataOutFile, ipCommand, wait, retries)
	result, err := exec.RunCommands(exec.RunParams{Commands: script})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("script failed unexpectedly - %s", result))
	c.Logf("%s\n%s\n", string(result.Stdout), string(result.Stderr))

	for file, expected := range map[string]string{
		dataBakFile: s.originalSystemNetworkInterfaces,
		dataOutFile: expectedOutput,
		dataFile:    s.originalSystemNetworkInterfaces,
	} {
		data, err := ioutil.ReadFile(file)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("can't open %q file: %s", file, err))
		output := string(data)
		c.Assert(output, gc.Equals, expected)
	}
}

func (s *NetworkUbuntuSuite) createMockCommand(c *gc.C, outputs []string) string {
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	baseName := hex.EncodeToString(randBytes)
	basePath := filepath.Join(s.tempFolder, fmt.Sprintf("%s.%d", baseName, 0))
	script := fmt.Sprintf("#!/bin/bash\ncat %s\n", basePath)

	lastFile := ""
	for i, output := range outputs {
		dataFile := filepath.Join(s.tempFolder, fmt.Sprintf("%s.%d", baseName, i))
		err := ioutil.WriteFile(dataFile, []byte(output), 0644)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("can't write mock file"))
		if lastFile != "" {
			script += fmt.Sprintf("mv %q %q || true\n", dataFile, lastFile)
		}
		lastFile = dataFile
	}

	scriptPath := filepath.Join(s.tempFolder, fmt.Sprintf("%s.sh", baseName))
	err := ioutil.WriteFile(scriptPath, []byte(script), 0755)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("can't write script file"))
	return scriptPath
}

const simpleENIFile = `auto lo
interface lo inet loopback

auto {ethe0_db_55_e4_1d_5b}
iface {ethe0_db_55_e4_1d_5b} inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 1.2.3.1

auto {ethe0_db_55_e4_1a_5b}
iface {ethe0_db_55_e4_1a_5b} inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 1.2.3.1

auto {ethe0_db_55_e4_1a_5d}
iface {ethe0_db_55_e4_1a_5d} inet static
    address 1.2.3.6
    netmask 255.255.255.0
    gateway 1.2.3.1
`

const simpleENIFileIPOutput = `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1\    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: eno1: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc pfifo_fast state DOWN mode DEFAULT group default qlen 1000\    link/ether e0:db:55:e4:1d:5b brd ff:ff:ff:ff:ff:ff
3: eno2: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc pfifo_fast state DOWN mode DEFAULT group default qlen 1000\    link/ether e0:db:55:e4:1a:5b brd ff:ff:ff:ff:ff:ff
3: eno3@if0: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc pfifo_fast state DOWN mode DEFAULT group default qlen 1000\    link/ether e0:db:55:e4:1a:5d brd ff:ff:ff:ff:ff:ff
`

const simpleENIFileExpected = `auto lo
interface lo inet loopback

auto eno1
iface eno1 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 1.2.3.1

auto eno2
iface eno2 inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 1.2.3.1

auto eno3
iface eno3 inet static
    address 1.2.3.6
    netmask 255.255.255.0
    gateway 1.2.3.1
`

const unknownMACENIFile = `auto lo
interface lo inet loopback

auto {ethe0_db_55_e4_1d_5b}
iface {ethe0_db_55_e4_1d_5b} inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 1.2.3.1

auto {ethe3_db_55_e4_1d_5b}
iface {ethe3_db_55_e4_1d_5b} inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 1.2.3.1
`

const unknownMACENIFileIPOutput = `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1\    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: eno1: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc pfifo_fast state DOWN mode DEFAULT group default qlen 1000\    link/ether e0:db:55:e4:1d:5b brd ff:ff:ff:ff:ff:ff
`

const unknownMACENIFileExpected = `auto lo
interface lo inet loopback

auto eno1
iface eno1 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 1.2.3.1

auto ethe3_db_55_e4_1d_5b
iface ethe3_db_55_e4_1d_5b inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 1.2.3.1
`

const hotplugENIFile = `auto lo
interface lo inet loopback

auto {ethe0_db_55_e4_1d_5b}
iface {ethe0_db_55_e4_1d_5b} inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 1.2.3.1

auto {ethe0_db_55_e4_1a_5b}
iface {ethe0_db_55_e4_1a_5b} inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 1.2.3.1
`

const hotplugENIFileIPOutputPre = `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1\    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: eno1: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc pfifo_fast state DOWN mode DEFAULT group default qlen 1000\    link/ether e0:db:55:e4:1d:5b brd ff:ff:ff:ff:ff:ff
`

const hotplugENIFileIPOutputPost = `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1\    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: eno1: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc pfifo_fast state DOWN mode DEFAULT group default qlen 1000\    link/ether e0:db:55:e4:1d:5b brd ff:ff:ff:ff:ff:ff
32: eno2: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc pfifo_fast state DOWN mode DEFAULT group default qlen 1000\    link/ether e0:db:55:e4:1a:5b brd ff:ff:ff:ff:ff:ff
`

const hotplugENIFileExpected = `auto lo
interface lo inet loopback

auto eno1
iface eno1 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 1.2.3.1

auto eno2
iface eno2 inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 1.2.3.1
`

const hotplugENIFileExpectedFail = `auto lo
interface lo inet loopback

auto eno1
iface eno1 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 1.2.3.1

auto ethe0_db_55_e4_1a_5b
iface ethe0_db_55_e4_1a_5b inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 1.2.3.1
`
