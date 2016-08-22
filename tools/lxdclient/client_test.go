// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3,linux

package lxdclient

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujuos "github.com/juju/utils/os"
	"github.com/lxc/lxd"
	gc "gopkg.in/check.v1"
)

type ConnectSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ConnectSuite{})

func (cs *ConnectSuite) SetUpSuite(c *gc.C) {
	cs.IsolationSuite.SetUpSuite(c)
	if jujuos.HostOS() != jujuos.Ubuntu {
		c.Skip("lxd is only supported on Ubuntu at the moment")
	}
}

func (cs *ConnectSuite) TestLocalConnectError(c *gc.C) {
	f, err := ioutil.TempFile("", "juju-lxd-remote-test")
	c.Assert(err, jc.ErrorIsNil)
	defer os.RemoveAll(f.Name())

	cfg, err := Config{
		Remote: Remote{
			Name: "local",
			Host: "unix://" + f.Name(),
		},
	}.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	/* ECONNREFUSED because it's not a socket (mimics behavior of a socket
	 * with nobody listening)
	 */
	_, err = Connect(cfg)
	c.Assert(err.Error(), gc.Equals, `can't connect to the local LXD server: LXD refused connections; is LXD running?

Please configure LXD by running:
	$ newgrp lxd
	$ lxd init
`)

	/* EACCESS because we can't read/write */
	c.Assert(f.Chmod(0400), jc.ErrorIsNil)
	_, err = Connect(cfg)
	c.Assert(err.Error(), gc.Equals, `can't connect to the local LXD server: Permisson denied, are you in the lxd group?

Please configure LXD by running:
	$ newgrp lxd
	$ lxd init
`)

	/* ENOENT because it doesn't exist */
	c.Assert(os.RemoveAll(f.Name()), jc.ErrorIsNil)
	_, err = Connect(cfg)
	c.Assert(err.Error(), gc.Equals, `can't connect to the local LXD server: LXD socket not found; is LXD installed & running?

Please install LXD by running:
	$ sudo apt-get install lxd
and then configure it with:
	$ newgrp lxd
	$ lxd init
`)

	// Yes, the error message actually matters here... this is being displayed
	// to the user.
	cs.PatchValue(&lxdNewClientFromInfo, fakeNewClientFromInfo)
	_, err = Connect(cfg)
	c.Assert(err.Error(), gc.Equals, `can't connect to the local LXD server: boo!

Please install LXD by running:
	$ sudo apt-get install lxd
and then configure it with:
	$ newgrp lxd
	$ lxd init
`)
}

func (cs *ConnectSuite) TestCheckLXDBridgeConfiguration(c *gc.C) {
	var err error

	valid := `
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
`

	err = checkLXDBridgeConfiguration(valid)
	c.Assert(err, jc.ErrorIsNil)

	noBridge := `
USE_LXD_BRIDGE="false"
`
	err = checkLXDBridgeConfiguration(noBridge)
	c.Assert(err.Error(), gc.Equals, `lxdbr0 not enabled but required
It looks like your lxdbr0 has not yet been configured. Please configure it via:

	sudo dpkg-reconfigure -p medium lxd

and then bootstrap again.`)

	badName := `
USE_LXD_BRIDGE="true"
LXD_BRIDGE="meshuggahrocks"
`
	err = checkLXDBridgeConfiguration(badName)
	c.Assert(err.Error(), gc.Equals, LXDBridgeFile+` has a bridge named meshuggahrocks, not lxdbr0
It looks like your lxdbr0 has not yet been configured. Please configure it via:

	sudo dpkg-reconfigure -p medium lxd

and then bootstrap again.`)

	noSubnets := `
USE_LXD_BRIDGE="true"
LXD_BRIDGE="lxdbr0"
`
	err = checkLXDBridgeConfiguration(noSubnets)
	c.Assert(err.Error(), gc.Equals, `lxdbr0 has no ipv4 or ipv6 subnet enabled
It looks like your lxdbr0 has not yet been configured. Please configure it via:

	sudo dpkg-reconfigure -p medium lxd

and then bootstrap again.`)

}

func (cs *ConnectSuite) TestRemoteConnectError(c *gc.C) {
	cs.PatchValue(&lxdNewClientFromInfo, fakeNewClientFromInfo)

	cfg, err := Config{
		Remote: Remote{
			Name: "foo",
			Host: "a.b.c",
			Cert: &Cert{
				Name:    "really-valid",
				CertPEM: []byte("kinda-public"),
				KeyPEM:  []byte("super-secret"),
			},
		},
	}.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)
	_, err = Connect(cfg)

	c.Assert(errors.Cause(err), gc.Equals, testerr)
}

func (*ConnectSuite) CheckLogContains(c *gc.C, suffix string) {
	c.Check(c.GetTestLog(), jc.Contains, "WARNING juju.tools.lxdclient "+suffix)
}

func (*ConnectSuite) CheckVersionSupported(c *gc.C, version string, supported bool) {
	c.Check(isSupportedAPIVersion(version), gc.Equals, supported)
}

func (cs *ConnectSuite) TestBadVersionChecks(c *gc.C) {
	cs.CheckVersionSupported(c, "foo", false)
	cs.CheckLogContains(c, `LXD API version "foo": expected format <major>.<minor>`)

	cs.CheckVersionSupported(c, "a.b", false)
	cs.CheckLogContains(c, `LXD API version "a.b": unexpected major number: strconv.ParseInt: parsing "a": invalid syntax`)

	cs.CheckVersionSupported(c, "0.9", false)
	cs.CheckLogContains(c, `LXD API version "0.9": expected major version 1 or later`)
}

func (cs *ConnectSuite) TestGoodVersionChecks(c *gc.C) {
	cs.CheckVersionSupported(c, "1.0", true)
	cs.CheckVersionSupported(c, "2.0", true)
	cs.CheckVersionSupported(c, "2.1", true)
}

var testerr = errors.Errorf("boo!")

func fakeNewClientFromInfo(info lxd.ConnectInfo) (*lxd.Client, error) {
	return nil, testerr
}
