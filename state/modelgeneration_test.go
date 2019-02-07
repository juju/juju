// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

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
	c.Check(gen.ModelUUID(), gc.Equals, s.Model.UUID())
	c.Check(gen.Id(), gc.Not(gc.Equals), "")
}

func (s *generationSuite) TestNextGenerationExistsError(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	_, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.Model.AddGeneration(), gc.ErrorMatches, "model has a next generation that is not completed")
}

func (s *generationSuite) TestCanAutoCompleteAndCanMakeCurrent(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	comp, values, err := gen.CanMakeCurrent()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(comp, jc.IsTrue)
	c.Check(values, gc.DeepEquals, []string{})

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

	comp, values, err = gen.CanMakeCurrent()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(comp, jc.IsTrue)
	c.Check(values, gc.DeepEquals, []string{})

	auto, err = gen.CanAutoComplete()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auto, jc.IsFalse)

	// 2 apps; all units from one and some from the other.
	// Can not cancel or auto-complete.
	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	comp, values, err = gen.CanMakeCurrent()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(comp, jc.IsFalse)
	c.Check(values, gc.DeepEquals, []string{"riak/1"})

	auto, err = gen.CanAutoComplete()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auto, jc.IsFalse)

	// 2 apps; all units from both.
	// Can cancel and auto-complete.
	c.Assert(gen.AssignUnit("riak/1"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	comp, values, err = gen.CanMakeCurrent()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(comp, jc.IsTrue)
	c.Check(values, gc.DeepEquals, []string{})

	auto, err = gen.CanAutoComplete()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auto, jc.IsTrue)
}

func (s *generationSuite) TestAssignApplicationGenCompletedError(c *gc.C) {
	s.setupClockForComplete(c)
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent(), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignApplication("redis"), gc.ErrorMatches, "generation has been completed")
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

func (s *generationSuite) TestAssignUnitGenCompletedError(c *gc.C) {
	s.setupClockForComplete(c)
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent(), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignUnit("redis/0"), gc.ErrorMatches, "generation has been completed")
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
	riak := s.AddTestingApplication(c, "riak", charm)
	for i := 0; i < 4; i++ {
		_, err := riak.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)
}

func (s *generationSuite) TestAssignAllUnitsSuccessAll(c *gc.C) {
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)

	expected := []string{"riak/0", "riak/1", "riak/2", "riak/3"}

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], jc.SameContents, expected)

	// Idempotent.
	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], jc.SameContents, expected)
}

func (s *generationSuite) TestAssignAllUnitsSuccessRemaining(c *gc.C) {
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignUnit("riak/2"), jc.ErrorIsNil)
	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)

	expected := []string{"riak/2", "riak/3", "riak/1", "riak/0"}

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], jc.SameContents, expected)

	// Idempotent.
	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], jc.SameContents, expected)
}

func (s *generationSuite) TestAssignAllUnitsGenCompletedError(c *gc.C) {
	s.setupClockForComplete(c)
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent(), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignAllUnits("riak"), gc.ErrorMatches, "generation has been completed")
}

func (s *generationSuite) setupClockForComplete(c *gc.C) {
	now := testing.NonZeroTime()
	clock := testclock.NewClock(now)
	clock.Advance(400000 * time.Hour)

	err := s.State.SetClockForTesting(clock)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *generationSuite) TestAutoCompleteSuccess(c *gc.C) {
	s.setupAssignAllUnits(c)
	s.setupClockForComplete(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.IsCompleted(), jc.IsFalse)

	c.Assert(gen.AutoComplete(), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.IsCompleted(), jc.IsTrue)

	// Idempotent.
	c.Assert(gen.AutoComplete(), jc.ErrorIsNil)
}

func (s *generationSuite) TestAutoCompleteGenerationIncompleteError(c *gc.C) {
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AutoComplete(), gc.ErrorMatches, "generation can not be completed")
}

func (s *generationSuite) TestMakeCurrentSuccess(c *gc.C) {
	s.setupClockForComplete(c)
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.IsCompleted(), jc.IsFalse)
	c.Assert(gen.MakeCurrent(), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.IsCompleted(), jc.IsTrue)

	// Idempotent.
	c.Assert(gen.MakeCurrent(), jc.ErrorIsNil)
}

func (s *generationSuite) TestMakeCurrentCanNotCancelError(c *gc.C) {
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent(), gc.ErrorMatches, "cannot cancel generation, there are units behind a generation: riak/1, riak/2, riak/3")
}
