// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"os"
	"path/filepath"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	coretesting "github.com/juju/juju/testing"
)

type connectionSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&connectionSuite{})

func (s *connectionSuite) TestLxdSocketPathLxdDirSet(c *gc.C) {
	os.Setenv("LXD_DIR", "foobar")
	path := lxd.SocketPath(nil)
	c.Check(path, gc.Equals, filepath.Join("foobar", "unix.socket"))
}

func (s *connectionSuite) TestLxdSocketPathSnapSocketAndDebianSocketExists(c *gc.C) {
	os.Setenv("LXD_DIR", "")
	isSocket := func(path string) bool {
		return path == filepath.FromSlash("/var/snap/lxd/common/lxd/unix.socket") ||
			path == filepath.FromSlash("/var/lib/lxd/unix.socket")
	}
	path := lxd.SocketPath(isSocket)
	c.Check(path, gc.Equals, filepath.FromSlash("/var/snap/lxd/common/lxd/unix.socket"))
}

func (s *connectionSuite) TestLxdSocketPathNoSnapSocket(c *gc.C) {
	os.Setenv("LXD_DIR", "")
	isSocket := func(path string) bool {
		return path == filepath.FromSlash("/var/lib/lxd/unix.socket")
	}
	path := lxd.SocketPath(isSocket)
	c.Check(path, gc.Equals, filepath.FromSlash("/var/lib/lxd/unix.socket"))
}

func (s *connectionSuite) TestConnectRemoteBadProtocol(c *gc.C) {
	svr, err := lxd.ConnectImageRemote(lxd.ServerSpec{Host: "wrong-protocol-server", Protocol: "FOOBAR"})
	c.Check(svr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "bad protocol supplied for connection: FOOBAR")
}

func (s *connectionSuite) TestEnsureHTTPSUnchangedWhenCorrect(c *gc.C) {
	addr := "https://somewhere"
	c.Check(lxd.EnsureHTTPS(addr), gc.Equals, addr)
}

func (s *connectionSuite) TestEnsureHTTPSForHTTP(c *gc.C) {
	c.Check(lxd.EnsureHTTPS("http://somewhere"), gc.Equals, "https://somewhere")
}

func (s *connectionSuite) TestEnsureHTTPSForNoProtocol(c *gc.C) {
	c.Check(lxd.EnsureHTTPS("somewhere"), gc.Equals, "https://somewhere")
}
