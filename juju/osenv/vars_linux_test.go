// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
)

func (s *varsSuite) TestJujuXDGDataHome(c *gc.C) {
	path := `/foo/bar/baz/`
	// cleanup xdg config home because it has priority and it might
	// be set on the testing env.
	s.PatchEnvironment(osenv.XDGDataHome, "")
	s.PatchEnvironment("SNAP_REAL_HOME", path)
	c.Assert(osenv.JujuXDGDataHomeLinux(), gc.Equals, filepath.Join(path, ".local", "share", "juju"))
}

func (s *varsSuite) TestJujuXDGDataHomeNoSnapHome(c *gc.C) {
	path := `/foo/bar/baz/`
	// cleanup xdg config home because it has priority and it might
	// be set on the testing env.
	s.PatchEnvironment(osenv.XDGDataHome, "")
	s.PatchEnvironment("SNAP_REAL_HOME", "")
	err := os.Unsetenv("SNAP_REAL_HOME")
	c.Assert(err, jc.ErrorIsNil)
	s.PatchEnvironment("HOME", path)
	c.Assert(osenv.JujuXDGDataHomeLinux(), gc.Equals, filepath.Join(path, ".local", "share", "juju"))
}

func (s *varsSuite) TestJujuXDGDataHomeXDG(c *gc.C) {
	testJujuXDGHome := "/a/bogus/home"
	s.PatchEnvironment(osenv.XDGDataHome, testJujuXDGHome)
	homeLinux := osenv.JujuXDGDataHomeLinux()
	c.Assert(homeLinux, gc.Equals, filepath.Join(testJujuXDGHome, "juju"))
}

func (s *varsSuite) TestJujuXDGDataHomeNoXDGDefaultsConfig(c *gc.C) {
	s.PatchEnvironment(osenv.XDGDataHome, "")
	s.PatchEnvironment("SNAP_REAL_HOME", "/a/bogus/user/home")
	homeLinux := osenv.JujuXDGDataHomeLinux()
	c.Assert(homeLinux, gc.Equals, "/a/bogus/user/home/.local/share/juju")
}
