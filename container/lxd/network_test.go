package lxd_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	coretesting "github.com/juju/juju/testing"
)

type networkSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&networkSuite{})

func (s *networkSuite) TestCheckLXDBridgeConfiguration(c *gc.C) {
	var err error

	valid := func(string) ([]byte, error) {
		return []byte(`
# Whether to setup a new bridge or use an existing one
USE_LXD_BRIDGE="true"

# Bridge name
# This is still used even if USE_LXD_BRIDGE is set to false
# set to an empty value to fully disable
LXD_BRIDGE="lxdbr0"

# Path to an extra dnsmasq configuration file
LXD_CONFILE=""

# DNS domain for the bridge
LXD_DOMAIN="lxd"

# IPv4
## IPv4 address (e.g. 10.0.4.1)
LXD_IPV4_ADDR="10.0.4.1"

## IPv4 netmask (e.g. 255.255.255.0)
LXD_IPV4_NETMASK="255.255.255.0"

## IPv4 network (e.g. 10.0.4.0/24)
LXD_IPV4_NETWORK="10.0.4.1/24"

## IPv4 DHCP range (e.g. 10.0.4.2,10.0.4.254)
LXD_IPV4_DHCP_RANGE="10.0.4.2,10.0.4.254"

## IPv4 DHCP number of hosts (e.g. 250)
LXD_IPV4_DHCP_MAX="253"

## NAT IPv4 traffic
LXD_IPV4_NAT="true"

# IPv6
## IPv6 address (e.g. 2001:470:b368:4242::1)
LXD_IPV6_ADDR=""

## IPv6 CIDR mask (e.g. 64)
LXD_IPV6_MASK=""
LXD_IPV6_NETWORK=""
`), nil
	}

	err = lxd.CheckBridgeConfigFile(valid)
	c.Assert(err, jc.ErrorIsNil)

	noBridge := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="false"
`), nil
	}
	err = lxd.CheckBridgeConfigFile(noBridge)
	c.Assert(err.Error(), gc.Equals, `lxdbr0 not enabled but required
It looks like your lxdbr0 has not yet been configured. Configure it via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)

	badName := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="true"
LXD_BRIDGE="meshuggahrocks"
`), nil
	}
	err = lxd.CheckBridgeConfigFile(badName)
	c.Assert(err.Error(), gc.Equals, lxd.BridgeConfigFile+` has a bridge named meshuggahrocks, not lxdbr0
It looks like your lxdbr0 has not yet been configured. Configure it via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)

	noSubnets := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="true"
LXD_BRIDGE="lxdbr0"
`), nil
	}
	err = lxd.CheckBridgeConfigFile(noSubnets)
	c.Assert(err.Error(), gc.Equals, `lxdbr0 has no ipv4 or ipv6 subnet enabled
It looks like your lxdbr0 has not yet been configured. Configure it via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)

	ipv6 := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="true"
LXD_BRIDGE="lxdbr0"
LXD_IPV6_ADDR="2001:470:b368:4242::1"
`), nil
	}

	err = lxd.CheckBridgeConfigFile(ipv6)
	c.Assert(err.Error(), gc.Equals, lxd.BridgeConfigFile+` has IPv6 enabled.
Juju doesn't currently support IPv6.
Disable IPv6 via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)

}
