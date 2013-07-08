// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/jujutest"
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

type localJujuTestSuite struct {
	baseProviderSuite
	jujutest.Tests
	rootCheckFunc func() bool
}

func (s *localJujuTestSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
	// Construct the directories first.
	err := local.CreateDirs(c, minimalConfig(c))
	c.Assert(err, gc.IsNil)
	// Add in an admin secret
	s.Tests.TestConfig.Config["admin-secret"] = "sekrit"
	s.rootCheckFunc = local.SetRootCheckFunction(func() bool { return true })
	s.Tests.SetUpTest(c)
}

func (s *localJujuTestSuite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	local.SetRootCheckFunction(s.rootCheckFunc)
	s.baseProviderSuite.TearDownTest(c)
}

var _ = gc.Suite(&localJujuTestSuite{
	Tests: jujutest.Tests{
		TestConfig: jujutest.TestConfig{minimalConfigValues()},
	},
})

//func (s *localJujuTestSuite) TestBootstrap(c *gc.C) {
//	c.Skip("Bootstrap not implemented yet.")/
//}

func (s *localJujuTestSuite) TestStartStop(c *gc.C) {
	c.Skip("StartInstance not implemented yet.")
}
