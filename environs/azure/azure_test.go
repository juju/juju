// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdtesting "testing"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

func TestAzureProvider(t *stdtesting.T) {
	TestingT(t)
}

type ProviderSuite struct {
	testing.LoggingSuite
	environ              *azureEnviron
	originalShortAttempt utils.AttemptStrategy
	originalLongAttempt  utils.AttemptStrategy
}

var _ = Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.environ = &azureEnviron{}

	s.originalShortAttempt = shortAttempt
	s.originalLongAttempt = environs.LongAttempt

	// Careful: this must be an assignment ("="), not an
	// initialization (":=").  We're trying to change a
	// global variable here.
	shortAttempt = utils.AttemptStrategy{
		Total: 100 * time.Millisecond,
		Delay: 10 * time.Millisecond,
	}
	environs.LongAttempt = shortAttempt
}

func (s *ProviderSuite) TearDownSuite(c *C) {
	// Careful: this must be an assignment ("="), not an
	// initialization (":=").  We're trying to change a
	// global variable here.
	shortAttempt = s.originalShortAttempt
	environs.LongAttempt = s.originalLongAttempt
	s.LoggingSuite.TearDownSuite(c)
}
