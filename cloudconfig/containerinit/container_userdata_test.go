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

	networkInterfacesFile       string
	systemNetworkInterfacesFile string

	fakeInterfaces []network.InterfaceInfo

	expectedSampleConfig     string
	expectedSampleUserData   string
	expectedFallbackConfig   string
	expectedFallbackUserData string
}

var _ = gc.Suite(&UserDataSuite{})

func configToUserData(config string) string {
	userData := `#cloud-config
bootcmd:
- install -D -m 644 /dev/null '%[1]s'
- |-
  printf '%%s\n' '`

	for _, line := range strings.Split(strings.TrimSuffix(config, "\n"), "\n") {
		if line != "" {
			userData += "  " + line + "\n"
		} else {
			userData += "\n"
		}
	}

	userData += `  ' > '%[1]s'
runcmd:
- |-
  if [ -f %[1]s ]; then
      ifdown -a
      sleep 1.5
      if ifup -a --interfaces=%[1]s; then
          cp %[2]s %[2]s-orig
          cp %[1]s %[2]s
      else
          ifup -a
      fi
  fi
`
	return userData
}

func (s *UserDataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.networkInterfacesFile = filepath.Join(c.MkDir(), "juju-interfaces")
	s.systemNetworkInterfacesFile = filepath.Join(c.MkDir(), "system-interfaces")
	s.fakeInterfaces = []network.InterfaceInfo{{
		InterfaceName:    "eth0",
		CIDR:             "0.1.2.0/24",
		ConfigType:       network.ConfigStatic,
		NoAutoStart:      false,
		Address:          network.NewAddress("0.1.2.3"),
		DNSServers:       network.NewAddresses("ns1.invalid", "ns2.invalid"),
		DNSSearchDomains: []string{"foo", "bar"},
		GatewayAddress:   network.NewAddress("0.1.2.1"),
		MACAddress:       "aa:bb:cc:dd:ee:f0",
	}, {
		InterfaceName: "eth1",
		CIDR:          "0.1.2.0/24",
		ConfigType:    network.ConfigStatic,
		NoAutoStart:   false,
		Address:       network.NewAddress("0.1.2.4"),
		MACAddress:    "aa:bb:cc:dd:ee:f0",
	}, {
		InterfaceName: "eth2",
		ConfigType:    network.ConfigDHCP,
		NoAutoStart:   true,
	}, {
		InterfaceName: "eth3",
		ConfigType:    network.ConfigDHCP,
		NoAutoStart:   false,
	}, {
		InterfaceName: "eth4",
		ConfigType:    network.ConfigManual,
		NoAutoStart:   true,
	}}
	s.expectedSampleConfig = `
auto eth0 eth1 eth3 lo

iface lo inet loopback
  dns-nameservers ns1.invalid ns2.invalid
  dns-search bar foo

iface eth0 inet static
  address 0.1.2.3/24
  gateway 0.1.2.1

iface eth1 inet static
  address 0.1.2.4/24

iface eth2 inet dhcp

iface eth3 inet dhcp

iface eth4 inet manual
`
	s.expectedSampleUserData = configToUserData(s.expectedSampleConfig)

	s.expectedFallbackConfig = `
auto eth0 lo

iface lo inet loopback

iface eth0 inet dhcp
`
	s.expectedFallbackUserData = configToUserData(s.expectedFallbackConfig)

	s.PatchValue(containerinit.NetworkInterfacesFile, s.networkInterfacesFile)
	s.PatchValue(containerinit.SystemNetworkInterfacesFile, s.systemNetworkInterfacesFile)
}

func (s *UserDataSuite) TestGenerateNetworkConfig(c *gc.C) {
	data, err := containerinit.GenerateNetworkConfig(nil)
	c.Assert(err, gc.ErrorMatches, "missing container network config")
	c.Assert(data, gc.Equals, "")

	netConfig := container.BridgeNetworkConfig("foo", 0, nil)
	data, err = containerinit.GenerateNetworkConfig(netConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.Equals, s.expectedFallbackConfig)

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

	expected := fmt.Sprintf(s.expectedSampleUserData, s.networkInterfacesFile, s.systemNetworkInterfacesFile)
	assertUserData(c, cloudConf, expected)
}

func (s *UserDataSuite) TestNewCloudInitConfigWithNetworksFallbackConfig(c *gc.C) {
	netConfig := container.BridgeNetworkConfig("foo", 0, nil)
	cloudConf, err := containerinit.NewCloudInitConfigWithNetworks("quantal", netConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudConf, gc.NotNil)
	expected := fmt.Sprintf(s.expectedFallbackUserData, s.networkInterfacesFile, s.systemNetworkInterfacesFile)
	assertUserData(c, cloudConf, expected)
}

func (s *UserDataSuite) TestCloudInitUserDataFallbackConfig(c *gc.C) {
	instanceConfig, err := containertesting.MockMachineConfig("1/lxd/0")
	c.Assert(err, jc.ErrorIsNil)
	networkConfig := container.BridgeNetworkConfig("foo", 0, nil)
	data, err := containerinit.CloudInitUserData(instanceConfig, networkConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.NotNil)

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
	expected := fmt.Sprintf(s.expectedFallbackUserData, s.networkInterfacesFile, s.systemNetworkInterfacesFile)

	var expectedLinesToMatch []string

	for _, line := range strings.Split(expected, "\n") {
		if strings.HasPrefix(line, "runcmd:") {
			break
		}
		expectedLinesToMatch = append(expectedLinesToMatch, line)
	}

	c.Assert(strings.Join(linesToMatch, "\n")+"\n", gc.Equals, strings.Join(expectedLinesToMatch, "\n")+"\n")
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

func (s *UserDataSuite) TestDualNicBug1602054(c *gc.C) {
	networkConfig := container.NetworkConfig{
		NetworkType: "bridge",
		Device:      "lxdbr0", MTU: 0,
		Interfaces: []network.InterfaceInfo{
			{
				DeviceIndex:         0,
				MACAddress:          "00:16:3e:29:de:c5",
				CIDR:                "192.168.200.0/24",
				ProviderId:          "33",
				ProviderSubnetId:    "3",
				ProviderSpaceId:     "",
				ProviderVLANId:      "5001",
				ProviderAddressId:   "216",
				AvailabilityZones:   []string(nil),
				VLANTag:             0,
				InterfaceName:       "eth0",
				ParentInterfaceName: "br-eth0",
				InterfaceType:       "ethernet",
				Disabled:            false,
				NoAutoStart:         false,
				ConfigType:          "static",
				Address:             network.NewScopedAddress("192.168.200.1", network.ScopeCloudLocal),
				DNSServers: []network.Address{
					network.NewScopedAddress("192.168.1.2", network.ScopeCloudLocal),
				},
				MTU:              1500,
				DNSSearchDomains: []string{"maas"},
				GatewayAddress:   network.Address{},
			}, {
				DeviceIndex:         0,
				MACAddress:          "00:16:3e:43:10:2b",
				CIDR:                "192.168.1.0/24",
				ProviderId:          "34",
				ProviderSubnetId:    "1",
				ProviderSpaceId:     "",
				ProviderVLANId:      "0",
				ProviderAddressId:   "218",
				AvailabilityZones:   []string(nil),
				VLANTag:             0,
				InterfaceName:       "eth1",
				ParentInterfaceName: "br-eth1",
				InterfaceType:       "ethernet",
				Disabled:            false,
				NoAutoStart:         false,
				ConfigType:          "static",
				Address:             network.NewScopedAddress("192.168.200.102", network.ScopeCloudLocal),
				DNSServers:          []network.Address{},
				MTU:                 1500,
				DNSSearchDomains:    []string(nil),
				GatewayAddress:      network.NewScopedAddress("192.168.1.1", network.ScopeCloudLocal),
			}},
	}

	data, err := containerinit.GenerateNetworkConfig(&networkConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.NotNil)

	expected := `
auto eth0 eth1 lo

iface lo inet loopback
  dns-nameservers 192.168.1.2
  dns-search maas

iface eth0 inet static
  address 192.168.200.1/24

iface eth1 inet static
  address 192.168.200.102/24
  gateway 192.168.1.1
`
	c.Assert(data, gc.Equals, expected)
}
