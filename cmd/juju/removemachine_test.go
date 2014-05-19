// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd/envcmd"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type RemoveMachineSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&RemoveMachineSuite{})

func runRemoveMachine(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveMachineCommand{}), args)
	return err
}

func (s *RemoveMachineSuite) TestRemoveMachineWithUnit(c *gc.C) {
	// Create a machine running a unit.
	testing.Charms.BundlePath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, gc.IsNil)

	// Get the state entities to allow sane testing.
	u, err := s.State.Unit("riak/0")
	c.Assert(err, gc.IsNil)
	mid, err := u.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	c.Assert(mid, gc.Equals, "0")

	// Try to destroy the machine and fail.
	err = runRemoveMachine(c, "0")
	c.Assert(err, gc.ErrorMatches, `no machines were destroyed: machine 0 has unit "riak/0" assigned`)
}

func (s *RemoveMachineSuite) TestDestroyEmptyMachine(c *gc.C) {
	// Destroy an empty machine alongside a state server; only the empty machine
	// gets destroyed.
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = runRemoveMachine(c, "0", "1")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 1 does not exist`)
	err = m0.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(m0.Life(), gc.Equals, state.Dying)

	// Destroying a destroyed machine is a no-op.
	err = runRemoveMachine(c, "0")
	c.Assert(err, gc.IsNil)
	err = m0.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(m0.Life(), gc.Equals, state.Dying)
}

func (s *RemoveMachineSuite) TestDestroyDeadMachine(c *gc.C) {
	// Destroying a Dead machine is a no-op; destroying it alongside a JobManageEnviron
	m0, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	// machine complains only about the JME machine.
	m1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = m1.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = runRemoveMachine(c, "0", "1")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the environment`)
	err = m1.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(m1.Life(), gc.Equals, state.Dead)
	err = m1.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(m0.Life(), gc.Equals, state.Alive)
}

func (s *RemoveMachineSuite) TestForce(c *gc.C) {
	// Create a manager machine.
	m0, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)

	// Create a machine running a unit.
	testing.Charms.BundlePath(s.SeriesPath, "riak")
	err = runDeploy(c, "local:riak", "riak")
	c.Assert(err, gc.IsNil)

	// Get the state entities to allow sane testing.
	u, err := s.State.Unit("riak/0")
	c.Assert(err, gc.IsNil)
	m1, err := s.State.Machine("1")
	c.Assert(err, gc.IsNil)

	// Try to force-destroy the machines.
	err = runRemoveMachine(c, "0", "1", "--force")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the environment`)

	// Clean up, check state.
	err = s.State.Cleanup()
	c.Assert(err, gc.IsNil)
	err = u.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = m1.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(m1.Life(), gc.Equals, state.Dead)

	err = m0.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(m0.Life(), gc.Equals, state.Alive)
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
	c.Assert(err, gc.IsNil)
	err = runRemoveMachine(c, "0", "-e", "dummyenv")
	c.Assert(err, gc.IsNil)
}
