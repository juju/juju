// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/testing/testbase"
)

func TestJoyentProvider(t *stdtesting.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	testbase.LoggingSuite
	envtesting.ToolsFixture
	restoreTimeouts func()
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies()
	s.LoggingSuite.SetUpSuite(c)
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	s.restoreTimeouts()
	s.LoggingSuite.TearDownSuite(c)
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

/*const exampleAgentName = "dfb69555-0bc4-4d1f-85f2-4ee390974984"

// makeEnviron creates a functional maasEnviron for a test.
func (suite *JoyentSuite) makeEnviron() *maasEnviron {
	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
	"name":            "test env",
	"type":            "maas",
	"maas-oauth":      "a:b:c",
	"maas-server":     suite.testMAASObject.TestServer.URL,
	"maas-agent-name": exampleAgentName,
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		panic(err)
	}
	env, err := NewEnviron(cfg)
	if err != nil {
		panic(err)
	}
	return env
}  */
