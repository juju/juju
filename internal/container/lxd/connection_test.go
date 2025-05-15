// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"os"
	"path/filepath"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/container/lxd"
	coretesting "github.com/juju/juju/internal/testing"
)

type connectionSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&connectionSuite{})

func (s *connectionSuite) TestLxdSocketPathLxdDirSet(c *tc.C) {
	c.Assert(os.Setenv("LXD_DIR", "foobar"), tc.ErrorIsNil)
	isSocket := func(path string) bool {
		return path == filepath.FromSlash("foobar/unix.socket")
	}
	c.Check(lxd.SocketPath(isSocket), tc.Equals, filepath.Join("foobar", "unix.socket"))
}

func (s *connectionSuite) TestLxdSocketPathSnapSocketAndDebianSocketExists(c *tc.C) {
	c.Assert(os.Setenv("LXD_DIR", ""), tc.ErrorIsNil)
	isSocket := func(path string) bool {
		return path == filepath.FromSlash("/var/snap/lxd/common/lxd/unix.socket") ||
			path == filepath.FromSlash("/var/lib/lxd/unix.socket")
	}
	c.Check(lxd.SocketPath(isSocket), tc.Equals, filepath.FromSlash("/var/snap/lxd/common/lxd/unix.socket"))
}

func (s *connectionSuite) TestLxdSocketPathNoSnapSocket(c *tc.C) {
	c.Assert(os.Setenv("LXD_DIR", ""), tc.ErrorIsNil)
	isSocket := func(path string) bool {
		return path == filepath.FromSlash("/var/lib/lxd/unix.socket")
	}
	c.Check(lxd.SocketPath(isSocket), tc.Equals, filepath.FromSlash("/var/lib/lxd/unix.socket"))
}

func (s *connectionSuite) TestLxdSocketPathNoSocket(c *tc.C) {
	c.Assert(os.Setenv("LXD_DIR", ""), tc.ErrorIsNil)
	isSocket := func(path string) bool { return false }
	c.Check(lxd.SocketPath(isSocket), tc.Equals, "")
}

func (s *connectionSuite) TestConnectRemoteBadProtocol(c *tc.C) {
	svr, err := lxd.ConnectImageRemote(c.Context(), lxd.ServerSpec{Host: "wrong-protocol-server", Protocol: "FOOBAR"})
	c.Check(svr, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "bad protocol supplied for connection: FOOBAR")
}

func (s *connectionSuite) TestEnsureHTTPSUnchangedWhenCorrect(c *tc.C) {
	addr := "https://somewhere"
	c.Check(lxd.EnsureHTTPS(addr), tc.Equals, addr)
}

func (s *connectionSuite) TestEnsureHTTPS(c *tc.C) {
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
		c.Assert(got, tc.Equals, t.Output)
	}
}

func (s *connectionSuite) TestEnsureHostPort(c *tc.C) {
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
		c.Assert(err, tc.IsNil)
		c.Assert(got, tc.Equals, t.Output)
	}
}
