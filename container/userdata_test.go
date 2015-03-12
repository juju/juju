// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudinit"
	"github.com/juju/juju/container"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type UserDataSuite struct {
	testing.BaseSuite

	networkInterfacesFile string
}

var _ = gc.Suite(&UserDataSuite{})

func (s *UserDataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.networkInterfacesFile = filepath.Join(c.MkDir(), "interfaces")
	s.PatchValue(container.NetworkInterfacesFile, s.networkInterfacesFile)
}

func (s *UserDataSuite) TestNewCloudInitConfigWithNetworks(c *gc.C) {
	ifaces := []network.InterfaceInfo{{
		InterfaceName:  "eth0",
		CIDR:           "0.1.2.0/24",
		ConfigType:     network.ConfigStatic,
		NoAutoStart:    false,
		Address:        network.NewAddress("0.1.2.3", network.ScopeUnknown),
		DNSServers:     network.NewAddresses("ns1.invalid", "ns2.invalid"),
		GatewayAddress: network.NewAddress("0.1.2.1", network.ScopeUnknown),
	}, {
		InterfaceName: "eth1",
		ConfigType:    network.ConfigDHCP,
		NoAutoStart:   true,
	}}
	netConfig := container.BridgeNetworkConfig("foo", ifaces)
	cloudConf, err := container.NewCloudInitConfigWithNetworks(netConfig)
	c.Assert(err, jc.ErrorIsNil)
	expected := `
#cloud-config
bootcmd:
- install -D -m 644 /dev/null '`[1:] + s.networkInterfacesFile + `'
- |-
  printf '%s\n' '
  # loopback interface
  auto lo
  iface lo inet loopback

  # interface "eth0"
  auto eth0
  iface eth0 inet static
      address 0.1.2.3
      netmask 0.1.2.0/24
      dns-nameservers ns1.invalid ns2.invalid
      pre-up ip route add 0.1.2.1 dev eth0
      pre-up ip route add default via 0.1.2.1
      post-down ip route del default via 0.1.2.1
      post-down ip route del 0.1.2.1 dev eth0

  # interface "eth1"
  iface eth1 inet dhcp
  ' > '` + s.networkInterfacesFile + `'
`
	assertUserData(c, cloudConf, expected)
}

func (s *UserDataSuite) TestNewCloudInitConfigWithNetworksNoConfig(c *gc.C) {
	netConfig := container.BridgeNetworkConfig("foo", nil)
	cloudConf, err := container.NewCloudInitConfigWithNetworks(netConfig)
	c.Assert(err, jc.ErrorIsNil)
	expected := "#cloud-config\n{}\n"
	assertUserData(c, cloudConf, expected)
}

func (s *UserDataSuite) TestCloudInitUserData(c *gc.C) {
	machineConfig, err := containertesting.MockMachineConfig("1/lxc/0")
	c.Assert(err, jc.ErrorIsNil)
	networkConfig := container.BridgeNetworkConfig("foo", nil)
	data, err := container.CloudInitUserData(machineConfig, networkConfig)
	c.Assert(err, jc.ErrorIsNil)
	// No need to test the exact contents here, as they are already
	// tested separately.
	c.Assert(string(data), jc.HasPrefix, "#cloud-config\n")
}

func assertUserData(c *gc.C, cloudConf *cloudinit.Config, expected string) {
	renderer, err := cloudinit.NewRenderer("quantal")
	c.Assert(err, jc.ErrorIsNil)
	data, err := renderer.Render(cloudConf)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, expected)
}
