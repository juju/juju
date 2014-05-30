// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type DeploySuite struct {
	testing.RepoSuite
}

var _ = gc.Suite(&DeploySuite{})

func runDeploy(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{}), args...)
	return err
}

var initErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: nil,
		err:  `no charm specified`,
	}, {
		args: []string{"charm-name", "service-name", "hotdog"},
		err:  `unrecognized args: \["hotdog"\]`,
	}, {
		args: []string{"craz~ness"},
		err:  `invalid charm name "craz~ness"`,
	}, {
		args: []string{"craziness", "burble-1"},
		err:  `invalid service name "burble-1"`,
	}, {
		args: []string{"craziness", "burble1", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{"craziness", "burble1", "--to", "bigglesplop"},
		err:  `invalid --to parameter "bigglesplop"`,
	}, {
		args: []string{"craziness", "burble1", "-n", "2", "--to", "123"},
		err:  `cannot use --num-units > 1 with --to`,
	}, {
		args: []string{"craziness", "burble1", "--constraints", "gibber=plop"},
		err:  `invalid value "gibber=plop" for flag --constraints: unknown constraint "gibber"`,
	},
}

func (s *DeploySuite) TestInitErrors(c *gc.C) {
	for i, t := range initErrorTests {
		c.Logf("test %d", i)
		err := coretesting.InitCommand(envcmd.Wrap(&DeployCommand{}), t.args)
		c.Assert(err, gc.ErrorMatches, t.err)
	}
}

func (s *DeploySuite) TestNoCharm(c *gc.C) {
	err := runDeploy(c, "local:unknown-123")
	c.Assert(err, gc.ErrorMatches, `charm not found in ".*": local:precise/unknown-123`)
}

func (s *DeploySuite) TestCharmDir(c *gc.C) {
	coretesting.Charms.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy")
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestUpgradeReportsDeprecated(c *gc.C) {
	coretesting.Charms.ClonedDirPath(s.SeriesPath, "dummy")
	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{}), "local:dummy", "-u")
	c.Assert(err, gc.IsNil)

	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")
	output := strings.Split(coretesting.Stderr(ctx), "\n")
	c.Check(output[0], gc.Matches, `Added charm ".*" to the environment.`)
	c.Check(output[1], gc.Equals, "--upgrade (or -u) is deprecated and ignored; charms are always deployed with a unique revision.")
}

func (s *DeploySuite) TestUpgradeCharmDir(c *gc.C) {
	// Add the charm, so the url will exist and a new revision will be
	// picked in ServiceDeploy.
	dummyCharm := s.AddTestingCharm(c, "dummy")

	dirPath := coretesting.Charms.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:quantal/dummy")
	c.Assert(err, gc.IsNil)
	upgradedRev := dummyCharm.Revision() + 1
	curl := dummyCharm.URL().WithRevision(upgradedRev)
	s.AssertService(c, "dummy", curl, 1, 0)
	// Check the charm dir was left untouched.
	ch, err := charm.ReadDir(dirPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
}

func (s *DeploySuite) TestCharmBundle(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)
}

func (s *DeploySuite) TestSubordinateCharm(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err := runDeploy(c, "local:logging")
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL("local:precise/logging-1")
	s.AssertService(c, "logging", curl, 0, 0)
}

func (s *DeploySuite) TestConfig(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, "local:dummy", "dummy-service", "--config", path)
	c.Assert(err, gc.IsNil)
	service, err := s.State.Service("dummy-service")
	c.Assert(err, gc.IsNil)
	settings, err := service.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"skill-level": int64(9000),
		"username":    "admin001",
	})
}

func (s *DeploySuite) TestRelativeConfigPath(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	// Putting a config file in home is okay as $HOME is set to a tempdir
	setupConfigFile(c, utils.Home())
	err := runDeploy(c, "local:dummy", "dummy-service", "--config", "~/testconfig.yaml")
	c.Assert(err, gc.IsNil)
}

func (s *DeploySuite) TestConfigError(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, "local:dummy", "other-service", "--config", path)
	c.Assert(err, gc.ErrorMatches, `no settings found for "other-service"`)
	_, err = s.State.Service("other-service")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DeploySuite) TestConstraints(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "--constraints", "mem=2G cpu-cores=2")
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	service, _ := s.AssertService(c, "dummy", curl, 1, 0)
	cons, err := service.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, constraints.MustParse("mem=2G cpu-cores=2"))
}

func (s *DeploySuite) TestNetworks(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "--networks", ", net1, net2 , ")
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	service, _ := s.AssertService(c, "dummy", curl, 1, 0)
	includeNetworks, excludeNetworks, err := service.Networks()
	c.Assert(err, gc.IsNil)
	c.Assert(includeNetworks, gc.DeepEquals, []string{"net1", "net2"})
	c.Assert(excludeNetworks, gc.HasLen, 0)
}

func (s *DeploySuite) TestSubordinateConstraints(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err := runDeploy(c, "local:logging", "--constraints", "mem=1G")
	c.Assert(err, gc.ErrorMatches, "cannot use --constraints with subordinate service")
}

func (s *DeploySuite) TestNumUnits(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-n", "13")
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "dummy", curl, 13, 0)
}

func (s *DeploySuite) TestNumUnitsSubordinate(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err := runDeploy(c, "--num-units", "3", "local:logging")
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate service")
	_, err = s.State.Service("dummy")
	c.Assert(err, gc.ErrorMatches, `service "dummy" not found`)
}

func (s *DeploySuite) assertForceMachine(c *gc.C, machineId string) {
	svc, err := s.State.Service("portlandia")
	c.Assert(err, gc.IsNil)
	units, err := svc.AllUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 1)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, gc.IsNil)
	c.Assert(mid, gc.Equals, machineId)
}

func (s *DeploySuite) TestForceMachine(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = runDeploy(c, "--to", machine.Id(), "local:dummy", "portlandia")
	c.Assert(err, gc.IsNil)
	s.assertForceMachine(c, machine.Id())
}

func (s *DeploySuite) TestForceMachineExistingContainer(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	template := state.MachineTemplate{
		Series: "precise",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXC)
	c.Assert(err, gc.IsNil)
	err = runDeploy(c, "--to", container.Id(), "local:dummy", "portlandia")
	c.Assert(err, gc.IsNil)
	s.assertForceMachine(c, container.Id())
	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNewContainer(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = runDeploy(c, "--to", "lxc:"+machine.Id(), "local:dummy", "portlandia")
	c.Assert(err, gc.IsNil)
	s.assertForceMachine(c, machine.Id()+"/lxc/0")
	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNotFound(c *gc.C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "--to", "42", "local:dummy", "portlandia")
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "portlandia/0" to machine: machine 42 not found`)
	_, err = s.State.Service("dummy")
	c.Assert(err, gc.ErrorMatches, `service "dummy" not found`)
}

func (s *DeploySuite) TestForceMachineSubordinate(c *gc.C) {
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err = runDeploy(c, "--to", machine.Id(), "local:logging")
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate service")
	_, err = s.State.Service("dummy")
	c.Assert(err, gc.ErrorMatches, `service "dummy" not found`)
}
