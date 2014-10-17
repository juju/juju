// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"runtime"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type varsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&varsSuite{})

func (s *varsSuite) TestJujuHomeEnvVar(c *gc.C) {
	path := "/foo/bar/baz"
	s.PatchEnvironment(osenv.JujuHomeEnvKey, path)
	c.Assert(osenv.JujuHomeDir(), gc.Equals, path)
}

func (s *varsSuite) TestBlankJujuHomeEnvVar(c *gc.C) {
	s.PatchEnvironment(osenv.JujuHomeEnvKey, "")

	if runtime.GOOS == "windows" {
		s.PatchEnvironment("APPDATA", `P:\foobar`)
	} else {
		s.PatchEnvironment("HOME", "/foobar")
	}
	c.Assert(osenv.JujuHomeDir(), gc.Not(gc.Equals), "")

	if runtime.GOOS == "windows" {
		c.Assert(osenv.JujuHomeDir(), gc.Equals, osenv.JujuHomeWin())
	} else {
		c.Assert(osenv.JujuHomeDir(), gc.Equals, osenv.JujuHomeLinux())
	}
}
