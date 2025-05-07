// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
)

type JujuXDGDataHomeSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&JujuXDGDataHomeSuite{})

func (s *JujuXDGDataHomeSuite) TearDownTest(c *tc.C) {
	osenv.SetJujuXDGDataHome("")
	s.BaseSuite.TearDownTest(c)
}

func (s *JujuXDGDataHomeSuite) TestStandardHome(c *tc.C) {
	testJujuXDGDataHome := c.MkDir()
	osenv.SetJujuXDGDataHome(testJujuXDGDataHome)
	c.Assert(osenv.JujuXDGDataHome(), tc.Equals, testJujuXDGDataHome)
}

func (s *JujuXDGDataHomeSuite) TestHomePath(c *tc.C) {
	testJujuHome := c.MkDir()
	osenv.SetJujuXDGDataHome(testJujuHome)
	envPath := osenv.JujuXDGDataHomeDir()
	c.Assert(envPath, tc.Equals, testJujuHome)
}
