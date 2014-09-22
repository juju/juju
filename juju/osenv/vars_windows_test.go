// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"path/filepath"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type varsWindowsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&varsWindowsSuite{})


func (s *varsWindowsSuite) TestJujuHome(c *gc.C) {
	path := `P:\FooBar\AppData`
	s.PatchEnvironment("APPDATA", path)

	c.Assert(osenv.JujuHomeWin(), gc.Equals, filepath.Join(path, "Juju"))
}

func (s *varsWindowsSuite) TestJujuHomeEnvVar(c *gc.C) {
	path := `P:\foo\bar`
	s.PatchEnvironment(osenv.JujuHomeEnvKey, path)

	c.Assert(osenv.JujuHomeDir(), gc.Equals, path)
}

func (s *varsWindowsSuite) TestBlankJujuHomeEnvVar(c *gc.C) {
	s.PatchEnvironment(osenv.JujuHomeEnvKey, "")
	s.PatchEnvironment("APPDATA", `P:\FooBar\AppData`)

	c.Assert(osenv.JujuHomeDir(), gc.Equals, osenv.JujuHomeWin())
}
