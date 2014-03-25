// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"

	"launchpad.net/juju-core/environs/config"
	envtesting "launchpad.net/juju-core/environs/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type providerSuite struct {
	testbase.LoggingSuite
	envtesting.ToolsFixture
	testMAASObject  *gomaasapi.TestMAASObject
	restoreTimeouts func()
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(&shortAttempt)
	s.LoggingSuite.SetUpSuite(c)
	TestMAASObject := gomaasapi.NewTestMAAS("1.0")
	s.testMAASObject = TestMAASObject
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddSuiteCleanup(func(*gc.C) { restoreFinishBootstrap() })
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.testMAASObject.TestServer.Clear()
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	s.testMAASObject.Close()
	s.restoreTimeouts()
	s.LoggingSuite.TearDownSuite(c)
}

const exampleAgentName = "dfb69555-0bc4-4d1f-85f2-4ee390974984"

var maasEnvAttrs = coretesting.Attrs{
	"name":            "test env",
	"type":            "maas",
	"maas-oauth":      "a:b:c",
	"maas-agent-name": exampleAgentName,
}

// makeEnviron creates a functional maasEnviron for a test.
func (suite *providerSuite) makeEnviron() *maasEnviron {
	testAttrs := maasEnvAttrs
	testAttrs["maas-server"] = suite.testMAASObject.TestServer.URL
	attrs := coretesting.FakeConfig().Merge(maasEnvAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		panic(err)
	}
	env, err := NewEnviron(cfg)
	if err != nil {
		panic(err)
	}
	return env
}
