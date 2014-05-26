// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"path/filepath"
	"runtime"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing"
)

type varsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&varsSuite{})

func (s *varsSuite) TestJujuHomeWin(c *gc.C) {
	path := `P:\FooBar\AppData`
	s.PatchEnvironment("APPDATA", path)
	c.Assert(osenv.JujuHomeWin(), gc.Equals, filepath.Join(path, "Juju"))
}

func (s *varsSuite) TestJujuHomeLinux(c *gc.C) {
	path := `/foo/bar/baz/`
	s.PatchEnvironment("HOME", path)
	c.Assert(osenv.JujuHomeLinux(), gc.Equals, filepath.Join(path, ".juju"))
}

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
