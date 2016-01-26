// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type JujuHomeSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&JujuHomeSuite{})

func (s *JujuHomeSuite) TestStandardHome(c *gc.C) {
	testJujuHome := c.MkDir()
	osenv.SetJujuHome(testJujuHome)
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
	osenv.SetJujuHome(testJujuHome)
	envPath := osenv.JujuHomePath("environments.yaml")
	c.Assert(envPath, gc.Equals, filepath.Join(testJujuHome, "environments.yaml"))
}

func (s *JujuHomeSuite) TestIsHomeSet(c *gc.C) {
	c.Assert(osenv.IsJujuHomeSet(), jc.IsFalse)
	osenv.SetJujuHome(c.MkDir())
	c.Assert(osenv.IsJujuHomeSet(), jc.IsTrue)
}
