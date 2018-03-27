// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/replicaset"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/state"
)

type EnableHASuite struct {
	ConnSuite
}

var _ = gc.Suite(&EnableHASuite{})

func (s *EnableHASuite) TestEnableHAFailsWithBadCount(c *gc.C) {
	for _, n := range []int{-1, 2, 6} {
		changes, err := s.State.EnableHA(n, constraints.Value{}, "", nil, "")
		c.Assert(err, gc.ErrorMatches, "number of controllers must be odd and non-negative")
		c.Assert(changes.Added, gc.HasLen, 0)
	}
	_, err := s.State.EnableHA(replicaset.MaxPeers+2, constraints.Value{}, "", nil, "")
	c.Assert(err, gc.ErrorMatches, `controller count is too large \(allowed \d+\)`)
}

func (s *EnableHASuite) TestEnableHAAddsNewMachines(c *gc.C) {
	// Don't use agent presence to decide on machine availability.
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	ids := make([]string, 3)
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits, state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	ids[0] = m0.Id()

	// Add a non-controller machine just to make sure.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.assertControllerInfo(c, []string{"0"}, []string{"0"}, nil)

	cons := constraints.Value{
		Mem: newUint64(100),
	}
	changes, err := s.State.EnableHA(3, cons, "quantal", nil, m0.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)

	for i := 1; i < 3; i++ {
		m, err := s.State.Machine(fmt.Sprint(i + 1))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{
			state.JobHostUnits,
			state.JobManageModel,
		})
		gotCons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotCons, gc.DeepEquals, cons)
		c.Assert(m.WantsVote(), jc.IsTrue)
		ids[i] = m.Id()
	}
	s.assertControllerInfo(c, ids, ids, nil)
}

func (s *EnableHASuite) TestEnableHATo(c *gc.C) {
	// Don't use agent presence to decide on machine availability.
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	ids := make([]string, 3)
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits, state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	ids[0] = m0.Id()

	// Add two non-controller machines.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.assertControllerInfo(c, []string{"0"}, []string{"0"}, nil)

	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", []string{"1", "2"}, m0.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 0)
	c.Assert(changes.Converted, gc.HasLen, 2)

	for i := 1; i < 3; i++ {
		m, err := s.State.Machine(fmt.Sprint(i))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{
			state.JobHostUnits,
			state.JobManageModel,
		})
		gotCons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotCons, gc.DeepEquals, constraints.Value{})
		c.Assert(m.WantsVote(), jc.IsTrue)
		ids[i] = m.Id()
	}
	s.assertControllerInfo(c, ids, ids, nil)
}

func newUint64(i uint64) *uint64 {
	return &i
}

func (s *EnableHASuite) assertControllerInfo(c *gc.C, machineIds []string, votingMachineIds []string, placement []string) {
	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.ModelTag, gc.Equals, s.modelTag)
	c.Assert(info.MachineIds, jc.SameContents, machineIds)

	foundVoting := make([]string, 0)
	for i, id := range machineIds {
		m, err := s.State.Machine(id)
		c.Assert(err, jc.ErrorIsNil)
		if len(placement) == 0 || i >= len(placement) {
			c.Check(m.Placement(), gc.Equals, "")
		} else {
			c.Check(m.Placement(), gc.Equals, placement[i])
		}
		if m.WantsVote() {
			foundVoting = append(foundVoting, m.Id())
		}
	}
	c.Check(foundVoting, gc.DeepEquals, votingMachineIds)
}

func (s *EnableHASuite) TestEnableHASamePlacementAsNewCount(c *gc.C) {
	placement := []string{"p1", "p2", "p3"}
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", placement, "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2", "p3"})
}

func (s *EnableHASuite) TestEnableHAMorePlacementThanNewCount(c *gc.C) {
	placement := []string{"p1", "p2", "p3", "p4"}
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", placement, "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2", "p3"})
}

func (s *EnableHASuite) TestEnableHALessPlacementThanNewCount(c *gc.C) {
	placement := []string{"p1", "p2"}
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", placement, "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2"})
}

func (s *EnableHASuite) TestEnableHADemotesUnavailableMachines(c *gc.C) {
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	// If EnableHA run on machine 0, it won't be demoted.
	changes, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)
	c.Assert(changes.Maintained, gc.HasLen, 2)

	// New controller machine "3" is created; "0" still exists in MachineIds,
	// but no longer WantsVote.
	s.assertControllerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsFalse)
	c.Assert(m0.IsManager(), jc.IsTrue) // job still intact for now
	m3, err := s.State.Machine("3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m3.WantsVote(), jc.IsTrue)
	c.Assert(m3.IsManager(), jc.IsTrue)
}

func (s *EnableHASuite) TestEnableHAMockBootstrap(c *gc.C) {
	// Testing based on lp:1748275 - Juju HA fails due to demotion of Machine 0
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits, state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = m0.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)

	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil, m0.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)
	c.Assert(changes.Maintained, gc.DeepEquals, []string{"0"})
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
}

func (s *EnableHASuite) TestEnableHAPromotesAvailableMachines(c *gc.C) {
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	// If EnableHA run on machine 0, it won't be demoted.
	changes, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)
	c.Assert(changes.Demoted, gc.DeepEquals, []string{"0"})
	c.Assert(changes.Maintained, gc.HasLen, 2)

	// New controller machine "3" is created; "0" still exists in MachineIds,
	// but no longer in WantsVote.
	s.assertControllerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsFalse)

	// Mark machine 0 as having a vote, so it doesn't get removed, and make it
	// available once more.
	err = m0.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})
	changes, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 0)

	// No change; we've got as many voting machines as we need.
	s.assertControllerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)

	// Make machine 3 unavailable; machine 0 should be promoted, and two new
	// machines created.
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "3", nil
	})
	changes, err = s.State.EnableHA(5, constraints.Value{}, "quantal", nil, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)
	c.Assert(changes.Demoted, gc.DeepEquals, []string{"3"})
	s.assertControllerInfo(c, []string{"0", "1", "2", "3", "4", "5"}, []string{"0", "1", "2", "4", "5"}, nil)
	err = m0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsTrue)
}

func (s *EnableHASuite) TestEnableHARemovesUnavailableMachines(c *gc.C) {
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)

	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	// If EnableHA run on machine 0, it won't be demoted.
	changes, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)
	s.assertControllerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)
	// machine 0 does not have a vote, so another call to EnableHA
	// will remove machine 0's JobEnvironManager job.
	changes, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "1")
	c.Assert(changes.Removed, gc.HasLen, 1)
	c.Assert(changes.Maintained, gc.HasLen, 3)
	c.Assert(err, jc.ErrorIsNil)
	s.assertControllerInfo(c, []string{"1", "2", "3"}, []string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.IsManager(), jc.IsFalse)
}

func (s *EnableHASuite) TestEnableHAMaintainsVoteList(c *gc.C) {
	changes, err := s.State.EnableHA(5, constraints.Value{}, "quantal", nil, "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 5)

	s.assertControllerInfo(c,
		[]string{"0", "1", "2", "3", "4"},
		[]string{"0", "1", "2", "3", "4"}, nil)
	// Mark machine-0 as dead, so we'll want to create another one again
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	// If EnableHA run on machine 0, it won't be demoted.
	changes, err = s.State.EnableHA(0, constraints.Value{}, "quantal", nil, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)

	// New controller machine "5" is created; "0" still exists in MachineIds,
	// but no longer in WantsVote.
	s.assertControllerInfo(c,
		[]string{"0", "1", "2", "3", "4", "5"},
		[]string{"1", "2", "3", "4", "5"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsFalse)
	c.Assert(m0.IsManager(), jc.IsTrue) // job still intact for now
	m3, err := s.State.Machine("5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m3.WantsVote(), jc.IsTrue)
	c.Assert(m3.IsManager(), jc.IsTrue)
}

func (s *EnableHASuite) TestEnableHADefaultsTo3(c *gc.C) {
	changes, err := s.State.EnableHA(0, constraints.Value{}, "quantal", nil, "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	// Mark machine-0 as dead, so we'll want to create it again
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	// If EnableHA run on machine 0, it won't be demoted.
	changes, err = s.State.EnableHA(0, constraints.Value{}, "quantal", nil, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)

	// New controller machine "3" is created; "0" still exists in MachineIds,
	// but no longer in WantsVote.
	s.assertControllerInfo(c,
		[]string{"0", "1", "2", "3"},
		[]string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsFalse)
	c.Assert(m0.IsManager(), jc.IsTrue) // job still intact for now
	m3, err := s.State.Machine("3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m3.WantsVote(), jc.IsTrue)
	c.Assert(m3.IsManager(), jc.IsTrue)
}

func (s *EnableHASuite) TestEnableHAConcurrentSame(c *gc.C) {
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "")
		c.Assert(err, jc.ErrorIsNil)
		// The outer EnableHA call will allocate IDs 0..2,
		// and the inner one 3..5.
		c.Assert(changes.Added, gc.HasLen, 3)
		expected := []string{"3", "4", "5"}
		s.assertControllerInfo(c, expected, expected, nil)
	}).Check()

	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.DeepEquals, []string{"0", "1", "2"})
	s.assertControllerInfo(c, []string{"3", "4", "5"}, []string{"3", "4", "5"}, nil)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *EnableHASuite) TestEnableHAConcurrentLess(c *gc.C) {
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(changes.Added, gc.HasLen, 3)
		// The outer EnableHA call will initially allocate IDs 0..4,
		// and the inner one 5..7.
		expected := []string{"5", "6", "7"}
		s.assertControllerInfo(c, expected, expected, nil)
	}).Check()

	// This call to EnableHA will initially attempt to allocate
	// machines 0..4, and fail due to the concurrent change. It will then
	// allocate machines 8..9 to make up the difference from the concurrent
	// EnableHA call.
	changes, err := s.State.EnableHA(5, constraints.Value{}, "quantal", nil, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)
	expected := []string{"5", "6", "7", "8", "9"}
	s.assertControllerInfo(c, expected, expected, nil)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *EnableHASuite) TestEnableHAConcurrentMore(c *gc.C) {
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnableHA(5, constraints.Value{}, "quantal", nil, "")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(changes.Added, gc.HasLen, 5)
		// The outer EnableHA call will allocate IDs 0..2,
		// and the inner one 3..7.
		expected := []string{"3", "4", "5", "6", "7"}
		s.assertControllerInfo(c, expected, expected, nil)
	}).Check()

	// This call to EnableHA will initially attempt to allocate
	// machines 0..2, and fail due to the concurrent change. It will then
	// find that the number of voting machines in state is greater than
	// what we're attempting to ensure, and fail.
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "")
	c.Assert(err, gc.ErrorMatches, "failed to create new controller machines: cannot reduce controller count")
	c.Assert(changes.Added, gc.HasLen, 0)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *EnableHASuite) TestForceDestroyFromHA(c *gc.C) {
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	m0, err := s.State.AddMachine("quantal", state.JobHostUnits, state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = m0.ForceDestroy()
	c.Assert(err, gc.ErrorMatches, "machine 0 is the only controller machine")
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	err = m0.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.Refresh(), jc.ErrorIsNil)
	c.Check(m0.Life(), gc.Equals, state.Dying)
	c.Check(m0.WantsVote(), jc.IsFalse)
}

func (s *EnableHASuite) TestForceDestroyRaceLastController(c *gc.C) {
	c.Skip("not able to race yet")
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits, state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil, "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)

	// TODO(jam) 2018-03-25: We don't directly mutate the DB, we need to expose a function that can actually cause a
	// race, so that we know we have the right assertions in the ForceDestroy txns.
	defer state.SetBeforeHooks(c, s.State, func() {
		// Somehow remove a different controller machine while we're waiting for machine 0 to be removed
	}).Check()
	err = m0.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.Refresh(), jc.ErrorIsNil)
	c.Check(m0.Life(), gc.Equals, state.Dying)
	c.Check(m0.WantsVote(), jc.IsFalse)
}
