// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type ApplicationMachinesSuite struct {
	ConnSuite
	wordpress *state.Application
	mysql     *state.Application
	machines  []*state.Machine
}

var _ = gc.Suite(&ApplicationMachinesSuite{})

func (s *ApplicationMachinesSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	s.wordpress = s.AddTestingApplication(
		c,
		"wordpress",
		s.AddTestingCharm(c, "wordpress"),
	)
	s.mysql = s.AddTestingApplication(
		c,
		"mysql",
		s.AddTestingCharm(c, "mysql"),
	)

	s.machines = make([]*state.Machine, 5)
	for i := range s.machines {
		var err error
		s.machines[i], err = s.State.AddOneMachine(state.MachineTemplate{
			Base: state.UbuntuBase("12.10"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	for _, i := range []int{0, 1, 4} {
		unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = unit.AssignToMachine(s.machines[i])
		c.Assert(err, jc.ErrorIsNil)
	}
	for _, i := range []int{2, 3} {
		unit, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = unit.AssignToMachine(s.machines[i])
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ApplicationMachinesSuite) TestApplicationMachines(c *gc.C) {
	machines, err := state.ApplicationMachines(s.State, "mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.DeepEquals, []string{"2", "3"})

	machines, err = state.ApplicationMachines(s.State, "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.DeepEquals, []string{"0", "1", "4"})

	machines, err = state.ApplicationMachines(s.State, "fred")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 0)
}
