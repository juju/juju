// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type ModelMigrationSuite struct {
	ConnSuite
	State2  *state.State
	clock   *coretesting.Clock
	stdSpec state.ModelMigrationSpec
}

var _ = gc.Suite(new(ModelMigrationSuite))

func (s *ModelMigrationSuite) SetUpTest(c *gc.C) {
	s.clock = coretesting.NewClock(time.Now().Truncate(time.Second))
	s.PatchValue(&state.GetClock, func() clock.Clock {
		return s.clock
	})

	s.ConnSuite.SetUpTest(c)

	// Create a hosted model to migrate.
	s.State2 = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { s.State2.Close() })

	targetControllerTag := names.NewModelTag(utils.MustNewUUID().String())

	// Plausible migration arguments to test with.
	s.stdSpec = state.ModelMigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: targetControllerTag,
			Addrs:         []string{"1.2.3.4:5555", "4.3.2.1:6666"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("user"),
			Password:      "password",
		},
	}
}

func (s *ModelMigrationSuite) TestCreate(c *gc.C) {
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mig.ModelUUID(), gc.Equals, s.State2.ModelUUID())
	checkIdAndAttempt(c, mig, 0)

	c.Check(mig.StartTime(), gc.Equals, s.clock.Now())

	c.Check(mig.SuccessTime().IsZero(), jc.IsTrue)
	c.Check(mig.EndTime().IsZero(), jc.IsTrue)
	c.Check(mig.StatusMessage(), gc.Equals, "")
	c.Check(mig.InitiatedBy(), gc.Equals, "admin")

	info, err := mig.TargetInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*info, jc.DeepEquals, s.stdSpec.TargetInfo)

	assertPhase(c, mig, migration.QUIESCE)
	c.Check(mig.PhaseChangedTime(), gc.Equals, mig.StartTime())

	assertMigrationActive(c, s.State2)
}

func (s *ModelMigrationSuite) TestIdSequencesAreIndependent(c *gc.C) {
	st2 := s.State2
	st3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { st3.Close() })

	mig2, err := st2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	checkIdAndAttempt(c, mig2, 0)

	mig3, err := st3.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	checkIdAndAttempt(c, mig3, 0)
}

func (s *ModelMigrationSuite) TestIdSequencesIncrement(c *gc.C) {
	for attempt := 0; attempt < 3; attempt++ {
		mig, err := s.State2.CreateModelMigration(s.stdSpec)
		c.Assert(err, jc.ErrorIsNil)
		checkIdAndAttempt(c, mig, attempt)
		c.Check(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	}
}

func (s *ModelMigrationSuite) TestIdSequencesIncrementOnlyWhenNecessary(c *gc.C) {
	// Ensure that sequence numbers aren't "used up" unnecessarily
	// when the create txn is going to fail.

	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	checkIdAndAttempt(c, mig, 0)

	// This attempt will fail because a migration is already in
	// progress.
	_, err = s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, gc.ErrorMatches, ".+already in progress")

	// Now abort the migration and create another. The Id sequence
	// should have only incremented by 1.
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)

	mig, err = s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	checkIdAndAttempt(c, mig, 1)
}

func (s *ModelMigrationSuite) TestSpecValidation(c *gc.C) {
	tests := []struct {
		label        string
		tweakSpec    func(*state.ModelMigrationSpec)
		errorPattern string
	}{{
		"invalid InitiatedBy",
		func(spec *state.ModelMigrationSpec) {
			spec.InitiatedBy = names.UserTag{}
		},
		"InitiatedBy not valid",
	}, {
		"TargetInfo is validated",
		func(spec *state.ModelMigrationSpec) {
			spec.TargetInfo.Password = ""
		},
		"empty Password not valid",
	}}
	for _, test := range tests {
		c.Logf("---- %s -----------", test.label)

		// Set up spec.
		spec := s.stdSpec
		test.tweakSpec(&spec)

		// Check Validate directly.
		err := spec.Validate()
		c.Check(errors.IsNotValid(err), jc.IsTrue)
		c.Check(err, gc.ErrorMatches, test.errorPattern)

		// Ensure that CreateModelMigration rejects the bad spec too.
		mig, err := s.State2.CreateModelMigration(spec)
		c.Check(mig, gc.IsNil)
		c.Check(errors.IsNotValid(err), jc.IsTrue)
		c.Check(err, gc.ErrorMatches, test.errorPattern)
	}
}

func (s *ModelMigrationSuite) TestCreateWithControllerModel(c *gc.C) {
	// This is the State for the controller
	mig, err := s.State.CreateModelMigration(s.stdSpec)
	c.Check(mig, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "controllers can't be migrated")
}

func (s *ModelMigrationSuite) TestCreateMigrationInProgress(c *gc.C) {
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(mig, gc.Not(gc.IsNil))
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Check(mig2, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "failed to create migration: already in progress")
}

func (s *ModelMigrationSuite) TestCreateMigrationRace(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State2, func() {
		mig, err := s.State2.CreateModelMigration(s.stdSpec)
		c.Assert(mig, gc.Not(gc.IsNil))
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Check(mig, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "failed to create migration: already in progress")
}

func (s *ModelMigrationSuite) TestCreateMigrationWhenModelNotAlive(c *gc.C) {
	// Set the hosted model to Dying.
	model, err := s.State2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(), jc.ErrorIsNil)

	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Check(mig, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "failed to create migration: model is not alive")
}

func (s *ModelMigrationSuite) TestMigrationToSameController(c *gc.C) {
	spec := s.stdSpec
	spec.TargetInfo.ControllerTag = s.State.ModelTag()

	mig, err := s.State2.CreateModelMigration(spec)
	c.Check(mig, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "model already attached to target controller")
}

func (s *ModelMigrationSuite) TestGet(c *gc.C) {
	mig1, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := s.State2.GetModelMigration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mig1.Id(), gc.Equals, mig2.Id())
}

func (s *ModelMigrationSuite) TestGetNotExist(c *gc.C) {
	mig, err := s.State.GetModelMigration()
	c.Check(mig, gc.IsNil)
	c.Check(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ModelMigrationSuite) TestGetsLatestAttempt(c *gc.C) {
	modelUUID := s.State2.ModelUUID()

	for i := 0; i < 10; i++ {
		c.Logf("loop %d", i)
		_, err := s.State2.CreateModelMigration(s.stdSpec)
		c.Assert(err, jc.ErrorIsNil)

		mig, err := s.State2.GetModelMigration()
		c.Check(mig.Id(), gc.Equals, fmt.Sprintf("%s:%d", modelUUID, i))

		c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	}
}

func (s *ModelMigrationSuite) TestRefresh(c *gc.C) {
	mig1, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := s.State2.GetModelMigration()
	c.Assert(err, jc.ErrorIsNil)

	err = mig1.SetPhase(migration.READONLY)
	c.Assert(err, jc.ErrorIsNil)

	assertPhase(c, mig2, migration.QUIESCE)
	err = mig2.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertPhase(c, mig2, migration.READONLY)
}

func (s *ModelMigrationSuite) TestSuccessfulPhaseTransitions(c *gc.C) {
	st := s.State2

	mig, err := st.CreateModelMigration(s.stdSpec)
	c.Assert(mig, gc.Not(gc.IsNil))
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := st.GetModelMigration()
	c.Assert(err, jc.ErrorIsNil)

	phases := []migration.Phase{
		migration.READONLY,
		migration.PRECHECK,
		migration.IMPORT,
		migration.VALIDATION,
		migration.SUCCESS,
		migration.LOGTRANSFER,
		migration.REAP,
		migration.DONE,
	}

	var successTime time.Time
	for _, phase := range phases[:len(phases)-1] {
		err := mig.SetPhase(phase)
		c.Assert(err, jc.ErrorIsNil)

		assertPhase(c, mig, phase)
		c.Assert(mig.PhaseChangedTime(), gc.Equals, s.clock.Now())

		// Check success timestamp is set only when SUCCESS is
		// reached.
		if phase < migration.SUCCESS {
			c.Assert(mig.SuccessTime().IsZero(), jc.IsTrue)
		} else {
			if phase == migration.SUCCESS {
				successTime = s.clock.Now()
			}
			c.Assert(mig.SuccessTime(), gc.Equals, successTime)
		}

		// Check still marked as active.
		assertMigrationActive(c, s.State2)
		c.Assert(mig.EndTime().IsZero(), jc.IsTrue)

		// Ensure change was peristed.
		c.Assert(mig2.Refresh(), jc.ErrorIsNil)
		assertPhase(c, mig2, phase)

		s.clock.Advance(time.Millisecond)
	}

	// Now move to the final phase (DONE) and ensure fields are set as
	// expected.
	err = mig.SetPhase(migration.DONE)
	c.Assert(err, jc.ErrorIsNil)
	assertPhase(c, mig, migration.DONE)
	s.assertMigrationCleanedUp(c, mig)
}

func (s *ModelMigrationSuite) TestABORTCleanup(c *gc.C) {
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.clock.Advance(time.Millisecond)
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)

	s.assertMigrationCleanedUp(c, mig)
}

func (s *ModelMigrationSuite) TestREAPFAILEDCleanup(c *gc.C) {
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	// Advance the migration to REAPFAILED.
	phases := []migration.Phase{
		migration.READONLY,
		migration.PRECHECK,
		migration.IMPORT,
		migration.VALIDATION,
		migration.SUCCESS,
		migration.LOGTRANSFER,
		migration.REAP,
		migration.REAPFAILED,
	}
	for _, phase := range phases {
		s.clock.Advance(time.Millisecond)
		c.Assert(mig.SetPhase(phase), jc.ErrorIsNil)
	}

	s.assertMigrationCleanedUp(c, mig)
}

func (s *ModelMigrationSuite) assertMigrationCleanedUp(c *gc.C, mig state.ModelMigration) {
	c.Assert(mig.PhaseChangedTime(), gc.Equals, s.clock.Now())
	c.Assert(mig.EndTime(), gc.Equals, s.clock.Now())
	assertMigrationNotActive(c, s.State2)
}

func (s *ModelMigrationSuite) TestIllegalPhaseTransition(c *gc.C) {
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	err = mig.SetPhase(migration.SUCCESS)
	c.Check(err, gc.ErrorMatches, "illegal phase change: QUIESCE -> SUCCESS")
}

func (s *ModelMigrationSuite) TestPhaseChangeRace(c *gc.C) {
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(mig, gc.Not(gc.IsNil))

	defer state.SetBeforeHooks(c, s.State2, func() {
		mig, err := s.State2.GetModelMigration()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mig.SetPhase(migration.READONLY), jc.ErrorIsNil)
	}).Check()

	err = mig.SetPhase(migration.READONLY)
	c.Assert(err, gc.ErrorMatches, "phase already changed")
	assertPhase(c, mig, migration.QUIESCE)

	// After a refresh it the phase change should be ok.
	c.Assert(mig.Refresh(), jc.ErrorIsNil)
	err = mig.SetPhase(migration.READONLY)
	c.Assert(err, jc.ErrorIsNil)
	assertPhase(c, mig, migration.READONLY)
}

func (s *ModelMigrationSuite) TestStatusMessage(c *gc.C) {
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(mig, gc.Not(gc.IsNil))

	mig2, err := s.State2.GetModelMigration()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mig.StatusMessage(), gc.Equals, "")
	c.Check(mig2.StatusMessage(), gc.Equals, "")

	err = mig.SetStatusMessage("foo bar")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mig.StatusMessage(), gc.Equals, "foo bar")

	c.Assert(mig2.Refresh(), jc.ErrorIsNil)
	c.Check(mig2.StatusMessage(), gc.Equals, "foo bar")
}

func (s *ModelMigrationSuite) TestWatchForModelMigration(c *gc.C) {
	// Start watching for migration.
	w, wc := s.createWatcher(c, s.State2)
	wc.AssertNoChange()

	// Create the migration - should be reported.
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Ending the migration should not be reported.
	c.Check(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *ModelMigrationSuite) TestWatchForModelMigrationInProgress(c *gc.C) {
	// Create a migration.
	_, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	// Start watching for a migration - the in progress one should be reported.
	_, wc := s.createWatcher(c, s.State2)
	wc.AssertOneChange()
}

func (s *ModelMigrationSuite) TestWatchForModelMigrationMultiModel(c *gc.C) {
	_, wc2 := s.createWatcher(c, s.State2)
	wc2.AssertNoChange()

	// Create another hosted model to migrate and watch for
	// migrations.
	State3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { State3.Close() })
	_, wc3 := s.createWatcher(c, State3)
	wc3.AssertNoChange()

	// Create a migration for 2.
	_, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc2.AssertOneChange()
	wc3.AssertNoChange()

	// Create a migration for 3.
	_, err = State3.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc2.AssertNoChange()
	wc3.AssertOneChange()
}

func (s *ModelMigrationSuite) createWatcher(c *gc.C, st *state.State) (
	state.NotifyWatcher, statetesting.NotifyWatcherC,
) {
	w, err := st.WatchForModelMigration()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { statetesting.AssertStop(c, w) })
	return w, statetesting.NewNotifyWatcherC(c, st, w)
}

func (s *ModelMigrationSuite) TestWatchMigrationStatus(c *gc.C) {
	w, wc := s.createStatusWatcher(c, s.State2)
	wc.AssertNoChange()

	// Create a migration.
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// End it.
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	wc.AssertOneChange()

	// Start another.
	mig2, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Change phase.
	c.Assert(mig2.SetPhase(migration.READONLY), jc.ErrorIsNil)
	wc.AssertOneChange()

	// End it.
	c.Assert(mig2.SetPhase(migration.ABORT), jc.ErrorIsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *ModelMigrationSuite) TestWatchMigrationStatusPreexisting(c *gc.C) {
	// Create an aborted migration.
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)

	_, wc := s.createStatusWatcher(c, s.State2)
	wc.AssertOneChange()
}

func (s *ModelMigrationSuite) TestWatchMigrationStatusMultiModel(c *gc.C) {
	_, wc2 := s.createStatusWatcher(c, s.State2)
	wc2.AssertNoChange()

	// Create another hosted model to migrate and watch for
	// migrations.
	State3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { State3.Close() })
	_, wc3 := s.createStatusWatcher(c, State3)
	wc3.AssertNoChange()

	// Create a migration for 2.
	mig, err := s.State2.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc2.AssertOneChange()
	wc3.AssertNoChange()

	// Create a migration for 3.
	_, err = State3.CreateModelMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc2.AssertNoChange()
	wc3.AssertOneChange()

	// Update the migration for 2.
	err = mig.SetPhase(migration.ABORT)
	c.Assert(err, jc.ErrorIsNil)
	wc2.AssertOneChange()
	wc3.AssertNoChange()
}

func (s *ModelMigrationSuite) createStatusWatcher(c *gc.C, st *state.State) (
	state.NotifyWatcher, statetesting.NotifyWatcherC,
) {
	w, err := st.WatchMigrationStatus()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { statetesting.AssertStop(c, w) })
	return w, statetesting.NewNotifyWatcherC(c, st, w)
}

func assertPhase(c *gc.C, mig state.ModelMigration, phase migration.Phase) {
	actualPhase, err := mig.Phase()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actualPhase, gc.Equals, phase)
}

func assertMigrationActive(c *gc.C, st *state.State) {
	c.Check(isMigrationActive(c, st), jc.IsTrue)
}

func assertMigrationNotActive(c *gc.C, st *state.State) {
	c.Check(isMigrationActive(c, st), jc.IsFalse)
}

func isMigrationActive(c *gc.C, st *state.State) bool {
	isActive, err := st.IsModelMigrationActive()
	c.Assert(err, jc.ErrorIsNil)
	return isActive
}

func checkIdAndAttempt(c *gc.C, mig state.ModelMigration, expected int) {
	c.Check(mig.Id(), gc.Equals, fmt.Sprintf("%s:%d", mig.ModelUUID(), expected))
	attempt, err := mig.Attempt()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(attempt, gc.Equals, expected)
}
