// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/local"
	jc "launchpad.net/juju-core/testing/checkers"
)

type environSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&environSuite{})

func (*environSuite) TestOpenFailsWithoutDirs(c *gc.C) {
	testConfig := minimalConfig(c)

	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.ErrorMatches, "storage directory .* does not exist, bootstrap first")
	c.Assert(environ, gc.IsNil)
}

func (s *environSuite) TestNameAndStorage(c *gc.C) {
	c.Logf("root: %s", s.root)
	c.Assert(s.root, jc.IsDirectory)

	testConfig := minimalConfig(c)
	err := local.CreateDirs(c, testConfig)
	c.Assert(err, gc.IsNil)

	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.IsNil)
	c.Assert(environ.Name(), gc.Equals, "test")
	c.Assert(environ.Storage(), gc.NotNil)
	c.Assert(environ.PublicStorage(), gc.NotNil)
}
