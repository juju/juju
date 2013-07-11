// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
)

type DeploySuite struct {
	testing.RepoSuite
}

var _ = Suite(&DeploySuite{})

func runDeploy(c *C, args ...string) error {
	_, err := coretesting.RunCommand(c, &DeployCommand{}, args)
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
		args: []string{"craz~ness"},
		err:  `invalid charm name "craz~ness"`,
	}, {
		args: []string{"craziness", "burble-1"},
		err:  `invalid service name "burble-1"`,
	}, {
		args: []string{"craziness", "burble1", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{"craziness", "burble1", "--force-machine", "bigglesplop"},
		err:  `invalid force machine id "bigglesplop"`,
	}, {
		args: []string{"craziness", "burble1", "-n", "2", "--force-machine", "123"},
		err:  `cannot use --num-units with --force-machine`,
	}, {
		args: []string{"craziness", "burble1", "--constraints", "gibber=plop"},
		err:  `invalid value "gibber=plop" for flag --constraints: unknown constraint "gibber"`,
	},
}

func (s *DeploySuite) TestInitErrors(c *C) {
	for i, t := range initErrorTests {
		c.Logf("test %d", i)
		err := coretesting.InitCommand(&DeployCommand{}, t.args)
		c.Assert(err, ErrorMatches, t.err)
	}
}

func (s *DeploySuite) TestNoCharm(c *C) {
	err := runDeploy(c, "local:unknown-123")
	c.Assert(err, ErrorMatches, `cannot get charm: charm not found in ".*": local:precise/unknown-123`)
}

func (s *DeploySuite) TestCharmDir(c *C) {
	coretesting.Charms.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestUpgradeCharmDir(c *C) {
	dirPath := coretesting.Charms.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-u")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-2")
	s.AssertService(c, "dummy", curl, 1, 0)
	// Check the charm really was upgraded.
	ch, err := charm.ReadDir(dirPath)
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, 2)
}

func (s *DeploySuite) TestCharmBundle(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)
}

func (s *DeploySuite) TestCannotUpgradeCharmBundle(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-u")
	c.Assert(err, ErrorMatches, `cannot increment revision of charm "local:precise/dummy-1": not a directory`)
	// Verify state not touched...
	curl := charm.MustParseURL("local:precise/dummy-1")
	_, err = s.State.Charm(curl)
	c.Assert(err, ErrorMatches, `charm "local:precise/dummy-1" not found`)
	_, err = s.State.Service("dummy")
	c.Assert(err, ErrorMatches, `service "dummy" not found`)
}

func (s *DeploySuite) TestSubordinateCharm(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err := runDeploy(c, "local:logging")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/logging-1")
	s.AssertService(c, "logging", curl, 0, 0)
}

func (s *DeploySuite) TestConfig(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	path := setupConfigfile(c, c.MkDir())
	err := runDeploy(c, "local:dummy", "dummy-service", "--config", path)
	c.Assert(err, IsNil)
	service, err := s.State.Service("dummy-service")
	c.Assert(err, IsNil)
	settings, err := service.ConfigSettings()
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, charm.Settings{
		"skill-level": int64(9000),
		"username":    "admin001",
	})
}

func (s *DeploySuite) TestConfigError(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	path := setupConfigfile(c, c.MkDir())
	err := runDeploy(c, "local:dummy", "other-service", "--config", path)
	c.Assert(err, ErrorMatches, `no settings found for "other-service"`)
	_, err = s.State.Service("other-service")
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *DeploySuite) TestConstraints(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "--constraints", "mem=2G cpu-cores=2")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	service, _ := s.AssertService(c, "dummy", curl, 1, 0)
	cons, err := service.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, constraints.MustParse("mem=2G cpu-cores=2"))
}

func (s *DeploySuite) TestSubordinateConstraints(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err := runDeploy(c, "local:logging", "--constraints", "mem=1G")
	c.Assert(err, ErrorMatches, "cannot use --constraints with subordinate service")
}

func (s *DeploySuite) TestNumUnits(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-n", "13")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "dummy", curl, 13, 0)
}

func (s *DeploySuite) TestNumUnitsSubordinate(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err := runDeploy(c, "--num-units", "3", "local:logging")
	c.Assert(err, ErrorMatches, "cannot use --num-units or --force-machine with subordinate service")
	_, err = s.State.Service("dummy")
	c.Assert(err, ErrorMatches, `service "dummy" not found`)
}

func (s *DeploySuite) assertForceMachine(c *C, machineId string) {
	svc, err := s.State.Service("portlandia")
	c.Assert(err, IsNil)
	units, err := svc.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 1)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, machineId)
}

func (s *DeploySuite) TestForceMachine(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = runDeploy(c, "--force-machine", machine.Id(), "local:dummy", "portlandia")
	c.Assert(err, IsNil)
	s.assertForceMachine(c, machine.Id())
}

func (s *DeploySuite) TestForceMachineExistingContainer(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	params := &state.AddMachineParams{
		Series:        "precise",
		ContainerType: instance.LXC,
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineWithConstraints(params)
	c.Assert(err, IsNil)
	err = runDeploy(c, "--force-machine", container.Id(), "local:dummy", "portlandia")
	c.Assert(err, IsNil)
	s.assertForceMachine(c, container.Id())
	ms, err := s.State.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(ms, HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNewContainer(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = runDeploy(c, "--force-machine", machine.Id()+"/lxc", "local:dummy", "portlandia")
	c.Assert(err, IsNil)
	s.assertForceMachine(c, machine.Id()+"/lxc/0")
	ms, err := s.State.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(ms, HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNotFound(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "--force-machine", "42", "local:dummy", "portlandia")
	c.Assert(err, ErrorMatches, `cannot assign unit "portlandia/0" to machine: machine 42 not found`)
	_, err = s.State.Service("dummy")
	c.Assert(err, ErrorMatches, `service "dummy" not found`)
}

func (s *DeploySuite) TestForceMachineSubordinate(c *C) {
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, IsNil)
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err = runDeploy(c, "--force-machine", machine.Id(), "local:logging")
	c.Assert(err, ErrorMatches, "cannot use --num-units or --force-machine with subordinate service")
	_, err = s.State.Service("dummy")
	c.Assert(err, ErrorMatches, `service "dummy" not found`)
}
