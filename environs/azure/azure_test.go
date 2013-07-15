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

type ProviderSuite struct {
	testing.LoggingSuite
	environ         *azureEnviron
	restoreTimeouts func()
}

var _ = Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.environ = &azureEnviron{}
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(&shortAttempt)
}

func (s *ProviderSuite) TearDownSuite(c *C) {
	s.restoreTimeouts()
	s.LoggingSuite.TearDownSuite(c)
}
