// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"context"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	coretesting "github.com/juju/juju/testing"
)

type connectionSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&connectionSuite{})

func (s *connectionSuite) TestLxdSocketPathLxdDirSet(c *gc.C) {
	c.Assert(os.Setenv("LXD_DIR", "foobar"), jc.ErrorIsNil)
	isSocket := func(path string) bool {
		return path == filepath.FromSlash("foobar/unix.socket")
	}
	c.Check(lxd.SocketPath(isSocket), gc.Equals, filepath.Join("foobar", "unix.socket"))
}

func (s *connectionSuite) TestLxdSocketPathSnapSocketAndDebianSocketExists(c *gc.C) {
	c.Assert(os.Setenv("LXD_DIR", ""), jc.ErrorIsNil)
	isSocket := func(path string) bool {
		return path == filepath.FromSlash("/var/snap/lxd/common/lxd/unix.socket") ||
			path == filepath.FromSlash("/var/lib/lxd/unix.socket")
	}
	c.Check(lxd.SocketPath(isSocket), gc.Equals, filepath.FromSlash("/var/snap/lxd/common/lxd/unix.socket"))
}

func (s *connectionSuite) TestLxdSocketPathNoSnapSocket(c *gc.C) {
	c.Assert(os.Setenv("LXD_DIR", ""), jc.ErrorIsNil)
	isSocket := func(path string) bool {
		return path == filepath.FromSlash("/var/lib/lxd/unix.socket")
	}
	c.Check(lxd.SocketPath(isSocket), gc.Equals, filepath.FromSlash("/var/lib/lxd/unix.socket"))
}

func (s *connectionSuite) TestLxdSocketPathNoSocket(c *gc.C) {
	c.Assert(os.Setenv("LXD_DIR", ""), jc.ErrorIsNil)
	isSocket := func(path string) bool { return false }
	c.Check(lxd.SocketPath(isSocket), gc.Equals, "")
}

func (s *connectionSuite) TestConnectRemoteBadProtocol(c *gc.C) {
	svr, err := lxd.ConnectImageRemote(context.Background(), lxd.ServerSpec{Host: "wrong-protocol-server", Protocol: "FOOBAR"})
	c.Check(svr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "bad protocol supplied for connection: FOOBAR")
}

func (s *connectionSuite) TestEnsureHTTPSUnchangedWhenCorrect(c *gc.C) {
	addr := "https://somewhere"
	c.Check(lxd.EnsureHTTPS(addr), gc.Equals, addr)
}

func (s *connectionSuite) TestEnsureHTTPS(c *gc.C) {
	for _, t := range []struct {
		Input  string
		Output string
	}{
		{
			Input:  "http://somewhere",
			Output: "https://somewhere",
		},
		{
			Input:  "https://somewhere",
			Output: "https://somewhere",
		},
		{
			Input:  "somewhere",
			Output: "https://somewhere",
		},
	} {
		got := lxd.EnsureHTTPS(t.Input)
		c.Assert(got, gc.Equals, t.Output)
	}
}

func (s *connectionSuite) TestEnsureHostPort(c *gc.C) {
	for _, t := range []struct {
		Input  string
		Output string
	}{
		{
			Input:  "https://somewhere",
			Output: "https://somewhere:8443",
		},
		{
			Input:  "somewhere",
			Output: "https://somewhere:8443",
		},
		{
			Input:  "http://somewhere:0",
			Output: "https://somewhere:0",
		},
		{
			Input:  "https://somewhere:123",
			Output: "https://somewhere:123",
		},
		{
			Input:  "https://somewhere:123/",
			Output: "https://somewhere:123",
		},
	} {
		got, err := lxd.EnsureHostPort(t.Input)
		c.Assert(err, gc.IsNil)
		c.Assert(got, gc.Equals, t.Output)
	}
}
