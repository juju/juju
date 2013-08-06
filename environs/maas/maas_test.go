// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"

	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/testing"
)

func TestMAAS(t *stdtesting.T) {
	gc.TestingT(t)
}

type ProviderSuite struct {
	testing.LoggingSuite
	environ         *maasEnviron
	testMAASObject  *gomaasapi.TestMAASObject
	restoreTimeouts func()
}

var _ = gc.Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpSuite(c *gc.C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(&shortAttempt)
	s.LoggingSuite.SetUpSuite(c)
	TestMAASObject := gomaasapi.NewTestMAAS("1.0")
	s.testMAASObject = TestMAASObject
	s.environ = &maasEnviron{name: "test env", maasClientUnlocked: &TestMAASObject.MAASObject}
}

func (s *ProviderSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
}

func (s *ProviderSuite) TearDownTest(c *gc.C) {
	s.testMAASObject.TestServer.Clear()
	s.LoggingSuite.TearDownTest(c)
}

func (s *ProviderSuite) TearDownSuite(c *gc.C) {
	s.testMAASObject.Close()
	s.restoreTimeouts()
	s.LoggingSuite.TearDownSuite(c)
}
