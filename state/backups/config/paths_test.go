// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups/config"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&pathsSuite{})

type pathsSuite struct {
	testing.BaseSuite
}

func (s *sourcesSuite) TestPathsNewPathsUnrooted(c *gc.C) {
	paths := config.NewPaths("", "a", "b", "c", "d", "e")

	c.Check(paths.DataDir(), gc.Equals, "a")
	c.Check(paths.StartupDir(), gc.Equals, "b")
	c.Check(paths.LoggingConfDir(), gc.Equals, "c")
	c.Check(paths.LogsDir(), gc.Equals, "d")
	c.Check(paths.SSHDir(), gc.Equals, "e")
}

func (s *sourcesSuite) TestPathsNewPathsRooted(c *gc.C) {
	paths := config.NewPaths("/some_root", "a", "b", "c", "d", "e")

	c.Check(paths.DataDir(), gc.Equals, "/some_root/a")
	c.Check(paths.StartupDir(), gc.Equals, "/some_root/b")
	c.Check(paths.LoggingConfDir(), gc.Equals, "/some_root/c")
	c.Check(paths.LogsDir(), gc.Equals, "/some_root/d")
	c.Check(paths.SSHDir(), gc.Equals, "/some_root/e")
}

func (s *sourcesSuite) TestPathsNewPathsFSRooted(c *gc.C) {
	paths := config.NewPaths("/", "a", "b", "c", "d", "e")

	c.Check(paths.DataDir(), gc.Equals, "/a")
	c.Check(paths.StartupDir(), gc.Equals, "/b")
	c.Check(paths.LoggingConfDir(), gc.Equals, "/c")
	c.Check(paths.LogsDir(), gc.Equals, "/d")
	c.Check(paths.SSHDir(), gc.Equals, "/e")
}

func (s *sourcesSuite) TestPathsNewPathsRelRooted(c *gc.C) {
	paths := config.NewPaths("some_root", "a", "b", "c", "d", "e")

	c.Check(paths.DataDir(), gc.Equals, "some_root/a")
	c.Check(paths.StartupDir(), gc.Equals, "some_root/b")
	c.Check(paths.LoggingConfDir(), gc.Equals, "some_root/c")
	c.Check(paths.LogsDir(), gc.Equals, "some_root/d")
	c.Check(paths.SSHDir(), gc.Equals, "some_root/e")
}

func (s *sourcesSuite) TestPathsNewPathsDefaultsRooted(c *gc.C) {
	paths := config.NewPathsDefaults("/some_root")

	c.Check(paths.DataDir(), gc.Equals, "/some_root/var/lib/juju")
	c.Check(paths.StartupDir(), gc.Equals, "/some_root/etc/init")
	c.Check(paths.LoggingConfDir(), gc.Equals, "/some_root/etc/rsyslog.d")
	c.Check(paths.LogsDir(), gc.Equals, "/some_root/var/log/juju")
	c.Check(paths.SSHDir(), gc.Equals, "/some_root/home/ubuntu/.ssh")
}

func (s *sourcesSuite) TestPathsNewPathsDefaultsUnrooted(c *gc.C) {
	paths := config.NewPathsDefaults("")

	c.Check(paths.DataDir(), gc.Equals, "/var/lib/juju")
	c.Check(paths.StartupDir(), gc.Equals, "/etc/init")
	c.Check(paths.LoggingConfDir(), gc.Equals, "/etc/rsyslog.d")
	c.Check(paths.LogsDir(), gc.Equals, "/var/log/juju")
	c.Check(paths.SSHDir(), gc.Equals, "/home/ubuntu/.ssh")
}

func (s *sourcesSuite) TestPathsDefaultPaths(c *gc.C) {
	c.Check(config.DefaultPaths, gc.DeepEquals, config.NewPathsDefaults(""))
}
