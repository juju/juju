// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
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
	s.PatchValue(lxd.OsStat, func(path string) (os.FileInfo, error) {
		return nil, nil
	})
	path := lxd.SocketPath()
	c.Check(path, gc.Equals, filepath.Join("foobar", "unix.socket"))
}

func (s *connectionSuite) TestLxdSocketPathSnapSocketAndDebianSocketExists(c *gc.C) {
	os.Setenv("LXD_DIR", "")
	s.PatchValue(lxd.OsStat, func(path string) (os.FileInfo, error) {
		if path == filepath.FromSlash("/var/snap/lxd/common/lxd") ||
			path == filepath.FromSlash("/var/lib/lxd/") {
			return nil, nil
		} else {
			return nil, errors.New("not found")
		}
	})
	path := lxd.SocketPath()
	c.Check(path, gc.Equals, filepath.FromSlash("/var/snap/lxd/common/lxd/unix.socket"))
}

func (s *connectionSuite) TestLxdSocketPathNoSnapSocket(c *gc.C) {
	os.Setenv("LXD_DIR", "")
	s.PatchValue(lxd.OsStat, func(path string) (os.FileInfo, error) {
		if path == filepath.FromSlash("/var/lib/lxd/") {
			return nil, nil
		} else {
			return nil, errors.New("not found")
		}
	})
	path := lxd.SocketPath()
	c.Check(path, gc.Equals, filepath.FromSlash("/var/lib/lxd/unix.socket"))
}

func (s *connectionSuite) TestConnectRemoteBadProtocol(c *gc.C) {
	svr, err := lxd.ConnectImageRemote(lxd.RemoteServer{Host: "wrong-protocol-server", Protocol: "FOOBAR"})
	c.Check(svr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "bad protocol supplied for connection: FOOBAR")
}
