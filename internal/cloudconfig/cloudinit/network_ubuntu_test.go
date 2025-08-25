// Copyright 2018 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"
	"gopkg.in/yaml.v2"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/testing"
)

type NetworkUbuntuSuite struct {
	testing.BaseSuite

	jujuNetplanFile string

	fakeInterfaces corenetwork.InterfaceInfos

	expectedSampleConfigHeader string
	expectedSampleUserData     string
	expectedFullNetplanYaml    string
	expectedFullNetplan        string
	tempFolder                 string
	pythonVersions             []string
}

func TestNetworkUbuntuSuite(t *stdtesting.T) {
	tc.Run(t, &NetworkUbuntuSuite{})
}

func (s *NetworkUbuntuSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.tempFolder = c.MkDir()
	netplanFolder := c.MkDir()
	s.jujuNetplanFile = filepath.Join(netplanFolder, "79-juju.yaml")

	s.fakeInterfaces = corenetwork.InterfaceInfos{{
		InterfaceName: "any0",
		ConfigType:    corenetwork.ConfigStatic,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("0.1.2.3", corenetwork.WithCIDR("0.1.2.0/24")).AsProviderAddress()},
		DNSServers:       []string{"ns1.invalid", "ns2.invalid"},
		DNSSearchDomains: []string{"foo", "bar"},
		GatewayAddress:   corenetwork.NewMachineAddress("0.1.2.1").AsProviderAddress(),
		MACAddress:       "aa:bb:cc:dd:ee:f0",
		MTU:              8317,
	}, {
		InterfaceName: "any1",
		ConfigType:    corenetwork.ConfigStatic,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("0.2.2.4", corenetwork.WithCIDR("0.2.2.0/24")).AsProviderAddress()},
		DNSServers:       []string{"ns1.invalid", "ns2.invalid"},
		DNSSearchDomains: []string{"foo", "bar"},
		GatewayAddress:   corenetwork.NewMachineAddress("0.2.2.1").AsProviderAddress(),
		MACAddress:       "aa:bb:cc:dd:ee:f1",
		Routes: []corenetwork.Route{{
			DestinationCIDR: "0.5.6.0/24",
			GatewayIP:       "0.2.2.1",
			Metric:          50,
		}},
	}, {
		InterfaceName: "any2",
		ConfigType:    corenetwork.ConfigDHCP,
		MACAddress:    "aa:bb:cc:dd:ee:f2",
		NoAutoStart:   true,
	}, {
		InterfaceName: "any3",
		ConfigType:    corenetwork.ConfigDHCP,
		MACAddress:    "aa:bb:cc:dd:ee:f3",
		NoAutoStart:   false,
	}, {
		InterfaceName: "any4",
		ConfigType:    corenetwork.ConfigManual,
		MACAddress:    "aa:bb:cc:dd:ee:f4",
		NoAutoStart:   true,
	}, {
		InterfaceName: "any5",
		ConfigType:    corenetwork.ConfigStatic,
		MACAddress:    "aa:bb:cc:dd:ee:f5",
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("2001:db8::dead:beef", corenetwork.WithCIDR("2001:db8::/64")).AsProviderAddress()},
		GatewayAddress: corenetwork.NewMachineAddress("2001:db8::dead:f00").AsProviderAddress(),
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
	c.Assert(s.pythonVersions, tc.Not(tc.HasLen), 0)

	s.expectedSampleConfigHeader = `#cloud-config
bootcmd:
`

	s.expectedSampleUserData = `
- |
  echo "Applying netplan configuration."
  netplan generate
  netplan apply
  for i in {1..5}; do
    hostip=$(hostname -I)
    if [ -z "$hostip" ]; then
      sleep 1
    else
      echo "Got IP addresses $hostip"
      break
    fi
  done
`[1:]

	s.expectedFullNetplanYaml = `
- install -D -m 644 /dev/null '%[1]s'
- |-
  echo 'network:
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

	s.PatchValue(cloudinit.JujuNetplanFile, s.jujuNetplanFile)
}

func (s *NetworkUbuntuSuite) TestGenerateNetplan(c *tc.C) {
	data, err := cloudinit.GenerateNetplan(nil, true)
	c.Assert(err, tc.ErrorMatches, "missing container network config")
	c.Assert(data, tc.Equals, "")

	netConfig := container.BridgeNetworkConfig(0, s.fakeInterfaces)
	data, err = cloudinit.GenerateNetplan(netConfig.Interfaces, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(data, tc.Equals, s.expectedFullNetplan)
}

func (s *NetworkUbuntuSuite) TestGenerateNetplanSkipIPv6LinkLocalDNS(c *tc.C) {
	s.fakeInterfaces = corenetwork.InterfaceInfos{{
		InterfaceName: "any5",
		ConfigType:    corenetwork.ConfigStatic,
		MACAddress:    "aa:bb:cc:dd:ee:f5",
		NoAutoStart:   false,
		DNSServers:    []string{"fe80:db8::dead:beef"},
		Addresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress(
				"2001:db8::dead:beef", corenetwork.WithCIDR("2001:db8::/64")).AsProviderAddress(),
		},
		GatewayAddress: corenetwork.NewMachineAddress("2001:db8::dead:f00").AsProviderAddress(),
	}}

	netConfig := container.BridgeNetworkConfig(0, s.fakeInterfaces)
	data, err := cloudinit.GenerateNetplan(netConfig.Interfaces, true)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(data, tc.Equals, `
network:
  version: 2
  ethernets:
    any5:
      match:
        macaddress: aa:bb:cc:dd:ee:f5
      addresses:
      - 2001:db8::dead:beef/64
      gateway6: 2001:db8::dead:f00
`[1:])
}

func (s *NetworkUbuntuSuite) TestGenerateNetplanWithoutMatchStanza(c *tc.C) {
	s.fakeInterfaces = corenetwork.InterfaceInfos{{
		InterfaceName: "any5",
		ConfigType:    corenetwork.ConfigStatic,
		MACAddress:    "aa:bb:cc:dd:ee:f5",
		Addresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("10.0.0.5", corenetwork.WithCIDR("10.0.0.0/8")).AsProviderAddress()},
	}}

	netConfig := container.BridgeNetworkConfig(0, s.fakeInterfaces)
	data, err := cloudinit.GenerateNetplan(netConfig.Interfaces, false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(data, tc.Equals, `
network:
  version: 2
  ethernets:
    any5:
      addresses:
      - 10.0.0.5/8
`[1:])
}

func (s *NetworkUbuntuSuite) TestAddNetworkConfigSampleConfig(c *tc.C) {
	netConfig := container.BridgeNetworkConfig(0, s.fakeInterfaces)
	cloudConf, err := cloudinit.New("ubuntu", cloudinit.WithNetplanMACMatch(true))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloudConf, tc.NotNil)
	err = cloudConf.AddNetworkConfig(netConfig.Interfaces)
	c.Assert(err, tc.ErrorIsNil)

	expected := s.expectedSampleConfigHeader
	expected += fmt.Sprintf(s.expectedFullNetplanYaml, s.jujuNetplanFile)
	expected += s.expectedSampleUserData
	assertUserData(c, cloudConf, expected)
}

func assertUserData(c *tc.C, cloudConf cloudinit.CloudConfig, expected string) {
	data, err := cloudConf.RenderYAML()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, expected)

	// Make sure it's valid YAML as well.
	out := make(map[string]interface{})
	err = yaml.Unmarshal(data, &out)
	c.Assert(err, tc.ErrorIsNil)
	if len(cloudConf.BootCmds()) > 0 {
		outcmds := out["bootcmd"].([]interface{})
		confcmds := cloudConf.BootCmds()
		c.Assert(len(outcmds), tc.Equals, len(confcmds))
		for i := range outcmds {
			c.Assert(outcmds[i].(string), tc.Equals, confcmds[i])
		}
	} else {
		c.Assert(out["bootcmd"], tc.IsNil)
	}
}

func (s *NetworkUbuntuSuite) TestPrepareNetworkConfigFromInterfacesBadCIDRError(c *tc.C) {
	s.fakeInterfaces[0].Addresses[0].CIDR = "invalid"
	_, err := cloudinit.PrepareNetworkConfigFromInterfaces(s.fakeInterfaces)
	c.Assert(err, tc.ErrorMatches, `invalid CIDR address: invalid`)
}

func (s *NetworkUbuntuSuite) TestGenerateNetplanBadAddressError(c *tc.C) {
	s.fakeInterfaces[0].Addresses[0].Value = "invalid"
	_, err := cloudinit.PrepareNetworkConfigFromInterfaces(s.fakeInterfaces)
	c.Assert(err, tc.ErrorMatches, `cannot parse IP address "invalid"`)
}
