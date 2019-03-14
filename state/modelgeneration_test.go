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
	s.setupTestingClock(c)

	c.Assert(s.Model.AddGeneration("user"), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen, gc.NotNil)
	c.Check(gen.ModelUUID(), gc.Equals, s.Model.UUID())
	c.Check(gen.Id(), gc.Not(gc.Equals), "")
	c.Check(gen.Created(), gc.Not(gc.Equals), 0)
	c.Check(gen.CreatedBy(), gc.Equals, "user")
}

func (s *generationSuite) TestNextGenerationExistsError(c *gc.C) {
	c.Assert(s.Model.AddGeneration("user"), jc.ErrorIsNil)

	_, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.Model.AddGeneration("user"), gc.ErrorMatches, "model has a next generation that is not completed")
}

func (s *generationSuite) TestAssignApplicationGenCompletedError(c *gc.C) {
	s.setupTestingClock(c)
	c.Assert(s.Model.AddGeneration("user"), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent("user"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignApplication("redis"), gc.ErrorMatches, "generation has been completed")
}

func (s *generationSuite) TestAssignApplicationSuccess(c *gc.C) {
	c.Assert(s.Model.AddGeneration("user"), jc.ErrorIsNil)

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
	s.setupTestingClock(c)
	c.Assert(s.Model.AddGeneration("user"), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent("user"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignUnit("redis/0"), gc.ErrorMatches, "generation has been completed")
}

func (s *generationSuite) TestAssignUnitSuccess(c *gc.C) {
	c.Assert(s.Model.AddGeneration("user"), jc.ErrorIsNil)

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
	c.Assert(s.Model.AddGeneration("user"), jc.ErrorIsNil)
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
	s.setupTestingClock(c)
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent("user"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignAllUnits("riak"), gc.ErrorMatches, "generation has been completed")
}

func (s *generationSuite) TestAutoCompleteSuccess(c *gc.C) {
	s.setupAssignAllUnits(c)
	s.setupTestingClock(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignAllUnits("riak"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.IsCompleted(), jc.IsFalse)

	completed, err := gen.AutoComplete("user")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(completed, jc.IsTrue)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.IsCompleted(), jc.IsTrue)
	c.Check(gen.CompletedBy(), gc.Equals, "user")

	// Idempotent.
	completed, err = gen.AutoComplete("user")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(completed, jc.IsTrue)
}

func (s *generationSuite) TestAutoCompleteGenerationIncomplete(c *gc.C) {
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	completed, err := gen.AutoComplete("user")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(completed, jc.IsFalse)
}

func (s *generationSuite) TestMakeCurrentSuccess(c *gc.C) {
	s.setupTestingClock(c)
	c.Assert(s.Model.AddGeneration("user"), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.IsCompleted(), jc.IsFalse)
	c.Assert(gen.MakeCurrent("user"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.IsCompleted(), jc.IsTrue)
	c.Check(gen.CompletedBy(), gc.Equals, "user")

	// Idempotent.
	c.Assert(gen.MakeCurrent("user"), jc.ErrorIsNil)
}

func (s *generationSuite) TestMakeCurrentCanNotCancelError(c *gc.C) {
	s.setupAssignAllUnits(c)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.MakeCurrent("user"), gc.ErrorMatches,
		"cannot cancel generation, there are units behind a generation: riak/1, riak/2, riak/3")
}

func (s *generationSuite) TestAppNoUnitsAutoCompleteErrorMakeCurrentSuccess(c *gc.C) {
	c.Assert(s.Model.AddGeneration("user"), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	// One application with changes, no units.
	mySqlCharm := s.AddTestingCharm(c, "mysql")
	mySqlApp := s.AddTestingApplication(c, "mysql", mySqlCharm)
	c.Assert(gen.AssignApplication(mySqlApp.Name()), jc.ErrorIsNil)

	// Can not auto-complete.
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	completed, err := gen.AutoComplete("user")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(completed, jc.IsFalse)

	// But can cancel.
	c.Assert(gen.MakeCurrent("user"), jc.ErrorIsNil)
}

func (s *generationSuite) TestHasNextGeneration(c *gc.C) {
	has, err := s.Model.HasNextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(has, jc.IsFalse)

	c.Assert(s.Model.AddGeneration("user"), jc.ErrorIsNil)

	has, err = s.Model.HasNextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(has, jc.IsTrue)
}

func (s *generationSuite) setupTestingClock(c *gc.C) {
	clock := testclock.NewClock(testing.NonZeroTime())
	clock.Advance(400000 * time.Hour)
	c.Assert(s.State.SetClockForTesting(clock), jc.ErrorIsNil)
}
