// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type RemoveMachineSuite struct {
	jujutesting.RepoSuite
	machine *state.Machine
}

var _ = gc.Suite(&RemoveMachineSuite{})

func runRemoveMachine(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveMachineCommand{}), args...)
	return err
}

func (s *RemoveMachineSuite) setupMachineWithUnit(c *gc.C) {
	// Create a machine running a unit.
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, jc.ErrorIsNil)

	s.machine, err = s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	// Get the state entities to allow sane testing.
	u, err := s.State.Unit("riak/0")
	c.Assert(err, jc.ErrorIsNil)
	mid, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, "0")
}

func assertLife(c *gc.C, entity state.Living, life state.Life) {
	err := entity.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Life(), gc.Equals, life)
}

func (s *RemoveMachineSuite) TestBlockRemoveMachineWithUnit(c *gc.C) {
	s.setupMachineWithUnit(c)

	// Block operation
	s.AssertConfigParameterUpdated(c, "block-remove-object", true)
	// Try to destroy the machine and fail.
	err := runRemoveMachine(c, "0")
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
	assertLife(c, s.machine, state.Alive)

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To unblock removal.*")
}

func (s *RemoveMachineSuite) TestRemoveMachineWithUnit(c *gc.C) {
	s.setupMachineWithUnit(c)

	// Try to destroy the machine and fail.
	err := runRemoveMachine(c, "0")
	c.Assert(err, gc.ErrorMatches, `no machines were destroyed: machine 0 has unit "riak/0" assigned`)
	assertLife(c, s.machine, state.Alive)
}

func (s *RemoveMachineSuite) TestBlockDestroyMachine(c *gc.C) {
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// Block operation
	s.AssertConfigParameterUpdated(c, "block-remove-object", true)
	// Try to destroy the machine and fail.
	err = runRemoveMachine(c, "0")
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
	assertLife(c, m0, state.Alive)

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To unblock removal.*")
}

func (s *RemoveMachineSuite) TestDestroyEmptyMachine(c *gc.C) {
	// Destroy an empty machine alongside a state server; only the empty machine
	// gets destroyed.
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = runRemoveMachine(c, "0", "1")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 1 does not exist`)
	assertLife(c, m0, state.Dying)

	// Destroying a destroyed machine is a no-op.
	err = runRemoveMachine(c, "0")
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, m0, state.Dying)
}

func (s *RemoveMachineSuite) TestDestroyDeadMachine(c *gc.C) {
	// Destroying a Dead machine is a no-op; destroying it alongside a JobManageEnviron
	m0, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	// machine complains only about the JME machine.
	m1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = m1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = runRemoveMachine(c, "0", "1")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the environment`)
	assertLife(c, m0, state.Alive)
}

func (s *RemoveMachineSuite) TestBlockForceRemoveMachine(c *gc.C) {
	// Block operation
	s.AssertConfigParameterUpdated(c, "block-remove-object", true)
	//should not be affected
	s.forceRemoveMachine(c)
}

func (s *RemoveMachineSuite) forceRemoveMachine(c *gc.C) {
	// Create a manager machine.
	m0, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)

	// Create a machine running a unit.
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "riak")
	err = runDeploy(c, "local:riak", "riak")
	c.Assert(err, jc.ErrorIsNil)

	// Get the state entities to allow sane testing.
	u, err := s.State.Unit("riak/0")
	c.Assert(err, jc.ErrorIsNil)
	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)

	// Try to force-destroy the machines.
	err = runRemoveMachine(c, "0", "1", "--force")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the environment`)

	// Clean up, check state.
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Dead)
}

func (s *RemoveMachineSuite) TestForce(c *gc.C) {
	s.forceRemoveMachine(c)
}

func (s *RemoveMachineSuite) TestBadArgs(c *gc.C) {
	// Check invalid args.
	err := runRemoveMachine(c)
	c.Assert(err, gc.ErrorMatches, `no machines specified`)
	err = runRemoveMachine(c, "1", "2", "nonsense", "rubbish")
	c.Assert(err, gc.ErrorMatches, `invalid machine id "nonsense"`)
}

func (s *RemoveMachineSuite) TestEnvironmentArg(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = runRemoveMachine(c, "0", "-e", "dummyenv")
	c.Assert(err, jc.ErrorIsNil)
}
