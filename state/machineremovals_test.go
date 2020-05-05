// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
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

	// Check marking a machine multiple times is ok.
	err = m1.MarkForRemoval()
	c.Assert(err, jc.ErrorIsNil)

	// Check machines haven't been removed.
	_, err = s.State.Machine(m1.Id())
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Machine(m2.Id())
	c.Assert(err, jc.ErrorIsNil)

	removals, err := s.State.AllMachineRemovals()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(removals, jc.SameContents, []string{m1.Id(), m2.Id()})

	err = s.State.CompleteMachineRemovals(m1.Id(), "100")
	c.Assert(err, jc.ErrorIsNil)
	removals2, err := s.State.AllMachineRemovals()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(removals2, jc.SameContents, []string{m2.Id()})

	_, err = s.State.Machine(m1.Id())
	c.Assert(err, gc.ErrorMatches, "machine 0 not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	// But m2 is still there.
	_, err = s.State.Machine(m2.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineRemovalSuite) TestMarkForRemovalRequiresDeadness(c *gc.C) {
	m := s.makeMachine(c, false)
	err := m.MarkForRemoval()
	c.Assert(err, gc.ErrorMatches, "cannot remove machine 0: machine is not dead")
}

func (s *MachineRemovalSuite) TestMarkForRemovalAssertsMachineStillExists(c *gc.C) {
	m := s.makeMachine(c, true)
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(m.Remove(), gc.IsNil)
	}).Check()
	err := m.MarkForRemoval()
	c.Assert(err, gc.ErrorMatches, "cannot remove machine 0: machine 0 not found")
}

func (s *MachineRemovalSuite) TestCompleteMachineRemovalsRequiresMark(c *gc.C) {
	m1 := s.makeMachine(c, true)
	m2 := s.makeMachine(c, true)
	err := s.State.CompleteMachineRemovals(m1.Id(), m2.Id())
	c.Assert(err, gc.ErrorMatches, "cannot remove machines 0, 1: not marked for removal")
}

func (s *MachineRemovalSuite) TestCompleteMachineRemovalsRequiresMarkSingular(c *gc.C) {
	m1 := s.makeMachine(c, true)
	err := s.State.CompleteMachineRemovals(m1.Id())
	c.Assert(err, gc.ErrorMatches, "cannot remove machine 0: not marked for removal")
}

func (s *MachineRemovalSuite) TestCompleteMachineRemovalsIgnoresNonexistent(c *gc.C) {
	err := s.State.CompleteMachineRemovals("0", "1")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineRemovalSuite) TestCompleteMachineRemovalsInvalid(c *gc.C) {
	err := s.State.CompleteMachineRemovals("A", "0/lxd/1", "B")
	c.Assert(err, gc.ErrorMatches, "Invalid machine ids: A, B")
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

	s.State.CompleteMachineRemovals(m1.Id(), m2.Id())
	wc.AssertOneChange()

	testing.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *MachineRemovalSuite) createRemovalWatcher(c *gc.C, st *state.State) (
	state.NotifyWatcher, testing.NotifyWatcherC,
) {
	w := st.WatchMachineRemovals()
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, w) })
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
