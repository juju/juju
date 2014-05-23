// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/constraints"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
	coretesting "launchpad.net/juju-core/testing"
)

type EnsureAvailabilitySuite struct {
	jujutesting.RepoSuite
	machine0Pinger *presence.Pinger
}

var _ = gc.Suite(&EnsureAvailabilitySuite{})

func (s *EnsureAvailabilitySuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	// Add a state server to the environment, and ensure that it is
	// considered 'alive' so that calls don't spawn new instances
	_, err := s.State.AddMachine("precise", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	m, err := s.BackingState.Machine("0")
	c.Assert(err, gc.IsNil)
	s.machine0Pinger, err = m.SetAgentAlive()
	c.Assert(err, gc.IsNil)
	s.BackingState.StartSync()
	err = m.WaitAgentAlive(coretesting.LongWait)
	c.Assert(err, gc.IsNil)
}

func (s *EnsureAvailabilitySuite) TearDownTest(c *gc.C) {
	// We have to Kill the Pinger before TearDownTest, otherwise the State
	// connection is already closed.
	if s.machine0Pinger != nil {
		s.machine0Pinger.Kill()
		s.machine0Pinger = nil
	}
	s.RepoSuite.TearDownTest(c)
}

func runEnsureAvailability(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&EnsureAvailabilityCommand{}), args...)
	return err
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailability(c *gc.C) {
	err := runEnsureAvailability(c, "-n", "1")
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, state.Alive)
	c.Assert(m.Series(), gc.DeepEquals, "precise")
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityWithSeries(c *gc.C) {
	err := runEnsureAvailability(c, "--series", "series", "-n", "3")
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine("1")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Series(), gc.DeepEquals, "series")
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityWithConstraints(c *gc.C) {
	err := runEnsureAvailability(c, "--constraints", "mem=4G", "-n", "3")
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine("1")
	c.Assert(err, gc.IsNil)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	expectedCons := constraints.MustParse("mem=4G")
	c.Assert(mcons, gc.DeepEquals, expectedCons)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityIdempotent(c *gc.C) {
	for i := 0; i < 2; i++ {
		err := runEnsureAvailability(c, "-n", "1")
		c.Assert(err, gc.IsNil)
	}
	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 1)
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)

	// Running ensure-availability with constraints or series
	// will have no effect unless new machines are
	// created.
	err = runEnsureAvailability(c, "-n", "1", "--constraints", "mem=4G")
	c.Assert(err, gc.IsNil)
	m, err = s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	mcons, err = m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityMultiple(c *gc.C) {
	err := runEnsureAvailability(c, "-n", "3", "--constraints", "mem=4G")
	c.Assert(err, gc.IsNil)

	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 3)
	mcons, err := machines[0].Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	for i := 1; i < 3; i++ {
		mcons, err := machines[i].Constraints()
		c.Assert(err, gc.IsNil)
		expectedCons := constraints.MustParse("mem=4G")
		c.Assert(mcons, gc.DeepEquals, expectedCons)
	}
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityErrors(c *gc.C) {
	for _, n := range []int{-1, 2} {
		err := runEnsureAvailability(c, "-n", fmt.Sprint(n))
		c.Assert(err, gc.ErrorMatches, "must specify a number of state servers odd and non-negative")
	}
	err := runEnsureAvailability(c, "-n", "3")
	c.Assert(err, gc.IsNil)
	err = runEnsureAvailability(c, "-n", "1")
	c.Assert(err, gc.ErrorMatches, "cannot reduce state server count")
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityAllows0(c *gc.C) {
	err := runEnsureAvailability(c, "-n", "0")
	c.Assert(err, gc.IsNil)
	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 3)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityDefaultsTo3(c *gc.C) {
	err := runEnsureAvailability(c)
	c.Assert(err, gc.IsNil)
	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 3)
}
