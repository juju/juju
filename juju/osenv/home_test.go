// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
)

type JujuHomeSuite struct {
	jujuHome string
}

var _ = gc.Suite(&JujuHomeSuite{})

func (s *JujuHomeSuite) TestStandardHome(c *gc.C) {
	testJujuHome := c.MkDir()
	defer osenv.SetJujuHome(osenv.SetJujuHome(testJujuHome))
	c.Assert(osenv.JujuHome(), gc.Equals, testJujuHome)
}

func (s *JujuHomeSuite) TestErrorHome(c *gc.C) {
	// Invalid juju home leads to panic when retrieving.
	f := func() { _ = osenv.JujuHome() }
	c.Assert(f, gc.PanicMatches, "juju home hasn't been initialized")
	f = func() { _ = osenv.JujuHomePath("environments.yaml") }
	c.Assert(f, gc.PanicMatches, "juju home hasn't been initialized")
}

func (s *JujuHomeSuite) TestHomePath(c *gc.C) {
	testJujuHome := c.MkDir()
	defer osenv.SetJujuHome(osenv.SetJujuHome(testJujuHome))
	envPath := osenv.JujuHomePath("environments.yaml")
	c.Assert(envPath, gc.Equals, filepath.Join(testJujuHome, "environments.yaml"))
}
