// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type MachineRemovalSuite struct {
	ConnSuite
}

var _ = gc.Suite(&MachineRemovalSuite{})

func (s *MachineRemovalSuite) TestMarkingAndCompletingMachineRemoval(c *gc.C) {
	m1 := s.makeMachine(c, true)
	m2 := s.makeMachine(c, true)

	err := m1.MarkForRemoval()
	c.Assert(err, jc.ErrorIsNil)
	err = m2.MarkForRemoval()
	c.Assert(err, jc.ErrorIsNil)

	// Check machines haven't been removed.
	_, err = s.State.Machine(m1.Id())
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Machine(m2.Id())
	c.Assert(err, jc.ErrorIsNil)

	removals, err := s.State.AllMachineRemovals()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(removals, jc.SameContents, []string{m1.Id(), m2.Id()})

	err = s.State.CompleteMachineRemovals([]string{m1.Id(), "ignored"})
	c.Assert(err, jc.ErrorIsNil)
	removals2, err := s.State.AllMachineRemovals()
	c.Check(removals2, jc.SameContents, []string{m2.Id()})

	_, err = s.State.Machine(m1.Id())
	c.Assert(err, gc.ErrorMatches, "machine 0 not found")
	// But m2 is still there.
	_, err = s.State.Machine(m2.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineRemovalSuite) TestMarkForRemovalRequiresDeadness(c *gc.C) {
	m := s.makeMachine(c, false)
	err := m.MarkForRemoval()
	c.Assert(err, gc.ErrorMatches, "can't remove machine 0: machine is not dead")
}

func (s *MachineRemovalSuite) TestCompleteMachineRemovalsRequiresMark(c *gc.C) {
	m1 := s.makeMachine(c, true)
	m2 := s.makeMachine(c, true)
	err := s.State.CompleteMachineRemovals([]string{m1.Id(), m2.Id()})
	c.Assert(err, gc.ErrorMatches, "can't remove machines \\[0, 1\\]: not marked for removal")
}

func (s *MachineRemovalSuite) TestCompleteMachineRemovalsIgnoresUnmarked(c *gc.C) {
	err := s.State.CompleteMachineRemovals([]string{"A", "B"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineRemovalSuite) TestWatchMachineRemovals(c *gc.C) {
	w, wc := s.createRemovalWatcher(c, s.State)
	wc.AssertOneChange() // Initial event.

	m1 := s.makeMachine(c, true)
	m2 := s.makeMachine(c, true)

	err := m1.MarkForRemoval()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	err = m2.MarkForRemoval()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	s.State.CompleteMachineRemovals([]string{m1.Id(), m2.Id()})
	wc.AssertOneChange()

	testing.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *MachineRemovalSuite) createRemovalWatcher(c *gc.C, st *state.State) (
	state.NotifyWatcher, testing.NotifyWatcherC,
) {
	w := st.WatchMachineRemovals()
	s.AddCleanup(func(c *gc.C) { testing.AssertStop(c, w) })
	return w, testing.NewNotifyWatcherC(c, st, w)
}

func (s *MachineRemovalSuite) makeMachine(c *gc.C, deadAlready bool) *state.Machine {
	m, err := s.State.AddMachine("xenial", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	if deadAlready {
		deadenMachine(c, m)
	}
	return m
}

func deadenMachine(c *gc.C, m *state.Machine) {
	c.Assert(m.EnsureDead(), jc.ErrorIsNil)
}
