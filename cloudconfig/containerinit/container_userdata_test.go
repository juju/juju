// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerinit_test

import (
	"path/filepath"
	"strings"
	gotesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v1"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/containerinit"
	"github.com/juju/juju/container"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/service"
	"github.com/juju/juju/testing"
)

func Test(t *gotesting.T) {
	gc.TestingT(t)
}

type UserDataSuite struct {
	testing.BaseSuite

	networkInterfacesFile string
	fakeInterfaces        []network.InterfaceInfo
	expectedNetConfig     string
}

var _ = gc.Suite(&UserDataSuite{})

func (s *UserDataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.networkInterfacesFile = filepath.Join(c.MkDir(), "interfaces")
	s.fakeInterfaces = []network.InterfaceInfo{{
		InterfaceName:  "eth0",
		CIDR:           "0.1.2.0/24",
		ConfigType:     network.ConfigStatic,
		NoAutoStart:    false,
		Address:        network.NewAddress("0.1.2.3"),
		DNSServers:     network.NewAddresses("ns1.invalid", "ns2.invalid"),
		GatewayAddress: network.NewAddress("0.1.2.1"),
	}, {
		InterfaceName: "eth1",
		ConfigType:    network.ConfigDHCP,
		NoAutoStart:   true,
	}}
	s.expectedNetConfig = `
# loopback interface
auto lo
iface lo inet loopback

# interface "eth0"
auto eth0
iface eth0 inet manual
    dns-nameservers ns1.invalid ns2.invalid
    pre-up ip address add 0.1.2.3/32 dev eth0 &> /dev/null || true
    up ip route replace 0.1.2.1 dev eth0
    up ip route replace default via 0.1.2.1
    down ip route del default via 0.1.2.1 &> /dev/null || true
    down ip route del 0.1.2.1 dev eth0 &> /dev/null || true
    post-down ip address del 0.1.2.3/32 dev eth0 &> /dev/null || true

# interface "eth1"
iface eth1 inet dhcp
`
	s.PatchValue(containerinit.NetworkInterfacesFile, s.networkInterfacesFile)
}

func (s *UserDataSuite) TestGenerateNetworkConfig(c *gc.C) {
	// No config or no interfaces - no error, but also noting to generate.
	data, err := containerinit.GenerateNetworkConfig(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, 0)
	netConfig := container.BridgeNetworkConfig("foo", nil)
	data, err = containerinit.GenerateNetworkConfig(netConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, 0)

	// Test with all interface types.
	netConfig = container.BridgeNetworkConfig("foo", s.fakeInterfaces)
	data, err = containerinit.GenerateNetworkConfig(netConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.Equals, s.expectedNetConfig)
}

func (s *UserDataSuite) TestNewCloudInitConfigWithNetworks(c *gc.C) {
	netConfig := container.BridgeNetworkConfig("foo", s.fakeInterfaces)
	cloudConf, err := containerinit.NewCloudInitConfigWithNetworks("quantal", netConfig)
	c.Assert(err, jc.ErrorIsNil)
	// We need to indent expectNetConfig to make it valid YAML,
	// dropping the last new line and using unindented blank lines.
	lines := strings.Split(s.expectedNetConfig, "\n")
	indentedNetConfig := strings.Join(lines[:len(lines)-1], "\n  ")
	indentedNetConfig = strings.Replace(indentedNetConfig, "\n  \n", "\n\n", -1)
	expected := `
#cloud-config
bootcmd:
- install -D -m 644 /dev/null '`[1:] + s.networkInterfacesFile + `'
- |-
  printf '%s\n' '` + indentedNetConfig + `
  ' > '` + s.networkInterfacesFile + `'
`
	assertUserData(c, cloudConf, expected)
}

func (s *UserDataSuite) TestNewCloudInitConfigWithNetworksNoConfig(c *gc.C) {
	netConfig := container.BridgeNetworkConfig("foo", nil)
	cloudConf, err := containerinit.NewCloudInitConfigWithNetworks("quantal", netConfig)
	c.Assert(err, jc.ErrorIsNil)
	expected := "#cloud-config\n{}\n"
	assertUserData(c, cloudConf, expected)
}

func (s *UserDataSuite) TestCloudInitUserData(c *gc.C) {
	instanceConfig, err := containertesting.MockMachineConfig("1/lxc/0")
	c.Assert(err, jc.ErrorIsNil)
	networkConfig := container.BridgeNetworkConfig("foo", nil)
	data, err := containerinit.CloudInitUserData(instanceConfig, networkConfig)
	c.Assert(err, jc.ErrorIsNil)
	// No need to test the exact contents here, as they are already
	// tested separately.
	c.Assert(string(data), jc.HasPrefix, "#cloud-config\n")
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
		c.Assert(out["bootcmd"], jc.DeepEquals, cloudConf.BootCmds())
	} else {
		c.Assert(out["bootcmd"], gc.IsNil)
	}
}

func (s *UserDataSuite) TestShutdownInitScript(c *gc.C) {
	script, err := containerinit.ShutdownInitScript(service.InitSystemUpstart)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(script, jc.DeepEquals, `
cat >> /etc/init/juju-template-restart.conf << 'EOF'
description "juju shutdown job"
author "Juju Team <juju@lists.ubuntu.com>"
start on stopped cloud-final

script
  /bin/cat > /etc/network/interfaces << EOC
# loopback interface
auto lo
iface lo inet loopback

# primary interface
auto eth0
iface eth0 inet dhcp
EOC
  /bin/rm -fr /var/lib/dhcp/dhclient* /var/log/cloud-init*.log /var/log/upstart/*.log
  /sbin/shutdown -h now
end script

post-stop script
  rm /etc/init/juju-template-restart.conf
end script

EOF
`[1:])
}
