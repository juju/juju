// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
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

	auto, err := gen.CanMakeCurrent()
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

	auto, err = gen.CanMakeCurrent()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auto, jc.IsFalse)

	// 2 apps; all units from one and some from the other.
	// Can not cancel or auto-complete.
	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	comp, err = gen.CanCancel()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(comp, jc.IsFalse)

	auto, err = gen.CanMakeCurrent()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auto, jc.IsFalse)

	// 2 apps; all units from both.
	// Can cancel and auto-complete.
	c.Assert(gen.AssignUnit("riak/1"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	comp, err = gen.CanCancel()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(comp, jc.IsTrue)

	auto, err = gen.CanMakeCurrent()
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

func (s *generationSuite) setupAssignAllUnits(c *gc.C) {
	charm := s.AddTestingCharm(c, "riak")
	redis := s.AddTestingApplication(c, "riak", charm)
	for i := 0; i < 4; i++ {
		_, err := redis.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)
}

func (s *generationSuite) TestAssignAllUnitsSuccessAll(c *gc.C) {
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)

	expected := map[string][]string{"riak": {"riak/0", "riak/1", "riak/2", "riak/3"}}

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, expected)

	// Idempotent.
	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, expected)
}

func (s *generationSuite) TestAssignAllUnitsSuccessRemaining(c *gc.C) {
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignUnit("riak/2"), jc.ErrorIsNil)
	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)

	expected := map[string][]string{"riak": {"riak/2", "riak/3", "riak/1", "riak/0"}}

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, expected)

	// Idempotent.
	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, expected)
}

func (s *generationSuite) TestAssignAllUnitsNotActiveError(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	// If the "next" generation is not active,
	// a call to AssignAllUnits should fail.
	c.Assert(s.Model.SwitchGeneration(model.GenerationCurrent), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignAllUnits("riak"), gc.ErrorMatches, "generation is not currently active")
}

func (s *generationSuite) setupClockforMakeCurrent(c *gc.C) {
	now := testing.NonZeroTime()
	clock := testclock.NewClock(now)
	clock.Advance(400000 * time.Hour)

	err := s.State.SetClockForTesting(clock)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *generationSuite) TestMakeCurrentSuccess(c *gc.C) {
	s.setupAssignAllUnits(c)
	s.setupClockforMakeCurrent(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent(), jc.ErrorIsNil)

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.Active(), jc.IsFalse)

	// Idempotent.
	c.Assert(gen.MakeCurrent(), jc.ErrorIsNil)
}

func (s *generationSuite) TestMakeCurrentNotActiveError(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	// If the "next" generation is not active,
	// a call to MakeCurrent should fail.
	c.Assert(s.Model.SwitchGeneration(model.GenerationCurrent), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent(), gc.ErrorMatches, "generation is not currently active")
}

func (s *generationSuite) TestMakeCurrentGenerationIncompleteError(c *gc.C) {
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent(), gc.ErrorMatches, "generation can not be completed")
}
