// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	stdtesting "testing"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

func TestMAAS(t *stdtesting.T) {
	TestingT(t)
}

type ProviderSuite struct {
	testing.LoggingSuite
	environ              *maasEnviron
	testMAASObject       *gomaasapi.TestMAASObject
	originalShortAttempt utils.AttemptStrategy
	originalLongAttempt  utils.AttemptStrategy
}

var _ = Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpSuite(c *C) {
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
	s.LoggingSuite.SetUpSuite(c)
	TestMAASObject := gomaasapi.NewTestMAAS("1.0")
	s.testMAASObject = TestMAASObject
	s.environ = &maasEnviron{name: "test env", maasClientUnlocked: &TestMAASObject.MAASObject}
}

func (s *ProviderSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
}

func (s *ProviderSuite) TearDownTest(c *C) {
	s.testMAASObject.TestServer.Clear()
	s.LoggingSuite.TearDownTest(c)
}

func (s *ProviderSuite) TearDownSuite(c *C) {
	s.testMAASObject.Close()

	// Careful: this must be an assignment ("="), not an
	// initialization (":=").  We're trying to change a
	// global variable here.
	shortAttempt = s.originalShortAttempt
	environs.LongAttempt = s.originalLongAttempt
	s.LoggingSuite.TearDownSuite(c)
}
