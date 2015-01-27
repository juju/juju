// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudinit"
	"github.com/juju/juju/container"
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
	// TODO(dimitern) Test all cases here.
	ifaces := []network.InterfaceInfo{{
		InterfaceName:  "eth0",
		ConfigType:     network.ConfigStatic,
		NoAutoStart:    false,
		Address:        network.NewAddress("0.1.2.3", network.ScopeUnknown),
		DNSServers:     network.NewAddresses("ns1.invalid", "ns2.invalid"),
		GatewayAddress: network.NewAddress("0.1.2.1", network.ScopeUnknown),
	}}
	netConfig := container.BridgeNetworkConfig("foo", ifaces)
	cloudConf, err := container.NewCloudInitConfigWithNetworks(netConfig)
	c.Assert(err, jc.ErrorIsNil)
	renderer, err := cloudinit.NewRenderer("quantal")
	c.Assert(err, jc.ErrorIsNil)
	data, err := renderer.Render(cloudConf)
	c.Assert(err, jc.ErrorIsNil)
	expected := `
#cloud-config
runcmd:
- install -D -m 644 /dev/null '`[1:] + s.networkInterfacesFile + `'
- |-
  printf '%s\n' '
  # interface "lo"
  auto lo
  iface lo inet dhcp


  # interface "eth0"
  auto eth0
  iface eth0 inet static
      address 0.1.2.3
      netmask 255.255.255.255
      dns-nameservers ns1.invalid ns2.invalid
      pre-up ip route add 0.1.2.1 dev eth0
      pre-up ip route add default via 0.1.2.1
      post-down ip route del default via 0.1.2.1
      post-down ip route del 0.1.2.1 dev eth0
  ' > '` + s.networkInterfacesFile + `'
`
	c.Assert(string(data), gc.Equals, expected)
}
