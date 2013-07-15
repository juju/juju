// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	stdtesting "testing"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	envtesting "launchpad.net/juju-core/environs/testing"
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
	restoreTimeouts func()
}

var _ = Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpSuite(c *C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(&shortAttempt)
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
	s.restoreTimeouts()
	s.LoggingSuite.TearDownSuite(c)
}
