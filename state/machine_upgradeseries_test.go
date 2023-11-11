// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
)

func (s *MachineSuite) TestCreateUpgradeSeriesLock(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	locked, err := mach.IsLockedForSeriesUpgrade()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(locked, jc.IsFalse)

	unitIds := []string{"multi-series/0", "multi-series-subordinate/0"}
	err = mach.CreateUpgradeSeriesLock(unitIds, state.UbuntuBase("16.04"))
	c.Assert(err, jc.ErrorIsNil)

	locked, err = mach.IsLockedForSeriesUpgrade()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(locked, jc.IsTrue)

	units, err := mach.UpgradeSeriesUnitStatuses()
	c.Assert(err, jc.ErrorIsNil)

	lockedUnitsIds := make([]string, len(units))
	i := 0
	for id := range units {
		lockedUnitsIds[i] = id
		i++
	}
	c.Assert(lockedUnitsIds, jc.SameContents, unitIds)
}

func (s *MachineSuite) TestIsParentLockedForSeriesUpgrade(c *gc.C) {
	parent, err := s.State.AddMachine(state.UbuntuBase("16.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	template := state.MachineTemplate{
		Base: state.UbuntuBase("16.04"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	child, err := s.State.AddMachineInsideMachine(template, parent.Id(), "lxd")
	c.Assert(err, jc.ErrorIsNil)

	err = parent.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("18.04"))
	c.Assert(err, jc.ErrorIsNil)

	locked, err := child.IsParentLockedForSeriesUpgrade()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(locked, jc.IsTrue)
}

func (s *MachineSuite) TestCreateUpgradeSeriesLockErrorsIfLockExists(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.CreateUpgradeSeriesLock([]string{"multi-series/0", "multi-series-subordinate/0"}, state.UbuntuBase("16.04"))
	c.Assert(err, jc.ErrorIsNil)
	err = mach.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("16.04"))
	c.Assert(err, gc.ErrorMatches, "upgrade series lock for machine \".*\" already exists")
}

func (s *MachineSuite) TestDoesNotCreateUpgradeSeriesLockOnDyingMachine(c *gc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("12.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = mach.Destroy(state.NewObjectStore(c, s.State))
	c.Assert(err, jc.ErrorIsNil)

	err = mach.CreateUpgradeSeriesLock([]string{""}, state.UbuntuBase("16.04"))
	c.Assert(err, gc.ErrorMatches, "machine not found or not alive")
}

func (s *MachineSuite) TestDoesNotCreateUpgradeSeriesLockOnSameSeries(c *gc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("16.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = mach.CreateUpgradeSeriesLock([]string{""}, state.UbuntuBase("16.04"))
	c.Assert(err, gc.ErrorMatches, "machine .* already at base ubuntu@16.04/stable")
}

func (s *MachineSuite) TestDoesNotCreateUpgradeSeriesLockUnitsChanged(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)

	err := mach.CreateUpgradeSeriesLock([]string{"wordpress/0"}, state.UbuntuBase("16.04"))
	c.Assert(err, gc.ErrorMatches, "Units have changed, please retry (.*)")
}

func (s *MachineSuite) TestUpgradeSeriesTarget(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)

	units := []string{"multi-series/0", "multi-series-subordinate/0"}
	err := mach.CreateUpgradeSeriesLock(units, state.UbuntuBase("18.04"))
	c.Assert(err, jc.ErrorIsNil)

	target, err := mach.UpgradeSeriesTarget()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(target, gc.Equals, "ubuntu@18.04/stable")
}

func (s *MachineSuite) TestRemoveUpgradeSeriesLockUnlocksMachine(c *gc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("12.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	AssertMachineIsNOTLockedForPrepare(c, mach)

	err = mach.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("16.04"))
	c.Assert(err, jc.ErrorIsNil)
	AssertMachineLockedForPrepare(c, mach)

	err = mach.RemoveUpgradeSeriesLock()
	c.Assert(err, jc.ErrorIsNil)
	AssertMachineIsNOTLockedForPrepare(c, mach)
}

func (s *MachineSuite) TestRemoveUpgradeSeriesLockIsNoOpIfMachineIsNotLocked(c *gc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("12.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	AssertMachineIsNOTLockedForPrepare(c, mach)

	err = mach.RemoveUpgradeSeriesLock()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestForceMarksSeriesLockUnlocksMachineForCleanup(c *gc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("12.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	AssertMachineIsNOTLockedForPrepare(c, mach)

	err = mach.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("16.04"))
	c.Assert(err, jc.ErrorIsNil)
	AssertMachineLockedForPrepare(c, mach)

	err = mach.ForceDestroy(dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// After a forced destroy an upgrade series lock on a machine should be
	// marked for cleanup and therefore should be cleaned up if anything
	// should trigger a state cleanup.
	s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State))

	// The machine, since it was destroyed, its lock should have been
	// cleaned up. Checking to see if the machine is not locked, that is,
	// checking to see if no lock exist for the machine should yield a
	// positive result.
	AssertMachineIsNOTLockedForPrepare(c, mach)
}

func (s *MachineSuite) TestCompleteSeriesUpgradeShouldFailWhenMachineIsNotComplete(c *gc.C) {
	err := s.machine.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("22.04"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.CompleteUpgradeSeries()
	assertMachineIsNotReadyForCompletion(c, err)
}

func (s *MachineSuite) TestCompleteSeriesUpgradeShouldSucceedWhenMachinePrepareIsComplete(c *gc.C) {
	unit0 := s.addMachineUnit(c, s.machine)
	err := s.machine.CreateUpgradeSeriesLock([]string{unit0.Name()}, state.UbuntuBase("22.04"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, "")
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.CompleteUpgradeSeries()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestCompleteSeriesUpgradeShouldSetCompleteStatusOfMachine(c *gc.C) {
	err := s.machine.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("22.04"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, "")
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.CompleteUpgradeSeries()
	c.Assert(err, jc.ErrorIsNil)

	sts, err := s.machine.UpgradeSeriesStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sts, gc.Equals, model.UpgradeSeriesCompleteStarted)
}

func (s *MachineSuite) TestCompleteSeriesUpgradeShouldFailIfAlreadyInCompleteState(c *gc.C) {
	unit0 := s.addMachineUnit(c, s.machine)
	err := s.machine.CreateUpgradeSeriesLock([]string{unit0.Name()}, state.UbuntuBase("22.04"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, "")
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.CompleteUpgradeSeries()
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.CompleteUpgradeSeries()
	assertMachineIsNotReadyForCompletion(c, err)
}

func (s *MachineSuite) TestHasUpgradeSeriesLocks(c *gc.C) {
	// Ensure we don't have any locks before testing.
	lock, err := s.State.HasUpgradeSeriesLocks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lock, jc.IsFalse)

	unit0 := s.addMachineUnit(c, s.machine)
	err = s.machine.CreateUpgradeSeriesLock([]string{unit0.Name()}, state.UbuntuBase("22.04"))
	c.Assert(err, jc.ErrorIsNil)

	lock, err = s.State.HasUpgradeSeriesLocks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lock, jc.IsTrue)
}

func assertMachineIsNotReadyForCompletion(c *gc.C, err error) {
	c.Assert(err, gc.ErrorMatches, "machine \"[0-9].*\" can not complete, it is either not prepared or already completed")
}

func (s *MachineSuite) TestUnitsHaveChangedFalse(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	changed, err := state.UnitsHaveChanged(mach, []string{"multi-series/0", "multi-series-subordinate/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changed, jc.IsFalse)
}

func (s *MachineSuite) TestUnitsHaveChangedTrue(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	changed, err := state.UnitsHaveChanged(mach, []string{"multi-series-subordinate/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changed, jc.IsTrue)
}

func (s *MachineSuite) TestUnitsHaveChangedFalseNoUnits(c *gc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("16.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	changed, err := state.UnitsHaveChanged(mach, []string{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changed, jc.IsFalse)
}

func (s *MachineSuite) TestGetUpgradeSeriesMessagesMissingLockMeansFinished(c *gc.C) {
	_, finished, err := s.machine.GetUpgradeSeriesMessages()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(finished, jc.IsTrue)
}

func (s *MachineSuite) TestIsLockedIndicatesUnlockedWhenNoLockDocIsFound(c *gc.C) {
	locked, err := s.machine.IsLockedForSeriesUpgrade()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(locked, jc.IsFalse)
}

func AssertMachineLockedForPrepare(c *gc.C, mach *state.Machine) {
	locked, err := mach.IsLockedForSeriesUpgrade()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(locked, jc.IsTrue)
}

func AssertMachineIsNOTLockedForPrepare(c *gc.C, mach *state.Machine) {
	locked, err := mach.IsLockedForSeriesUpgrade()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(locked, jc.IsFalse)
}
