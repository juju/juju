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

const (
	newBranchName    = "new-branch"
	newBranchCreator = "new-branch-user"
	branchCommitter  = "commit-user"
)

type generationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&generationSuite{})

func (s *generationSuite) TestNextGenerationNotFound(c *gc.C) {
	_, err := s.Model.Branch("non-extant-branch")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *generationSuite) TestNewGenerationSuccess(c *gc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)

	c.Assert(gen, gc.NotNil)
	c.Check(gen.ModelUUID(), gc.Equals, s.Model.UUID())
	c.Check(gen.GenerationId(), gc.Equals, 0)
	c.Check(gen.Created(), gc.Not(gc.Equals), 0)
	c.Check(gen.CreatedBy(), gc.Equals, newBranchCreator)
	c.Check(gen.BranchName(), gc.Equals, newBranchName)
	c.Check(gen.IsCompleted(), jc.IsFalse)
	c.Check(gen.CompletedBy(), gc.Equals, "")
}

func (s *generationSuite) TestAssignApplicationCompletedError(c *gc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)

	// Absence of changes will result in an aborted generation.
	_, err := gen.Commit(branchCommitter)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignApplication("redis"), gc.ErrorMatches, "branch was already aborted")
}

func (s *generationSuite) TestAssignApplicationSuccess(c *gc.C) {
	gen := s.addBranch(c)

	c.Assert(gen.AssignApplication("redis"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, map[string][]string{"redis": {}})

	// Idempotent.
	c.Assert(gen.AssignApplication("redis"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, map[string][]string{"redis": {}})
}

func (s *generationSuite) TestAssignUnitGenAbortedError(c *gc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)

	// Absence of changes will result in an aborted generation.
	_, err := gen.Commit(branchCommitter)

	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignUnit("redis/0"), gc.ErrorMatches, "branch was already aborted")
}

func (s *generationSuite) TestAssignUnitGenCommittedError(c *gc.C) {
	s.setupTestingClock(c)
	gen := s.setupAssignAllUnits(c)

	// Make a change so that commit is a real commit with a generation ID.
	c.Assert(gen.AssignApplication("riak"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	_, err := gen.Commit(branchCommitter)

	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignUnit("redis/0"), gc.ErrorMatches, "branch was already committed")
}

func (s *generationSuite) TestAssignUnitSuccess(c *gc.C) {
	gen := s.addBranch(c)

	c.Assert(gen.AssignUnit("redis/0"), jc.ErrorIsNil)

	expected := map[string][]string{"redis": {"redis/0"}}

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, expected)

	// Idempotent.
	c.Assert(gen.AssignUnit("redis/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, expected)
}

func (s *generationSuite) TestAssignAllUnitsSuccessAll(c *gc.C) {
	gen := s.setupAssignAllUnits(c)

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
	gen := s.setupAssignAllUnits(c)

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

func (s *generationSuite) TestAssignAllUnitsCompletedError(c *gc.C) {
	s.setupTestingClock(c)
	gen := s.setupAssignAllUnits(c)

	// Absence of changes will result in an aborted generation.
	_, err := gen.Commit(branchCommitter)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignAllUnits("riak"), gc.ErrorMatches, "branch was already aborted")
}

func (s *generationSuite) TestCommitAssignsRemainingUnits(c *gc.C) {
	s.setupTestingClock(c)
	gen := s.setupAssignAllUnits(c)

	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	genId, err := gen.Commit(branchCommitter)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(genId, gc.Not(gc.Equals), 0)

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.IsCompleted(), jc.IsTrue)
	c.Check(gen.CompletedBy(), gc.Equals, branchCommitter)
	c.Check(gen.AssignedUnits(), gc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], jc.SameContents, []string{"riak/0", "riak/1", "riak/2", "riak/3"})

	// Idempotent.
	_, err = gen.Commit(branchCommitter)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *generationSuite) TestCommitNoChangesEffectivelyAborted(c *gc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)

	genId, err := gen.Commit(branchCommitter)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(genId, gc.Equals, 0)

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.IsCompleted(), jc.IsTrue)
	c.Check(gen.CompletedBy(), gc.Equals, branchCommitter)
}

// TODO (manadart 2019-03-21): Tests for abort.

func (s *generationSuite) setupAssignAllUnits(c *gc.C) *state.Generation {
	charm := s.AddTestingCharm(c, "riak")
	riak := s.AddTestingApplication(c, "riak", charm)
	for i := 0; i < 4; i++ {
		_, err := riak.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
	}

	return s.addBranch(c)
}

func (s *generationSuite) addBranch(c *gc.C) *state.Generation {
	c.Assert(s.Model.AddBranch(newBranchName, newBranchCreator), jc.ErrorIsNil)
	branch, err := s.Model.Branch(newBranchName)
	c.Assert(err, jc.ErrorIsNil)
	return branch
}

func (s *generationSuite) setupTestingClock(c *gc.C) {
	clock := testclock.NewClock(testing.NonZeroTime())
	clock.Advance(400000 * time.Hour)
	c.Assert(s.State.SetClockForTesting(clock), jc.ErrorIsNil)
}
