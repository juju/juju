// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit_test

import (
	"regexp"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudinit"
	"github.com/juju/juju/cloudinit/sshinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	envcloudinit "github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type configureSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&configureSuite{})

type testProvider struct {
	environs.EnvironProvider
}

func (p *testProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	return map[string]string{}, nil
}

func init() {
	environs.RegisterProvider("sshinit_test", &testProvider{})
}

func testConfig(c *gc.C, stateServer bool, vers version.Binary) *config.Config {
	testConfig, err := config.New(config.UseDefaults, coretesting.FakeConfig())
	c.Assert(err, gc.IsNil)
	testConfig, err = testConfig.Apply(map[string]interface{}{
		"type":           "sshinit_test",
		"default-series": vers.Series,
		"agent-version":  vers.Number.String(),
	})
	c.Assert(err, gc.IsNil)
	return testConfig
}

func (s *configureSuite) getCloudConfig(c *gc.C, stateServer bool, vers version.Binary) *cloudinit.Config {
	var mcfg *envcloudinit.MachineConfig
	var err error
	if stateServer {
		mcfg, err = environs.NewBootstrapMachineConfig(constraints.Value{}, vers.Series)
		c.Assert(err, gc.IsNil)
		mcfg.InstanceId = "instance-id"
		mcfg.Jobs = []params.MachineJob{params.JobManageEnviron, params.JobHostUnits}
	} else {
		mcfg, err = environs.NewMachineConfig("0", "ya", imagemetadata.ReleasedStream, vers.Series, nil, nil, nil)
		c.Assert(err, gc.IsNil)
		mcfg.Jobs = []params.MachineJob{params.JobHostUnits}
	}
	mcfg.Tools = &tools.Tools{
		Version: vers,
		URL:     "http://testing.invalid/tools.tar.gz",
	}
	environConfig := testConfig(c, stateServer, vers)
	err = environs.FinishMachineConfig(mcfg, environConfig)
	c.Assert(err, gc.IsNil)
	cloudcfg := cloudinit.New()
	udata, err := envcloudinit.NewUserdataConfig(mcfg, cloudcfg)
	c.Assert(err, gc.IsNil)
	err = udata.Configure()
	c.Assert(err, gc.IsNil)
	return cloudcfg
}

var allSeries = [...]string{"precise", "quantal", "raring", "saucy"}

func checkIff(checker gc.Checker, condition bool) gc.Checker {
	if condition {
		return checker
	}
	return gc.Not(checker)
}

var aptgetRegexp = "(.|\n)*" + regexp.QuoteMeta(sshinit.Aptget)

func (s *configureSuite) TestAptSources(c *gc.C) {
	for _, series := range allSeries {
		vers := version.MustParseBinary("1.16.0-" + series + "-amd64")
		script, err := sshinit.ConfigureScript(s.getCloudConfig(c, true, vers))
		c.Assert(err, gc.IsNil)

		// Only Precise requires the cloud-tools pocket.
		//
		// The only source we add that requires an explicitly
		// specified key is cloud-tools.
		needsCloudTools := series == "precise"
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			"(.|\n)*apt-key add.*(.|\n)*",
		)
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			"(.|\n)*add-apt-repository.*cloud-tools(.|\n)*",
		)
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			"(.|\n)*Pin: release n=precise-updates/cloud-tools\nPin-Priority: 400(.|\n)*",
		)
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			"(.|\n)*install -D -m 644 /dev/null '/etc/apt/preferences.d/50-cloud-tools'(.|\n)*",
		)

		// Only install python-software-properties (add-apt-repository)
		// if we need to.
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			aptgetRegexp+"install.*python-software-properties(.|\n)*",
		)
	}
}

func assertScriptMatches(c *gc.C, cfg *cloudinit.Config, pattern string, match bool) {
	script, err := sshinit.ConfigureScript(cfg)
	c.Assert(err, gc.IsNil)
	checker := gc.Matches
	if !match {
		checker = gc.Not(checker)
	}
	c.Assert(script, checker, pattern)
}

func (s *configureSuite) TestAptUpdate(c *gc.C) {
	// apt-get update is run only if AptUpdate is set.
	aptGetUpdatePattern := aptgetRegexp + "update(.|\n)*"
	cfg := cloudinit.New()

	c.Assert(cfg.AptUpdate(), gc.Equals, false)
	c.Assert(cfg.AptSources(), gc.HasLen, 0)
	assertScriptMatches(c, cfg, aptGetUpdatePattern, false)

	cfg.SetAptUpdate(true)
	assertScriptMatches(c, cfg, aptGetUpdatePattern, true)

	// If we add sources, but disable updates, display an error.
	cfg.SetAptUpdate(false)
	cfg.AddAptSource("source", "key", nil)
	_, err := sshinit.ConfigureScript(cfg)
	c.Check(err, gc.ErrorMatches, "update sources were specified, but OS updates have been disabled.")
}

func (s *configureSuite) TestAptUpgrade(c *gc.C) {
	// apt-get upgrade is only run if AptUpgrade is set.
	aptGetUpgradePattern := aptgetRegexp + "upgrade(.|\n)*"
	cfg := cloudinit.New()
	cfg.SetAptUpdate(true)
	cfg.AddAptSource("source", "key", nil)
	assertScriptMatches(c, cfg, aptGetUpgradePattern, false)
	cfg.SetAptUpgrade(true)
	assertScriptMatches(c, cfg, aptGetUpgradePattern, true)
}

func (s *configureSuite) TestAptGetWrapper(c *gc.C) {
	aptgetRegexp := "(.|\n)* $(which eatmydata || true) " + regexp.QuoteMeta(sshinit.Aptget)
	cfg := cloudinit.New()
	cfg.SetAptUpdate(true)
	cfg.SetAptGetWrapper("eatmydata")
	assertScriptMatches(c, cfg, aptgetRegexp, false)
}
