// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

const (
	newBranchName    = "new-branch"
	newBranchCreator = "new-branch-user"
	branchCommitter  = "commit-user"
)

type generationSuite struct {
	ConnSuite

	ch *state.Charm
}

var _ = gc.Suite(&generationSuite{})

func (s *generationSuite) TestBranchNameNotFound(c *gc.C) {
	_, err := s.Model.Branch("non-extant-branch")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *generationSuite) TestAddBranchSuccess(c *gc.C) {
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

func (s *generationSuite) TestAssignUnitBranchAbortedError(c *gc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)

	// Absence of changes will result in an aborted generation.
	_, err := gen.Commit(branchCommitter)

	c.Assert(err, jc.ErrorIsNil)

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Assert(gen.AssignUnit("redis/0"), gc.ErrorMatches, "branch was already aborted")
}

func (s *generationSuite) TestAssignUnitNotExistsError(c *gc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)
	c.Assert(gen.AssignUnit("redis/0"), gc.ErrorMatches, `unit "redis/0" not found`)
}

func (s *generationSuite) TestAssignUnitBranchCommittedError(c *gc.C) {
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
	s.setupTestingClock(c)
	gen := s.setupAssignAllUnits(c)

	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)

	expected := map[string][]string{"riak": {"riak/0"}}

	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), gc.DeepEquals, expected)

	// Idempotent.
	c.Assert(gen.AssignUnit("riak/0"), jc.ErrorIsNil)
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

func (s *generationSuite) TestCommitAppliesConfigDeltas(c *gc.C) {
	s.setupTestingClock(c)
	gen := s.setupAssignAllUnits(c)

	app, err := s.State.Application("riak")
	c.Assert(err, jc.ErrorIsNil)

	newCfg := map[string]interface{}{"http_port": int64(9999)}
	c.Assert(app.UpdateCharmConfig(newBranchName, newCfg), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	_, err = gen.Commit(branchCommitter)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	cfg, err := app.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cfg, gc.DeepEquals, charm.Settings(newCfg))
}

// TODO (manadart 2019-03-21): Tests for abort.

func (s *generationSuite) TestBranchCharmConfigDeltas(c *gc.C) {
	gen := s.setupAssignAllUnits(c)
	c.Assert(gen.Config(), gc.HasLen, 0)

	current := state.GetPopulatedSettings(map[string]interface{}{
		"http_port":    8098,
		"handoff_port": 8099,
		"riak_version": "1.1.4-1",
	})

	// Process a modification, deletion, and addition.
	changes := charm.Settings{
		"http_port":    8100,
		"handoff_port": nil,
		"node_name":    "nodey-node",
	}
	c.Assert(gen.UpdateCharmConfig("riak", current, changes), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)
	c.Check(gen.Config(), gc.DeepEquals, map[string]settings.ItemChanges{"riak": {
		settings.MakeDeletion("handoff_port", 8099),
		settings.MakeModification("http_port", 8098, 8100),
		settings.MakeAddition("node_name", "nodey-node"),
	}})

	// Now simulate node_name being set on master in the meantime,
	// along with changes to http_port and handoff_port.
	current = state.GetPopulatedSettings(map[string]interface{}{
		"http_port":    100,
		"handoff_port": 100,
		"riak_version": "1.1.4-1",
		"node_name":    "come-lately",
	})

	// Re-set previously deleted handoff_port, update node_name.
	changes = charm.Settings{
		"handoff_port": 9000,
		"node_name":    "latest-nodey-node",
	}
	c.Assert(gen.UpdateCharmConfig("riak", current, changes), jc.ErrorIsNil)
	c.Assert(gen.Refresh(), jc.ErrorIsNil)

	// handoff_port old value is the original.
	// http_port unchanged.
	// node_name remains as an addition.
	c.Check(gen.Config(), gc.DeepEquals, map[string]settings.ItemChanges{"riak": {
		settings.MakeModification("handoff_port", 8099, 9000),
		settings.MakeModification("http_port", 8098, 8100),
		settings.MakeAddition("node_name", "latest-nodey-node"),
	}})
}

func (s *generationSuite) TestBranches(c *gc.C) {
	s.setupTestingClock(c)

	branches, err := s.State.Branches()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(branches, gc.HasLen, 0)

	_ = s.addBranch(c)
	branches, err = s.State.Branches()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(branches, gc.HasLen, 1)
	c.Check(branches[0].BranchName(), gc.Equals, newBranchName)

	const otherBranchName = "other-branch"
	c.Assert(s.Model.AddBranch(otherBranchName, newBranchCreator), jc.ErrorIsNil)
	branches, err = s.State.Branches()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(branches, gc.HasLen, 2)

	// Commit the newly added branch. Branches call should not return it.
	branch, err := s.Model.Branch(otherBranchName)
	c.Assert(err, jc.ErrorIsNil)
	_, err = branch.Commit(newBranchCreator)
	c.Assert(err, jc.ErrorIsNil)

	branches, err = s.State.Branches()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(branches, gc.HasLen, 1)
	c.Check(branches[0].BranchName(), gc.Equals, newBranchName)
}

func (s *generationSuite) setupAssignAllUnits(c *gc.C) *state.Generation {
	var cfgYAML = `
options:
  http_port: {default: 8089, description: HTTP Port, type: int}
`
	s.ch = s.AddConfigCharm(c, "riak", cfgYAML, 666)

	riak := s.AddTestingApplication(c, "riak", s.ch)
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
