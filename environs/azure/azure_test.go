// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdtesting "testing"

	. "launchpad.net/gocheck"

	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/testing"
)

func TestAzureProvider(t *stdtesting.T) {
	TestingT(t)
}

type providerSuite struct {
	testing.LoggingSuite
	restoreTimeouts func()
}

var _ = Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.restoreTimeouts = envtesting.PatchAttemptStrategies()
}

func (s *providerSuite) TearDownSuite(c *C) {
	s.restoreTimeouts()
	s.LoggingSuite.TearDownSuite(c)
}
