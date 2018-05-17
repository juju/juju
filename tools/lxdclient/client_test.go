// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package lxdclient

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	proxyutils "github.com/juju/proxy"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujuos "github.com/juju/utils/os"
	lxdclient "github.com/lxc/lxd/client"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/utils/proxy"
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
			Host: f.Name(),
		},
	}.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	/* ECONNREFUSED because it's not a socket (mimics behavior of a socket
	 * with nobody listening)
	 */
	_, err = Connect(cfg, false)
	c.Assert(err.Error(), gc.Equals, `can't connect to the local LXD server: LXD refused connections; is LXD running?

Please configure LXD by running:
	$ newgrp lxd
	$ lxd init
`)

	/* EACCESS because we can't read/write */
	c.Assert(f.Chmod(0400), jc.ErrorIsNil)
	_, err = Connect(cfg, false)
	c.Assert(err.Error(), gc.Equals, `can't connect to the local LXD server: Permisson denied, are you in the lxd group?

Please configure LXD by running:
	$ newgrp lxd
	$ lxd init
`)

	/* ENOENT because it doesn't exist */
	c.Assert(os.RemoveAll(f.Name()), jc.ErrorIsNil)
	_, err = Connect(cfg, false)
	c.Assert(err.Error(), gc.Equals, `can't connect to the local LXD server: LXD socket not found; is LXD installed & running?

Please install LXD by running:
	$ sudo snap install lxd
and then configure it with:
	$ newgrp lxd
	$ lxd init
`)

	// Yes, the error message actually matters here... this is being displayed
	// to the user.
	cs.PatchValue(&newSocketClientFromInfo, fakeNewClientFromInfo)
	_, err = Connect(cfg, false)
	c.Assert(err.Error(), gc.Equals, `can't connect to the local LXD server: boo!

Please install LXD by running:
	$ sudo snap install lxd
and then configure it with:
	$ newgrp lxd
	$ lxd init
`)
}

func (cs *ConnectSuite) TestRemoteConnectError(c *gc.C) {
	cs.PatchValue(&newHostClientFromInfo, fakeNewClientFromInfo)

	cfg, err := Config{
		Remote: Remote{
			Name: "foo",
			Host: "a.b.c",
			Cert: &lxd.Certificate{
				Name:    "really-valid",
				CertPEM: []byte("kinda-public"),
				KeyPEM:  []byte("super-secret"),
			},
		},
	}.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)
	_, err = Connect(cfg, false)

	c.Assert(errors.Cause(err), gc.Equals, testerr)
}

func (*ConnectSuite) CheckLogContains(c *gc.C, suffix string) {
	c.Check(c.GetTestLog(), gc.Matches, "(?s).*WARNING juju.tools.lxdclient "+suffix+".*")
}

func (*ConnectSuite) CheckVersionSupported(c *gc.C, version string, supported bool) {
	c.Check(isSupportedAPIVersion(version), gc.Equals, supported)
}

func (cs *ConnectSuite) TestBadVersionChecks(c *gc.C) {
	cs.CheckVersionSupported(c, "foo", false)
	cs.CheckLogContains(c, `LXD API version "foo": expected format <major>\.<minor>`)

	cs.CheckVersionSupported(c, "a.b", false)
	cs.CheckLogContains(c, `LXD API version "a.b": unexpected major number: strconv.(ParseInt|Atoi): parsing "a": invalid syntax`)

	cs.CheckVersionSupported(c, "0.9", false)
	cs.CheckLogContains(c, `LXD API version "0.9": expected major version 1 or later`)
}

func (cs *ConnectSuite) TestGoodVersionChecks(c *gc.C) {
	cs.CheckVersionSupported(c, "1.0", true)
	cs.CheckVersionSupported(c, "2.0", true)
	cs.CheckVersionSupported(c, "2.1", true)
}

func (cs *ConnectSuite) TestProxySettings(c *gc.C) {
	cs.AddCleanup(func(c *gc.C) {
		err := proxy.DefaultConfig.Set(proxyutils.Settings{})
		c.Assert(err, jc.ErrorIsNil)
	})

	cfg, err := Config{Remote: Remote{
		Name: "foo",
		Host: "https://host.invalid",
	}}.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	// The LXD client should use the Juju-managed proxy configuration.

	// No proxy by default.
	_, err = Connect(cfg, false)
	c.Assert(err, gc.ErrorMatches, `.*host\.invalid.*`)

	// Configure a proxy, expect it to be used.
	err = proxy.DefaultConfig.Set(proxyutils.Settings{Https: "https://proxy.invalid"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = Connect(cfg, false)
	c.Assert(err, gc.ErrorMatches, `.*proxy\.invalid.*`)
}

var testerr = errors.Errorf("boo!")

func fakeNewClientFromInfo(_ string, _ *lxdclient.ConnectionArgs) (lxdclient.ContainerServer, error) {
	return nil, testerr
}
