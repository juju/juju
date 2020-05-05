// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
)

type unitSuite struct {
	firewallerSuite

	apiUnit *firewaller.Unit
}

var _ = gc.Suite(&unitSuite{})

func (s *unitSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	s.apiUnit, err = s.firewaller.Unit(s.units[0].Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *unitSuite) TestUnit(c *gc.C) {
	apiUnitFoo, err := s.firewaller.Unit(names.NewUnitTag("foo/42"))
	c.Assert(err, gc.ErrorMatches, `unit "foo/42" not found`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apiUnitFoo, gc.IsNil)

	apiUnit0, err := s.firewaller.Unit(s.units[0].Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiUnit0, gc.NotNil)
	c.Assert(apiUnit0.Name(), gc.Equals, s.units[0].Name())
	c.Assert(apiUnit0.Tag(), gc.Equals, names.NewUnitTag(s.units[0].Name()))
}

func (s *unitSuite) TestRefresh(c *gc.C) {
	c.Assert(s.apiUnit.Life(), gc.Equals, life.Alive)

	err := s.units[0].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiUnit.Life(), gc.Equals, life.Alive)

	err = s.apiUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiUnit.Life(), gc.Equals, life.Dead)
}

func (s *unitSuite) TestAssignedMachine(c *gc.C) {
	machineTag, err := s.apiUnit.AssignedMachine()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineTag, gc.Equals, names.NewMachineTag(s.machines[0].Id()))

	// Unassign now and check CodeNotAssigned is reported.
	err = s.units[0].UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.apiUnit.AssignedMachine()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotAssigned)
}

func (s *unitSuite) TestApplication(c *gc.C) {
	application, err := s.apiUnit.Application()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(application.Name(), gc.Equals, s.application.Name())
}

func (s *unitSuite) TestName(c *gc.C) {
	c.Assert(s.apiUnit.Name(), gc.Equals, s.units[0].Name())
}
