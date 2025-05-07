// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"os"
	"path/filepath"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/juju/osenv"
)

func (s *varsSuite) TestJujuXDGDataHome(c *tc.C) {
	path := `/foo/bar/baz/`
	// cleanup xdg config home because it has priority and it might
	// be set on the testing env.
	s.PatchEnvironment(osenv.XDGDataHome, "")
	s.PatchEnvironment("SNAP_REAL_HOME", path)
	c.Assert(osenv.JujuXDGDataHomeLinux(), tc.Equals, filepath.Join(path, ".local", "share", "juju"))
}

func (s *varsSuite) TestJujuXDGDataHomeNoSnapHome(c *tc.C) {
	path := `/foo/bar/baz/`
	// cleanup xdg config home because it has priority and it might
	// be set on the testing env.
	s.PatchEnvironment(osenv.XDGDataHome, "")
	s.PatchEnvironment("SNAP_REAL_HOME", "")
	err := os.Unsetenv("SNAP_REAL_HOME")
	c.Assert(err, jc.ErrorIsNil)
	s.PatchEnvironment("HOME", path)
	c.Assert(osenv.JujuXDGDataHomeLinux(), tc.Equals, filepath.Join(path, ".local", "share", "juju"))
}

func (s *varsSuite) TestJujuXDGDataHomeXDG(c *tc.C) {
	testJujuXDGHome := "/a/bogus/home"
	s.PatchEnvironment(osenv.XDGDataHome, testJujuXDGHome)
	homeLinux := osenv.JujuXDGDataHomeLinux()
	c.Assert(homeLinux, tc.Equals, filepath.Join(testJujuXDGHome, "juju"))
}

func (s *varsSuite) TestJujuXDGDataHomeNoXDGDefaultsConfig(c *tc.C) {
	s.PatchEnvironment(osenv.XDGDataHome, "")
	s.PatchEnvironment("SNAP_REAL_HOME", "/a/bogus/user/home")
	homeLinux := osenv.JujuXDGDataHomeLinux()
	c.Assert(homeLinux, tc.Equals, "/a/bogus/user/home/.local/share/juju")
}
