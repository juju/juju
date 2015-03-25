// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type AddUnitSuite struct {
	jujutesting.RepoSuite
	CmdBlockHelper
}

func (s *AddUnitSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&AddUnitSuite{})

var initAddUnitErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"some-service-name", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{"some-service-name", "--to", "bigglesplop"},
		err:  `invalid --to parameter "bigglesplop"`,
	}, {
		args: []string{"some-service-name", "-n", "2", "--to", "123"},
		err:  `cannot use --num-units > 1 with --to`,
	},
}

func (s *AddUnitSuite) TestInitErrors(c *gc.C) {
	for i, t := range initAddUnitErrorTests {
		c.Logf("test %d", i)
		err := testing.InitCommand(envcmd.Wrap(&AddUnitCommand{}), t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func runAddUnit(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&AddUnitCommand{}), args...)
	return err
}

func (s *AddUnitSuite) setupService(c *gc.C) *charm.URL {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)
	return curl
}

func (s *AddUnitSuite) TestAddUnit(c *gc.C) {
	curl := s.setupService(c)

	err := runAddUnit(c, "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertService(c, "some-service-name", curl, 2, 0)

	err = runAddUnit(c, "--num-units", "2", "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertService(c, "some-service-name", curl, 4, 0)
}

func (s *AddUnitSuite) TestBlockAddUnit(c *gc.C) {
	s.setupService(c)

	// Block operation
	s.BlockAllChanges(c, "TestBlockAddUnit")
	s.AssertBlocked(c, runAddUnit(c, "some-service-name"), ".*TestBlockAddUnit.*")
}

// assertForceMachine ensures that the result of assigning a unit with --to
// is as expected.
func (s *AddUnitSuite) assertForceMachine(c *gc.C, svc *state.Service, expectedNumMachines, unitNum int, machineId string) {
	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, expectedNumMachines)
	mid, err := units[unitNum].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machineId)
}

func (s *AddUnitSuite) TestForceMachine(c *gc.C) {
	curl := s.setupService(c)
	machine, err := s.State.AddMachine(testing.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	machine2, err := s.State.AddMachine(testing.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = runAddUnit(c, "some-service-name", "--to", machine2.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = runAddUnit(c, "some-service-name", "--to", machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	svc, _ := s.AssertService(c, "some-service-name", curl, 3, 0)
	s.assertForceMachine(c, svc, 3, 1, machine2.Id())
	s.assertForceMachine(c, svc, 3, 2, machine.Id())
}

func (s *AddUnitSuite) TestBlockForceMachine(c *gc.C) {
	curl := s.setupService(c)
	machine, err := s.State.AddMachine(testing.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	machine2, err := s.State.AddMachine(testing.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = runAddUnit(c, "some-service-name", "--to", machine2.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = runAddUnit(c, "some-service-name", "--to", machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	svc, _ := s.AssertService(c, "some-service-name", curl, 3, 0)
	// Block operation: should be ignored :)
	s.BlockAllChanges(c, "TestBlockForceMachine")
	s.assertForceMachine(c, svc, 3, 1, machine2.Id())
	s.assertForceMachine(c, svc, 3, 2, machine.Id())
}

func (s *AddUnitSuite) TestForceMachineExistingContainer(c *gc.C) {
	curl := s.setupService(c)
	machine, err := s.State.AddMachine(testing.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	template := state.MachineTemplate{
		Series: testing.FakeDefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)

	err = runAddUnit(c, "some-service-name", "--to", container.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = runAddUnit(c, "some-service-name", "--to", machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	svc, _ := s.AssertService(c, "some-service-name", curl, 3, 0)
	s.assertForceMachine(c, svc, 3, 1, container.Id())
	s.assertForceMachine(c, svc, 3, 2, machine.Id())
}

func (s *AddUnitSuite) TestForceMachineNewContainer(c *gc.C) {
	curl := s.setupService(c)
	machine, err := s.State.AddMachine(testing.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = runAddUnit(c, "some-service-name", "--to", "lxc:"+machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = runAddUnit(c, "some-service-name", "--to", machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	svc, _ := s.AssertService(c, "some-service-name", curl, 3, 0)
	s.assertForceMachine(c, svc, 3, 1, machine.Id()+"/lxc/0")
	s.assertForceMachine(c, svc, 3, 2, machine.Id())
}

func (s *AddUnitSuite) TestNonLocalCannotHostUnits(c *gc.C) {
	err := runAddUnit(c, "some-service-name", "--to", "0")
	c.Assert(err, gc.Not(gc.ErrorMatches), "machine 0 is the state server for a local environment and cannot host units")
}

func (s *AddUnitSuite) TestBlockNonLocalCannotHostUnits(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockNonLocalCannotHostUnits")
	s.AssertBlocked(c, runAddUnit(c, "some-service-name", "--to", "0"), ".*TestBlockNonLocalCannotHostUnits.*")
}

func (s *AddUnitSuite) TestCannotDeployToNonExistentMachine(c *gc.C) {
	s.setupService(c)
	err := runAddUnit(c, "some-service-name", "--to", "42")
	c.Assert(err, gc.ErrorMatches, `cannot add units for service "some-service-name" to machine 42: machine 42 not found`)
}

func (s *AddUnitSuite) TestBlockCannotDeployToNonExistentMachine(c *gc.C) {
	s.setupService(c)
	// Block operation
	s.BlockAllChanges(c, "TestBlockCannotDeployToNonExistentMachine")
	s.AssertBlocked(c, runAddUnit(c, "some-service-name", "--to", "42"), ".*TestBlockCannotDeployToNonExistentMachine.*")
}

type AddUnitLocalSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&AddUnitLocalSuite{})

func (s *AddUnitLocalSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)

	// override provider type
	s.PatchValue(&getClientConfig, func(client *api.Client) (*config.Config, error) {
		attrs, err := client.EnvironmentGet()
		if err != nil {
			return nil, err
		}
		attrs["type"] = "local"
		return config.New(config.NoDefaults, attrs)
	})
}

func (s *AddUnitLocalSuite) TestLocalCannotHostUnits(c *gc.C) {
	err := runAddUnit(c, "some-service-name", "--to", "0")
	c.Assert(err, gc.ErrorMatches, "machine 0 is the state server for a local environment and cannot host units")
}

type namesSuite struct {
}

var _ = gc.Suite(&namesSuite{})

func (*namesSuite) TestNameChecks(c *gc.C) {
	assertMachineOrNewContainer := func(s string, expect bool) {
		c.Logf("%s -> %v", s, expect)
		c.Assert(isMachineOrNewContainer(s), gc.Equals, expect)
	}
	assertMachineOrNewContainer("0", true)
	assertMachineOrNewContainer("00", false)
	assertMachineOrNewContainer("1", true)
	assertMachineOrNewContainer("0/lxc/0", true)
	assertMachineOrNewContainer("lxc:0", true)
	assertMachineOrNewContainer("lxc:lxc:0", false)
	assertMachineOrNewContainer("kvm:0/lxc/1", true)
	assertMachineOrNewContainer("lxc:", false)
	assertMachineOrNewContainer(":lxc", false)
	assertMachineOrNewContainer("0/lxc/", false)
	assertMachineOrNewContainer("0/lxc", false)
	assertMachineOrNewContainer("kvm:0/lxc", false)
	assertMachineOrNewContainer("0/lxc/01", false)
	assertMachineOrNewContainer("0/lxc/10", true)
	assertMachineOrNewContainer("0/kvm/4", true)
}
