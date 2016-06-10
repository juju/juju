// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type UnitAssignmentSuite struct {
	ConnSuite
}

var _ = gc.Suite(&UnitAssignmentSuite{})

func (s *UnitAssignmentSuite) testAddServiceUnitAssignment(c *gc.C) (*state.Application, []state.UnitAssignment) {
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "dummy", Owner: s.Owner.String(),
		Charm: charm, NumUnits: 2,
		Placement: []*instance.Placement{{s.State.ModelUUID(), "abc"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 2)
	for _, u := range units {
		_, err := u.AssignedMachineId()
		c.Assert(err, jc.Satisfies, errors.IsNotAssigned)
	}

	assignments, err := s.State.AllUnitAssignments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignments, jc.SameContents, []state.UnitAssignment{
		{Unit: "dummy/0", Scope: s.State.ModelUUID(), Directive: "abc"},
		{Unit: "dummy/1"},
	})
	return svc, assignments
}

func (s *UnitAssignmentSuite) TestAddServiceUnitAssignment(c *gc.C) {
	s.testAddServiceUnitAssignment(c)
}

func (s *UnitAssignmentSuite) TestAssignStagedUnits(c *gc.C) {
	svc, _ := s.testAddServiceUnitAssignment(c)

	results, err := s.State.AssignStagedUnits([]string{
		"dummy/0", "dummy/1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.SameContents, []state.UnitAssignmentResult{
		{Unit: "dummy/0"},
		{Unit: "dummy/1"},
	})

	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 2)
	for _, u := range units {
		_, err := u.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
	}

	// There should be no staged assignments now.
	assignments, err := s.State.AllUnitAssignments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignments, gc.HasLen, 0)
}

func (s *UnitAssignmentSuite) TestAssignUnitWithPlacementMakesContainerInNewMachine(c *gc.C) {
	// Enables juju deploy <charm> --to lxd
	// https://bugs.launchpad.net/juju-core/+bug/1590960
	charm := s.AddTestingCharm(c, "dummy")
	placement := instance.Placement{Scope: "lxd"}
	svc, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "dummy", Owner: s.Owner.String(),
		Charm: charm, NumUnits: 1,
		Placement: []*instance.Placement{&placement},
	})
	c.Assert(err, jc.ErrorIsNil)
	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
	unit := units[0]

	err = s.State.AssignUnitWithPlacement(unit, &placement)
	c.Assert(err, jc.ErrorIsNil)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	parentId, isContainer := machine.ParentId()
	c.Assert(isContainer, jc.IsTrue)
	_, err = s.State.Machine(parentId)
	c.Assert(err, jc.ErrorIsNil)
}
