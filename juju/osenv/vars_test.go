// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"path/filepath"
	"runtime"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type importSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) TestJujuHomeWin(c *gc.C) {
	path := `P:\FooBar\AppData`
	s.PatchEnvironment("APPDATA", path)
	c.Assert(jujuHomeWin(), gc.Equals, filepath.Join(path, "Juju"))
}

func (s *importSuite) TestJujuHomeLinux(c *gc.C) {
	path := `/foo/bar/baz/`
	s.PatchEnvironment("HOME", path)
	c.Assert(jujuHomeLinux(), gc.Equals, filepath.Join(path, ".juju"))
}

func (s *importSuite) TestJujuHomeEnvVar(c *gc.C) {
	path := "/foo/bar/baz"
	s.PatchEnvironment(JujuHomeEnvKey, path)
	c.Assert(JujuHomeDir(), gc.Equals, path)
}

func (s *importSuite) TestBlankJujuHomeEnvVar(c *gc.C) {
	s.PatchEnvironment(JujuHomeEnvKey, "")

	if runtime.GOOS == "windows" {
		s.PatchEnvironment("APPDATA", `P:\foobar`)
	} else {
		s.PatchEnvironment("HOME", "/foobar")
	}
	c.Assert(JujuHomeDir(), gc.Not(gc.Equals), "")

	if runtime.GOOS == "windows" {
		c.Assert(JujuHomeDir(), gc.Equals, jujuHomeWin())
	} else {
		c.Assert(JujuHomeDir(), gc.Equals, jujuHomeLinux())
	}
}
