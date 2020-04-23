// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type JujuXDGDataHomeSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&JujuXDGDataHomeSuite{})

func (s *JujuXDGDataHomeSuite) TearDownTest(c *gc.C) {
	osenv.SetJujuXDGDataHome("")
	s.BaseSuite.TearDownTest(c)
}

func (s *JujuXDGDataHomeSuite) TestStandardHome(c *gc.C) {
	testJujuXDGDataHome := c.MkDir()
	osenv.SetJujuXDGDataHome(testJujuXDGDataHome)
	c.Assert(osenv.JujuXDGDataHome(), gc.Equals, testJujuXDGDataHome)
}

func (s *JujuXDGDataHomeSuite) TestHomePath(c *gc.C) {
	testJujuHome := c.MkDir()
	osenv.SetJujuXDGDataHome(testJujuHome)
	envPath := osenv.JujuXDGDataHomeDir()
	c.Assert(envPath, gc.Equals, testJujuHome)
}
