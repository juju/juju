// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	envtools "launchpad.net/juju-core/environs/tools"
	_ "launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type agentSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&agentSuite{})

func dummyConfig(c *gc.C, stateServer bool, vers version.Binary) *config.Config {
	testConfig, err := config.New(config.UseDefaults, coretesting.FakeConfig())
	c.Assert(err, gc.IsNil)
	testConfig, err = testConfig.Apply(map[string]interface{}{
		"type":           "dummy",
		"state-server":   stateServer,
		"default-series": vers.Series,
		"agent-version":  vers.Number.String(),
	})
	c.Assert(err, gc.IsNil)
	return testConfig
}

func (s *agentSuite) getMachineConfig(c *gc.C, stateServer bool, vers version.Binary) *cloudinit.MachineConfig {
	var mcfg *cloudinit.MachineConfig
	if stateServer {
		mcfg = environs.NewBootstrapMachineConfig("http://whatever/dotcom")
	} else {
		mcfg = environs.NewMachineConfig("0", "ya", nil, nil)
	}
	mcfg.Tools = &tools.Tools{
		Version: vers,
		URL:     "file:///var/lib/juju/storage/" + envtools.StorageName(vers),
	}
	environConfig := dummyConfig(c, stateServer, vers)
	err := environs.FinishMachineConfig(mcfg, environConfig, constraints.Value{})
	c.Assert(err, gc.IsNil)
	return mcfg
}

var allSeries = [...]string{"precise", "quantal", "raring", "saucy"}

func checkIff(checker gc.Checker, condition bool) gc.Checker {
	if condition {
		return checker
	}
	return gc.Not(checker)
}

func (s *agentSuite) TestAptSources(c *gc.C) {
	for _, series := range allSeries {
		vers := version.MustParseBinary("1.16.0-" + series + "-amd64")
		script, err := provisionMachineAgentScript(s.getMachineConfig(c, true, vers))
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
			"(.|\n)*apt-add-repository.*cloud-tools(.|\n)*",
		)

		// Only Quantal requires the PPA (for mongo).
		needsJujuPPA := series == "quantal"
		c.Assert(
			script,
			checkIff(gc.Matches, needsJujuPPA),
			"(.|\n)*apt-add-repository.*ppa:juju/stable(.|\n)*",
		)

		// Only install python-software-properties (apt-add-repository)
		// if we need to.
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools || needsJujuPPA),
			"(.|\n)*apt-get -y install.*python-software-properties(.|\n)*",
		)
	}
}
