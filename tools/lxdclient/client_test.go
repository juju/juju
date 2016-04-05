// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/lxc/lxd"
	gc "gopkg.in/check.v1"
)

type ConnectSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ConnectSuite{})

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

var testerr = errors.Errorf("boo!")

func fakeNewClientFromInfo(info lxd.ConnectInfo) (*lxd.Client, error) {
	return nil, testerr
}
