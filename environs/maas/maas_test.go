// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	stdtesting "testing"
)

func TestMAAS(t *stdtesting.T) {
	TestingT(t)
}

type ProviderSuite struct {
	testing.LoggingSuite
	environ             *maasEnviron
	testMAASObject      *gomaasapi.TestMAASObject
	originalLongAttempt utils.AttemptStrategy
}

var _ = Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpSuite(c *C) {
	s.originalLongAttempt = environs.LongAttempt
	environs.LongAttempt = utils.AttemptStrategy{
		Total: 10,
		Delay: 1,
	}
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
	environs.LongAttempt = s.originalLongAttempt
	s.LoggingSuite.TearDownSuite(c)
}
