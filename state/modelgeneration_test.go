// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
)

type generationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&generationSuite{})

func (s *generationSuite) TestNextGenerationNotFound(c *gc.C) {
	_, err := s.Model.NextGeneration()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *generationSuite) TestNextGenerationSuccess(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen, gc.NotNil)

	// A newly created generation is immediately the active one.
	c.Check(gen.Active(), jc.IsTrue)

	v, err := s.Model.ActiveGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, gc.Equals, model.GenerationNext)

	c.Check(gen.ModelUUID(), gc.Equals, s.Model.UUID())
	c.Check(gen.Id(), gc.Not(gc.Equals), "")
}

func (s *generationSuite) TestNextGenerationExistsError(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	_, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.Model.AddGeneration(), gc.ErrorMatches, "model has a next generation that is not completed")
}

func (s *generationSuite) TestActiveGenerationSwitchSuccess(c *gc.C) {
	v, err := s.Model.ActiveGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, gc.Equals, model.GenerationCurrent)

	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	v, err = s.Model.ActiveGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, gc.Equals, model.GenerationNext)

	c.Assert(s.Model.SwitchGeneration(model.GenerationCurrent), jc.ErrorIsNil)

	v, err = s.Model.ActiveGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, gc.Equals, model.GenerationCurrent)
}

func (s *generationSuite) TestCanAutoCompleteAndCanCancel(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	comp, err := gen.CanCancel()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(comp, jc.IsTrue)

	auto, err := gen.CanAutoComplete()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auto, jc.IsFalse)

	mySqlCharm := s.AddTestingCharm(c, "mysql")
	mySqlApp := s.AddTestingApplication(c, "mysql", mySqlCharm)
	_, err = mySqlApp.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	riakCharm := s.AddTestingCharm(c, "riak")
	riakApp := s.AddTestingApplication(c, "riak", riakCharm)
	_, err = riakApp.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = riakApp.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// 2 apps; all units from one and none from the other.
	// Can cancel, but not auto-complete.
	c.Assert(gen.AssignUnit("mysql/0"), jc.ErrorIsNil)
	c.Assert(gen.AssignApplication("riak"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	comp, err = gen.CanCancel()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(comp, jc.IsTrue)

	auto, err = gen.CanAutoComplete()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auto, jc.IsFalse)

	// 2 apps; all units from one and some from the other.
	// Can not cancel or auto-complete.
	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	comp, err = gen.CanCancel()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(comp, jc.IsFalse)

	auto, err = gen.CanAutoComplete()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auto, jc.IsFalse)

	// 2 apps; all units from both.
	// Can cancel and auto-complete.
	c.Assert(gen.AssignUnit("riak/1"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	comp, err = gen.CanCancel()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(comp, jc.IsTrue)

	auto, err = gen.CanAutoComplete()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auto, jc.IsTrue)
}

func (s *generationSuite) TestAssignApplicationNotActiveError(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	// If the "next" generation is not active, a call to AssignApplication,
	// such as would be made accompanying a configuration change,
	// should not succeed.
	c.Assert(s.Model.SwitchGeneration(model.GenerationCurrent), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignApplication("redis"), gc.ErrorMatches, "generation is not currently active")
}

func (s *generationSuite) TestAssignApplicationGenerationCompletedError(c *gc.C) {
	c.Skip("Test to be written once generation completion logic is implemented")
}

func (s *generationSuite) TestAssignApplicationSuccess(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignApplication("redis"), jc.ErrorIsNil)

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, map[string][]string{"redis": {}})

	// Idempotent.
	c.Assert(gen.AssignApplication("redis"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, map[string][]string{"redis": {}})
}

func (s *generationSuite) TestAssignUnitNotActiveError(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	// If the "next" generation is not active,
	// a call to AssignUnit should fail.
	c.Assert(s.Model.SwitchGeneration(model.GenerationCurrent), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignUnit("redis/0"), gc.ErrorMatches, "generation is not currently active")
}

func (s *generationSuite) TestAssignUnitGenerationCompletedError(c *gc.C) {
	c.Skip("Test to be written once generation completion logic is implemented")
}

func (s *generationSuite) TestAssignUnitSuccess(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignUnit("redis/0"), jc.ErrorIsNil)

	expected := map[string][]string{"redis": {"redis/0"}}

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, expected)

	// Idempotent.
	c.Assert(gen.AssignUnit("redis/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, expected)
}
