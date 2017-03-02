// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package containerinit_test

import (
	"fmt"
	"path/filepath"
	"strings"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
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
}

var _ = gc.Suite(&UserDataSuite{})

func (s *UserDataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

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
  printf '%%s\n' 'import subprocess, re
  from string import Formatter
  INTERFACES_FILE="%[1]s"
  IP_LINE = re.compile(r"^\d: (.*?):")
  IP_HWADDR = re.compile(r".*link/ether ((\w{2}|:){11})")


  def ip_parse(ip_output):
      """parses the output of the ip command
      and returns a hwaddr->nic-name dict"""
      devices = dict()
      print ("parsing ip command output")
      print (ip_output)
      for ip_line in ip_output:
          ip_line_str = str(ip_line, '"'"'utf-8'"'"')
          match = IP_LINE.match(ip_line_str)
          if match is None:
              continue
          nic_name = match.group(1)
          match = IP_HWADDR.match(ip_line_str)
          if match is None:
              continue
          nic_hwaddr = match.group(1)
          devices[nic_hwaddr]=nic_name
      print("found the following devices: " + str(devices))
      return devices

  def replace_ethernets(interfaces_file, devices):
      """check if the contents of interfaces_file contain template
      keys corresponding to hwaddresses and replace them with
      the proper device name"""
      interfaces_file_descriptor = open(interfaces_file, "r")
      interfaces = interfaces_file_descriptor.read()
      formatter = Formatter()
      hwaddrs = [v[1] for v in formatter.parse(interfaces) if v[1]]
      print("found the following hwaddrs: " + str(hwaddrs))
      device_replacements = dict()
      for hwaddr in hwaddrs:
          hwaddr_clean = hwaddr[3:].replace("_", ":")
          if devices.get(hwaddr_clean, None):
              device_replacements[hwaddr] = devices[hwaddr_clean]
      print ("will use the values in:" + str(device_replacements))
      print("to fix the interfaces file:")
      print(str(interfaces))
      formatted = interfaces.format(**device_replacements)
      print ("into")
      print(formatted)
      interfaces_file_descriptor = open(interfaces_file, "w")
      interfaces_file_descriptor.write(formatted)
      interfaces_file_descriptor.close()

  ip_output = ip_parse(subprocess.check_output(["ip", "-oneline", "link"]).splitlines())
  replace_ethernets(INTERFACES_FILE, ip_output)
  ' > '%[2]s'
- |2

  if [ -f /usr/bin/python ]; then
      python /etc/network/interfaces.py
  else
      python3 /etc/network/interfaces.py
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
