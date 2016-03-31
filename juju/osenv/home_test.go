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

type JujuXDGDataHomeSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&JujuXDGDataHomeSuite{})

func (s *JujuXDGDataHomeSuite) TestStandardHome(c *gc.C) {
	testJujuXDGDataHome := c.MkDir()
	osenv.SetJujuXDGDataHome(testJujuXDGDataHome)
	c.Assert(osenv.JujuXDGDataHome(), gc.Equals, testJujuXDGDataHome)
}

func (s *JujuXDGDataHomeSuite) TestErrorHome(c *gc.C) {
	// Invalid juju home leads to panic when retrieving.
	f := func() { _ = osenv.JujuXDGDataHome() }
	c.Assert(f, gc.PanicMatches, "juju home hasn't been initialized")
	f = func() { _ = osenv.JujuXDGDataHomePath("current-environment") }
	c.Assert(f, gc.PanicMatches, "juju home hasn't been initialized")
}

func (s *JujuXDGDataHomeSuite) TestHomePath(c *gc.C) {
	testJujuHome := c.MkDir()
	osenv.SetJujuXDGDataHome(testJujuHome)
	envPath := osenv.JujuXDGDataHomePath("current-environment")
	c.Assert(envPath, gc.Equals, filepath.Join(testJujuHome, "current-environment"))
}

func (s *JujuXDGDataHomeSuite) TestIsHomeSet(c *gc.C) {
	c.Assert(osenv.IsJujuXDGDataHomeSet(), jc.IsFalse)
	osenv.SetJujuXDGDataHome(c.MkDir())
	c.Assert(osenv.IsJujuXDGDataHomeSet(), jc.IsTrue)
}
