// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/local"
)

type environSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&environSuite{})

func (*environSuite) TestOpenFailsWithProtectedDirectories(c *gc.C) {
	testConfig := minimalConfig(c)
	testConfig, err := testConfig.Apply(map[string]interface{}{
		"root-dir": "/usr/lib/juju",
	})
	c.Assert(err, gc.IsNil)

	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.ErrorMatches, "mkdir .* permission denied")
	c.Assert(environ, gc.IsNil)
}

func (s *environSuite) TestNameAndStorage(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.IsNil)
	c.Assert(environ.Name(), gc.Equals, "test")
	c.Assert(environ.Storage(), gc.NotNil)
	c.Assert(environ.PublicStorage(), gc.NotNil)
}

type localJujuTestSuite struct {
	baseProviderSuite
	jujutest.Tests
}

func (s *localJujuTestSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
	s.Tests.SetUpTest(c)
}

func (s *localJujuTestSuite) TearDownTest(c *gc.C) {
	// TODO(thumper): add the TearDownTest for s.Tests when destroy is implemented
	// s.Tests.TearDownTest(c)
	s.baseProviderSuite.TearDownTest(c)
}

var _ = gc.Suite(&localJujuTestSuite{
	Tests: jujutest.Tests{
		TestConfig: jujutest.TestConfig{minimalConfigValues()},
	},
})

func (s *localJujuTestSuite) TestBootstrap(c *gc.C) {
	c.Skip("Bootstrap not implemented yet.")
}

func (s *localJujuTestSuite) TestStartStop(c *gc.C) {
	c.Skip("StartInstance not implemented yet.")
}
