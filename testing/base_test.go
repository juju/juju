// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"os"

	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type TestingBaseSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&TestingBaseSuite{})

func (s *TestingBaseSuite) SetUpTest(c *gc.C) {
	utils.SetHome("/home/eric")
	os.Setenv("JUJU_HOME", "/home/eric/juju")
	osenv.SetJujuHome("/home/eric/juju")

	s.BaseSuite.SetUpTest(c)
}

func (s *TestingBaseSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)

	// Test that the environment is restored.
	c.Assert(utils.Home(), gc.Equals, "/home/eric")
	c.Assert(os.Getenv("JUJU_HOME"), gc.Equals, "/home/eric/juju")
}

func (s *TestingBaseSuite) TestFakeHomeReplacesEnvironment(c *gc.C) {
	c.Assert(utils.Home(), gc.Not(gc.Equals), "/home/eric")
	c.Assert(os.Getenv("JUJU_HOME"), gc.Equals, "")
}
