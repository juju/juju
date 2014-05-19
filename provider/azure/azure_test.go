// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/testing"
)

func TestAzureProvider(t *stdtesting.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	testing.BaseSuite
	envtesting.ToolsFixture
	restoreTimeouts func()
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.restoreTimeouts = envtesting.PatchAttemptStrategies()
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	s.restoreTimeouts()
	s.BaseSuite.TearDownSuite(c)
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}
