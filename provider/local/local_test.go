// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"strings"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/provider/local"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

func TestLocal(t *stdtesting.T) {
	gc.TestingT(t)
}

type localSuite struct {
	testbase.LoggingSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&localSuite{})

func (s *localSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (*localSuite) TestProviderRegistered(c *gc.C) {
	provider, error := environs.Provider(provider.Local)
	c.Assert(error, gc.IsNil)
	c.Assert(provider, gc.DeepEquals, local.Provider)
}

func (s *localSuite) TestGetToolsMetadataSources(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.IsNil)

	sources, err := tools.GetMetadataSources(environ)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 1)
	url, err := sources[0].URL("")
	c.Assert(err, gc.IsNil)
	c.Assert(strings.Contains(url, "/tools"), jc.IsTrue)
}
