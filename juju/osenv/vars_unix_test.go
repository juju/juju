// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.
// +build !windows

package osenv_test

import (
	"path/filepath"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type varsUnixSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&varsUnixSuite{})


func (s *varsUnixSuite) TestJujuHome(c *gc.C) {
	path := `/foo/bar/baz/`
	s.PatchEnvironment("HOME", path)

	c.Assert(osenv.JujuHomeLinux(), gc.Equals, filepath.Join(path, ".juju"))
}

func (s *varsUnixSuite) TestJujuHomeEnvVar(c *gc.C) {
	path := "/foo/bar/baz"
	s.PatchEnvironment(osenv.JujuHomeEnvKey, path)

	c.Assert(osenv.JujuHomeDir(), gc.Equals, path)
}

func (s *varsUnixSuite) TestBlankJujuHomeEnvVar(c *gc.C) {
	s.PatchEnvironment(osenv.JujuHomeEnvKey, "")
	s.PatchEnvironment("HOME", "/home/foobar")

	c.Assert(osenv.JujuHomeDir(), gc.Equals, osenv.JujuHomeLinux())
}
