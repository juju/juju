// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"time"

	"github.com/juju/gomaasapi"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
)

type providerSuite struct {
	coretesting.FakeJujuHomeSuite
	envtesting.ToolsFixture
	testMAASObject *gomaasapi.TestMAASObject
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpSuite(c)
	restoreTimeouts := envtesting.PatchAttemptStrategies(&shortAttempt)
	TestMAASObject := gomaasapi.NewTestMAAS("1.0")
	s.testMAASObject = TestMAASObject
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddSuiteCleanup(func(*gc.C) {
		restoreFinishBootstrap()
		restoreTimeouts()
	})
	s.PatchValue(&nodeDeploymentTimeout, func(*maasEnviron) time.Duration {
		return coretesting.ShortWait
	})
	s.PatchValue(&resolveHostnames, func(addrs []network.Address) []network.Address {
		return addrs
	})
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&version.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(&series.HostSeries, func() string { return coretesting.FakeDefaultSeries })
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.SetFeatureFlags(feature.AddressAllocation)
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.testMAASObject.TestServer.Clear()
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuHomeSuite.TearDownTest(c)
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	s.testMAASObject.Close()
	s.FakeJujuHomeSuite.TearDownSuite(c)
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
