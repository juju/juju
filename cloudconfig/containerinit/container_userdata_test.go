// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package containerinit_test

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/containerinit"
	"github.com/juju/juju/container"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type UserDataSuite struct {
	testing.BaseSuite

	networkInterfacesPythonFile string
	systemNetworkInterfacesFile string

	fakeInterfaces []network.InterfaceInfo

	expectedSampleConfig        string
	expectedSampleConfigWriting string
	expectedSampleUserData      string
	expectedFallbackConfig      string
	expectedBaseConfig          string
	expectedFallbackUserData    string
	tempFolder                  string
	pythonVersions              []string
}

var _ = gc.Suite(&UserDataSuite{})

func (s *UserDataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.tempFolder = c.MkDir()
	networkFolder := c.MkDir()
	s.systemNetworkInterfacesFile = filepath.Join(networkFolder, "system-interfaces")
	s.networkInterfacesPythonFile = filepath.Join(networkFolder, "system-interfaces.py")
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
		NoAutoStart:   true,
	}, {
		InterfaceName: "any3",
		ConfigType:    network.ConfigDHCP,
		NoAutoStart:   false,
	}, {
		InterfaceName: "any4",
		ConfigType:    network.ConfigManual,
		NoAutoStart:   true,
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

	s.expectedSampleConfigWriting = `#cloud-config
bootcmd:
- install -D -m 644 /dev/null '%[1]s'
- |-
  printf '%%s\n' '
  auto lo {ethaa_bb_cc_dd_ee_f0} {ethaa_bb_cc_dd_ee_f1} {eth}

  iface lo inet loopback
    dns-nameservers ns1.invalid ns2.invalid
    dns-search bar foo

  iface {ethaa_bb_cc_dd_ee_f0} inet static
    address 0.1.2.3/24
    gateway 0.1.2.1

  iface {ethaa_bb_cc_dd_ee_f1} inet static
    address 0.2.2.4/24
    post-up ip route add 0.5.6.0/24 via 0.2.2.1 metric 50
    pre-down ip route del 0.5.6.0/24 via 0.2.2.1 metric 50

  iface {eth} inet dhcp

  iface {eth} inet dhcp

  iface {eth} inet dhcp
  ' > '%[1]s'
`
	s.expectedSampleConfig = `
auto lo {ethaa_bb_cc_dd_ee_f0} {ethaa_bb_cc_dd_ee_f1} {eth}

iface lo inet loopback
  dns-nameservers ns1.invalid ns2.invalid
  dns-search bar foo

iface {ethaa_bb_cc_dd_ee_f0} inet static
  address 0.1.2.3/24
  gateway 0.1.2.1

iface {ethaa_bb_cc_dd_ee_f1} inet static
  address 0.2.2.4/24
  post-up ip route add 0.5.6.0/24 via 0.2.2.1 metric 50
  pre-down ip route del 0.5.6.0/24 via 0.2.2.1 metric 50

iface {eth} inet dhcp

iface {eth} inet dhcp

iface {eth} inet dhcp
`
	s.expectedSampleUserData = `
- install -D -m 744 /dev/null '%[2]s'
- |-
  printf '%%s\n' 'import subprocess, re, argparse, os, time
  from string import Formatter
  INTERFACES_FILE="/etc/network/interfaces"
  IP_LINE = re.compile(r"^\d: (.*?):")
  IP_HWADDR = re.compile(r".*link/ether ((\w{2}|:){11})")
  COMMAND = "ip -oneline link"
  RETRIES = 3
  WAIT = 5

  # Python3 vs Python2
  try:
      strdecode = str.decode
  except AttributeError:
      strdecode = str

  def ip_parse(ip_output):
      """parses the output of the ip command
      and returns a hwaddr->nic-name dict"""
      devices = dict()
      print("Parsing ip command output %%s" %% ip_output)
      for ip_line in ip_output:
          ip_line_str = strdecode(ip_line, '"'"'utf-8'"'"')
          match = IP_LINE.match(ip_line_str)
          if match is None:
              continue
          nic_name = match.group(1)
          match = IP_HWADDR.match(ip_line_str)
          if match is None:
              continue
          nic_hwaddr = match.group(1)
          devices[nic_hwaddr]=nic_name
      print("Found the following devices: %%s" %% str(devices))
      return devices

  def replace_ethernets(interfaces_file, devices, fail_on_missing):
      """check if the contents of interfaces_file contain template
      keys corresponding to hwaddresses and replace them with
      the proper device name"""
      with open(interfaces_file, "r") as intf_file:
          interfaces = intf_file.read()

      formatter = Formatter()
      hwaddrs = [v[1] for v in formatter.parse(interfaces) if v[1]]
      print("Found the following hwaddrs: %%s" %% str(hwaddrs))
      device_replacements = dict()
      for hwaddr in hwaddrs:
          hwaddr_clean = hwaddr[3:].replace("_", ":")
          if devices.get(hwaddr_clean, None):
              device_replacements[hwaddr] = devices[hwaddr_clean]
          else:
              if fail_on_missing:
                  print("Can'"'"'t find device with MAC %%s, will retry" %% hwaddr_clean)
                  return False
              else:
                  print("WARNING: Can'"'"'t find device with MAC %%s when expected" %% hwaddr_clean)
                  device_replacements[hwaddr] = hwaddr
      formatted = interfaces.format(**device_replacements)
      print("Used the values in: %%s\nto fix the interfaces file:\n%%s\ninto\n%%s" %%
             (str(device_replacements), str(interfaces), str(formatted)))

      with open(interfaces_file + ".tmp", "w") as intf_file:
          intf_file.write(formatted)

      os.rename(interfaces_file, interfaces_file + ".bak")
      os.rename(interfaces_file + ".tmp", interfaces_file)
      return True

  def main():
      parser = argparse.ArgumentParser()
      parser.add_argument('"'"'--interfaces_file'"'"', dest = '"'"'intf_file'"'"', default = INTERFACES_FILE)
      parser.add_argument('"'"'--command'"'"', dest = '"'"'command'"'"', default = COMMAND)
      parser.add_argument('"'"'--retries'"'"', dest = '"'"'retries'"'"', default = RETRIES)
      parser.add_argument('"'"'--wait'"'"', dest = '"'"'wait'"'"', default = WAIT)
      args = parser.parse_args()
      retries = int(args.retries)
      for tries in range(retries):
          ip_output = ip_parse(subprocess.check_output(args.command.split()).splitlines())
          if replace_ethernets(args.intf_file, ip_output, (tries != retries - 1)):
               break
          else:
               time.sleep(float(args.wait))

  if __name__ == "__main__":
      main()
  ' > '%[2]s'
- |2

  if [ -f /usr/bin/python ]; then
      python %[2]s --interfaces_file %[1]s
  else
      python3 %[2]s --interfaces_file %[1]s
  fi
`[1:]

	s.expectedFallbackConfig = `#cloud-config
bootcmd:
- install -D -m 644 /dev/null '%[1]s'
- |-
  printf '%%s\n' '
  auto lo {eth}

  iface lo inet loopback

  iface {eth} inet dhcp
  ' > '%[1]s'
`
	s.expectedBaseConfig = `
auto lo {eth}

iface lo inet loopback

iface {eth} inet dhcp
`
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

	s.PatchValue(containerinit.NetworkInterfacesFile, s.systemNetworkInterfacesFile)
	s.PatchValue(containerinit.SystemNetworkInterfacesFile, s.systemNetworkInterfacesFile)
}

func (s *UserDataSuite) TestGenerateNetworkConfig(c *gc.C) {
	data, err := containerinit.GenerateNetworkConfig(nil)
	c.Assert(err, gc.ErrorMatches, "missing container network config")
	c.Assert(data, gc.Equals, "")

	netConfig := container.BridgeNetworkConfig("foo", 0, nil)
	data, err = containerinit.GenerateNetworkConfig(netConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.Equals, s.expectedBaseConfig)

	// Test with all interface types.
	netConfig = container.BridgeNetworkConfig("foo", 0, s.fakeInterfaces)
	data, err = containerinit.GenerateNetworkConfig(netConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.Equals, s.expectedSampleConfig)
}

func (s *UserDataSuite) TestNewCloudInitConfigWithNetworksSampleConfig(c *gc.C) {
	netConfig := container.BridgeNetworkConfig("foo", 0, s.fakeInterfaces)
	cloudConf, err := containerinit.NewCloudInitConfigWithNetworks("quantal", netConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudConf, gc.NotNil)

	expected := fmt.Sprintf(s.expectedSampleConfigWriting, s.systemNetworkInterfacesFile)
	expected += fmt.Sprintf(s.expectedSampleUserData, s.systemNetworkInterfacesFile, s.networkInterfacesPythonFile, s.systemNetworkInterfacesFile)
	assertUserData(c, cloudConf, expected)
}

func (s *UserDataSuite) TestNewCloudInitConfigWithNetworksFallbackConfig(c *gc.C) {
	netConfig := container.BridgeNetworkConfig("foo", 0, nil)
	cloudConf, err := containerinit.NewCloudInitConfigWithNetworks("quantal", netConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudConf, gc.NotNil)
	expected := fmt.Sprintf(s.expectedFallbackConfig, s.systemNetworkInterfacesFile, s.systemNetworkInterfacesFile)
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
	for _, line := range strings.Split(string(data), "\n") {
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

// TestCloudInitUserDataNoNetworkConfig tests that no network-interfaces, or
// related data, appear in user-data when no networkConfig is passed to
// CloudInitUserData.
func (s *UserDataSuite) TestCloudInitUserDataNoNetworkConfig(c *gc.C) {
	instanceConfig, err := containertesting.MockMachineConfig("1/lxd/0")
	c.Assert(err, jc.ErrorIsNil)
	data, err := containerinit.CloudInitUserData(instanceConfig, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.NotNil)

	linesToMatch := CloudInitDataExcludingOutputSection(string(data))

	c.Assert(strings.Join(linesToMatch, "\n"), gc.Equals, "#cloud-config")
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
		for i, _ := range outcmds {
			c.Assert(outcmds[i].(string), gc.Equals, confcmds[i])
		}
	} else {
		c.Assert(out["bootcmd"], gc.IsNil)
	}
}

func (s *UserDataSuite) TestENIScriptSimple(c *gc.C) {
	cmd := s.createMockCommand(c, []string{simpleENIFileIPOutput})
	s.runENIScriptWithAllPythons(c, cmd, simpleENIFile, simpleENIFileExpected, 0, 1)
}

func (s *UserDataSuite) TestENIScriptUnknownMAC(c *gc.C) {
	cmd := s.createMockCommand(c, []string{unknownMACENIFileIPOutput})
	s.runENIScriptWithAllPythons(c, cmd, unknownMACENIFile, unknownMACENIFileExpected, 0, 1)
}

func (s *UserDataSuite) TestENIScriptHotplugFail(c *gc.C) {
	cmd := s.createMockCommand(c, []string{hotplugENIFileIPOutputPre})
	s.runENIScriptWithAllPythons(c, cmd, hotplugENIFile, hotplugENIFileExpectedFail, 0, 3)
}

func (s *UserDataSuite) TestENIScriptHotplugTooLate(c *gc.C) {
	for _, python := range s.pythonVersions {
		c.Logf("test using %s", python)
		ipCommand := s.createMockCommand(c, []string{hotplugENIFileIPOutputPre, hotplugENIFileIPOutputPre, hotplugENIFileIPOutputPre, hotplugENIFileIPOutputPost})
		s.runENIScript(c, python, ipCommand, hotplugENIFile, hotplugENIFileExpectedFail, 0, 3)
	}
}

func (s *UserDataSuite) TestENIScriptHotplug(c *gc.C) {
	for _, python := range s.pythonVersions {
		c.Logf("test using %s", python)
		ipCommand := s.createMockCommand(c, []string{hotplugENIFileIPOutputPre, hotplugENIFileIPOutputPre, hotplugENIFileIPOutputPost})
		s.runENIScript(c, python, ipCommand, hotplugENIFile, hotplugENIFileExpected, 0, 3)
	}
}

func (s *UserDataSuite) runENIScriptWithAllPythons(c *gc.C, ipCommand, input, expectedOutput string, wait, retries int) {
	for _, python := range s.pythonVersions {
		c.Logf("test using %s", python)
		s.runENIScript(c, python, ipCommand, input, expectedOutput, wait, retries)
	}

}

func (s *UserDataSuite) runENIScript(c *gc.C, pythonBinary, ipCommand, input, expectedOutput string, wait, retries int) {
	dataFile := filepath.Join(s.tempFolder, "interfaces")
	scriptFile := filepath.Join(s.tempFolder, "script.py")

	err := ioutil.WriteFile(dataFile, []byte(input), 0644)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Can't write interfaces file"))

	err = ioutil.WriteFile(scriptFile, []byte(containerinit.PopulateNetworkInterfacesScript()), 0755)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Can't write script file"))

	script := fmt.Sprintf("%q %q --interfaces_file %q --command %q --wait %d --retries %d", pythonBinary, scriptFile, dataFile, ipCommand, wait, retries)
	result, err := exec.RunCommands(exec.RunParams{Commands: script})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("script failed unexpectedly - %s", result))
	data, err := ioutil.ReadFile(dataFile)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("can't open parsed interfaces file"))
	output := string(data)
	c.Assert(output, gc.Equals, expectedOutput)
}

func (s *UserDataSuite) createMockCommand(c *gc.C, outputs []string) string {
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
`

const simpleENIFileIPOutput = `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1\    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: eno1: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc pfifo_fast state DOWN mode DEFAULT group default qlen 1000\    link/ether e0:db:55:e4:1d:5b brd ff:ff:ff:ff:ff:ff
3: eno2: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc pfifo_fast state DOWN mode DEFAULT group default qlen 1000\    link/ether e0:db:55:e4:1a:5b brd ff:ff:ff:ff:ff:ff
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
3: eno2: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc pfifo_fast state DOWN mode DEFAULT group default qlen 1000\    link/ether e0:db:55:e4:1a:5b brd ff:ff:ff:ff:ff:ff
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
