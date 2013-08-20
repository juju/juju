// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	gc "launchpad.net/gocheck"
	"path/filepath"

	"launchpad.net/juju-core/environs/config"
)

type JujuHomeSuite struct {
	jujuHome string
}

var _ = gc.Suite(&JujuHomeSuite{})

func (s *JujuHomeSuite) TestStandardHome(c *gc.C) {
	testJujuHome := c.MkDir()
	defer config.SetJujuHome(config.SetJujuHome(testJujuHome))
	c.Assert(config.JujuHome(), gc.Equals, testJujuHome)
}

func (s *JujuHomeSuite) TestErrorHome(c *gc.C) {
	// Invalid juju home leads to panic when retrieving.
	f := func() { _ = config.JujuHome() }
	c.Assert(f, gc.PanicMatches, "juju home hasn't been initialized")
	f = func() { _ = config.JujuHomePath("environments.yaml") }
	c.Assert(f, gc.PanicMatches, "juju home hasn't been initialized")
}

func (s *JujuHomeSuite) TestHomePath(c *gc.C) {
	testJujuHome := c.MkDir()
	defer config.SetJujuHome(config.SetJujuHome(testJujuHome))
	envPath := config.JujuHomePath("environments.yaml")
	c.Assert(envPath, gc.Equals, filepath.Join(testJujuHome, "environments.yaml"))
}
