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

func (s *UnitAssignmentSuite) testAddApplicationUnitAssignment(c *gc.C) (*state.Application, []state.UnitAssignment) {
	charm := s.AddTestingCharm(c, "dummy")
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "dummy", Charm: charm, NumUnits: 2,
		Placement: []*instance.Placement{{s.State.ModelUUID(), "abc"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	units, err := app.AllUnits()
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
	return app, assignments
}

func (s *UnitAssignmentSuite) TestAddApplicationUnitAssignment(c *gc.C) {
	s.testAddApplicationUnitAssignment(c)
}

func (s *UnitAssignmentSuite) TestAssignStagedUnits(c *gc.C) {
	app, _ := s.testAddApplicationUnitAssignment(c)

	results, err := s.State.AssignStagedUnits([]string{
		"dummy/0", "dummy/1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.SameContents, []state.UnitAssignmentResult{
		{Unit: "dummy/0"},
		{Unit: "dummy/1"},
	})

	units, err := app.AllUnits()
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
	// Enables juju deploy <charm> --to <container-type>
	// It creates a new machine with a new container of that type.
	// https://bugs.launchpad.net/juju-core/+bug/1590960
	charm := s.AddTestingCharm(c, "dummy")
	placement := instance.Placement{Scope: "lxd"}
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:      "dummy",
		Charm:     charm,
		NumUnits:  1,
		Placement: []*instance.Placement{&placement},
	})
	c.Assert(err, jc.ErrorIsNil)
	units, err := app.AllUnits()
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

func (s *UnitAssignmentSuite) TestAssignUnitMachinePlacementUpgradeSeriesLockError(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.CreateUpgradeSeriesLock(nil, "trusty"), jc.ErrorIsNil)

	// As in --to 0
	s.testPlacementUpgradeSeriesLockError(c, &instance.Placement{Scope: "#", Directive: machine.Id()})
}

func (s *UnitAssignmentSuite) TestAssignUnitContainerOnMachinePlacementUpgradeSeriesLockError(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.CreateUpgradeSeriesLock(nil, "trusty"), jc.ErrorIsNil)

	// As in --to lxd:0
	s.testPlacementUpgradeSeriesLockError(c, &instance.Placement{Scope: "lxd", Directive: machine.Id()})
}

func (s *UnitAssignmentSuite) testPlacementUpgradeSeriesLockError(c *gc.C, placement *instance.Placement) {
	charm := s.AddTestingCharm(c, "dummy")
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:      "dummy",
		Charm:     charm,
		NumUnits:  1,
		Placement: []*instance.Placement{placement},
	})
	c.Assert(err, jc.ErrorIsNil)
	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	unit := units[0]
	err = s.State.AssignUnitWithPlacement(unit, placement)
	c.Assert(err, gc.ErrorMatches, `machine "\d" is locked for series upgrade`)
}
