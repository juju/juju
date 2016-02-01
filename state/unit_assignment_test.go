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

func (s *UnitAssignmentSuite) testAddServiceUnitAssignment(c *gc.C) (*state.Service, []state.UnitAssignment) {
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService(state.AddServiceArgs{
		Name: "dummy", Owner: s.Owner.String(),
		Charm: charm, NumUnits: 2,
		Placement: []*instance.Placement{{s.State.ModelUUID(), "abc"}},
	})
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
