// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type MigrationSuite struct {
	ConnSuite
	State2  *state.State
	stdSpec state.MigrationSpec
}

var _ = gc.Suite(new(MigrationSuite))

func (s *MigrationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	// Create a hosted model to migrate.
	s.State2 = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { s.State2.Close() })

	targetControllerTag := names.NewControllerTag(utils.MustNewUUID().String())

	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)

	// Plausible migration arguments to test with.
	s.stdSpec = state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag:   targetControllerTag,
			ControllerAlias: "target-controller",
			Addrs:           []string{"1.2.3.4:5555", "4.3.2.1:6666"},
			CACert:          "cert",
			AuthTag:         names.NewUserTag("user"),
			Password:        "password",
			Macaroons:       []macaroon.Slice{{mac}},
			Token:           "token",
		},
	}
	// Before we get into the tests, ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.State2.ModelUUID())
}

func (s *MigrationSuite) TestCreate(c *gc.C) {
	model, err := s.State2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeNone)

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mig.ModelUUID(), gc.Equals, s.State2.ModelUUID())
	checkIdAndAttempt(c, mig, 0)

	c.Check(mig.StartTime().IsZero(), jc.IsFalse)
	c.Check(mig.StartTime().Before(s.Clock.Now()), jc.IsTrue)
	c.Check(mig.SuccessTime().IsZero(), jc.IsTrue)
	c.Check(mig.EndTime().IsZero(), jc.IsTrue)
	c.Check(mig.StatusMessage(), gc.Equals, "starting")
	c.Check(mig.InitiatedBy(), gc.Equals, "admin")

	info, err := mig.TargetInfo()
	c.Assert(err, jc.ErrorIsNil)
	// Extract macaroons so we can compare them separately
	// (as they can't be compared using DeepEquals due to 'UnmarshaledAs')
	infoMacs := info.Macaroons
	info.Macaroons = nil
	assertMacaroonsEqual(c, infoMacs, s.stdSpec.TargetInfo.Macaroons)
	s.stdSpec.TargetInfo.Macaroons = nil
	c.Check(*info, jc.DeepEquals, s.stdSpec.TargetInfo)
	c.Check(info.ControllerAlias, gc.Equals, s.stdSpec.TargetInfo.ControllerAlias)

	assertPhase(c, mig, migration.QUIESCE)
	c.Check(mig.PhaseChangedTime(), gc.Equals, mig.StartTime())

	assertMigrationActive(c, s.State2)

	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Check(model.MigrationMode(), gc.Equals, state.MigrationModeExporting)
}

func (s *MigrationSuite) TestIsMigrationActive(c *gc.C) {
	check := func(expected bool) {
		isActive, err := s.State2.IsMigrationActive()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(isActive, gc.Equals, expected)

		isActive2, err := state.IsMigrationActive(s.State, s.State2.ModelUUID())
		c.Assert(err, jc.ErrorIsNil)
		c.Check(isActive2, gc.Equals, expected)
	}

	check(false)

	_, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	check(true)
}

func (s *MigrationSuite) TestIdSequencesAreIndependent(c *gc.C) {
	st2 := s.State2
	st3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { st3.Close() })

	mig2, err := st2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	checkIdAndAttempt(c, mig2, 0)

	mig3, err := st3.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	checkIdAndAttempt(c, mig3, 0)
}

func (s *MigrationSuite) TestIdSequencesIncrement(c *gc.C) {
	for attempt := 0; attempt < 3; attempt++ {
		mig, err := s.State2.CreateMigration(s.stdSpec)
		c.Assert(err, jc.ErrorIsNil)
		checkIdAndAttempt(c, mig, attempt)
		c.Check(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
		c.Check(mig.SetPhase(migration.ABORTDONE), jc.ErrorIsNil)
	}
}

func (s *MigrationSuite) TestIdSequencesIncrementOnlyWhenNecessary(c *gc.C) {
	// Ensure that sequence numbers aren't "used up" unnecessarily
	// when the create txn is going to fail.

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	checkIdAndAttempt(c, mig, 0)

	// This attempt will fail because a migration is already in
	// progress.
	_, err = s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, gc.ErrorMatches, ".+already in progress")

	// Now abort the migration and create another. The Id sequence
	// should have only incremented by 1.
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	c.Assert(mig.SetPhase(migration.ABORTDONE), jc.ErrorIsNil)

	mig, err = s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	checkIdAndAttempt(c, mig, 1)
}

func (s *MigrationSuite) TestSpecValidation(c *gc.C) {
	tests := []struct {
		label        string
		tweakSpec    func(*state.MigrationSpec)
		errorPattern string
	}{{
		"invalid InitiatedBy",
		func(spec *state.MigrationSpec) {
			spec.InitiatedBy = names.UserTag{}
		},
		"InitiatedBy not valid",
	}, {
		"TargetInfo is validated",
		func(spec *state.MigrationSpec) {
			spec.TargetInfo.Addrs = nil
		},
		"empty Addrs not valid",
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

		// Ensure that CreateMigration rejects the bad spec too.
		mig, err := s.State2.CreateMigration(spec)
		c.Check(mig, gc.IsNil)
		c.Check(errors.IsNotValid(err), jc.IsTrue)
		c.Check(err, gc.ErrorMatches, test.errorPattern)
	}
}

func (s *MigrationSuite) TestCreateWithControllerModel(c *gc.C) {
	// This is the State for the controller
	mig, err := s.State.CreateMigration(s.stdSpec)
	c.Check(mig, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "controllers can't be migrated")
}

func (s *MigrationSuite) TestCreateMigrationInProgress(c *gc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(mig, gc.Not(gc.IsNil))
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := s.State2.CreateMigration(s.stdSpec)
	c.Check(mig2, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "failed to create migration: already in progress")
}

func (s *MigrationSuite) TestCreateMigrationRace(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State2, func() {
		mig, err := s.State2.CreateMigration(s.stdSpec)
		c.Assert(mig, gc.Not(gc.IsNil))
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Check(mig, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "failed to create migration: already in progress")
}

func (s *MigrationSuite) TestCreateMigrationWhenModelNotAlive(c *gc.C) {
	// Set the hosted model to Dying.
	model, err := s.State2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Check(mig, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "failed to create migration: model is not alive")
}

func (s *MigrationSuite) TestMigrationToSameController(c *gc.C) {
	spec := s.stdSpec
	spec.TargetInfo.ControllerTag = s.State.ControllerTag()

	mig, err := s.State2.CreateMigration(spec)
	c.Check(mig, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "model already attached to target controller")
}

func (s *MigrationSuite) TestLatestMigration(c *gc.C) {
	mig1, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := s.State2.LatestMigration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mig1.Id(), gc.Equals, mig2.Id())
}

func (s *MigrationSuite) TestLatestMigrationNotExist(c *gc.C) {
	mig, err := s.State.LatestMigration()
	c.Check(mig, gc.IsNil)
	c.Check(errors.IsNotFound(err), jc.IsTrue)
}

func (s *MigrationSuite) TestGetsLatestAttempt(c *gc.C) {
	modelUUID := s.State2.ModelUUID()

	for i := 0; i < 10; i++ {
		c.Logf("loop %d", i)
		_, err := s.State2.CreateMigration(s.stdSpec)
		c.Assert(err, jc.ErrorIsNil)

		mig, err := s.State2.LatestMigration()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(mig.Id(), gc.Equals, fmt.Sprintf("%s:%d", modelUUID, i))

		c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
		c.Assert(mig.SetPhase(migration.ABORTDONE), jc.ErrorIsNil)
	}
}

func (s *MigrationSuite) TestLatestMigrationWithPrevious(c *gc.C) {
	// Check the scenario of a model having been migrated away and
	// then migrated back several times. The previous migrations
	// shouldn't be reported by LatestMigration.

	// Make it appear as if the model has been successfully
	// migrated. Don't actually remove model documents to simulate it
	// having been migrated back to the controller.
	phases := []migration.Phase{
		migration.IMPORT,
		migration.PROCESSRELATIONS,
		migration.VALIDATION,
		migration.SUCCESS,
		migration.LOGTRANSFER,
		migration.REAP,
		migration.DONE,
		// Check that it is idempotent on DONE.
		migration.DONE,
	}
	for i := 0; i < 10; i++ {
		mig, err := s.State2.CreateMigration(s.stdSpec)
		c.Assert(err, jc.ErrorIsNil)
		for _, phase := range phases {
			c.Assert(mig.SetPhase(phase), jc.ErrorIsNil)
		}
		state.ResetMigrationMode(c, s.State2)
	}

	// Previous migration shouldn't be reported.
	_, err := s.State2.LatestMigration()
	c.Check(errors.IsNotFound(err), jc.IsTrue)
	c.Check(err, gc.ErrorMatches, "migration not found")

	// Start a new migration attempt, which should be reported.
	migNext, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	migNextb, err := s.State2.LatestMigration()
	c.Check(err, jc.ErrorIsNil)
	c.Check(migNextb.Id(), gc.Equals, migNext.Id())
	phase, err := migNextb.Phase()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(phase, gc.Equals, migration.QUIESCE)
}

func (s *MigrationSuite) TestLatestRemovedModelMigration(c *gc.C) {
	model, err := s.State2.Model()
	c.Assert(err, jc.ErrorIsNil)

	mig1, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	for _, phase := range migration.SuccessfulMigrationPhases() {
		c.Assert(mig1.SetPhase(phase), jc.ErrorIsNil)
	}

	// CompletedMigration should fail as the model docs are still there
	_, err = s.State2.CompletedMigration()
	c.Assert(errors.IsNotFound(err), gc.Equals, true)

	// Delete the model and check that we get back the MigrationModel
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(s.State2.RemoveDyingModel(), jc.ErrorIsNil)

	mig2, err := s.State2.CompletedMigration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig2, jc.DeepEquals, mig1)

	// Check that LatestMigration works with the model removed
	mig3, err := s.State2.LatestMigration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig3, jc.DeepEquals, mig1)
}

func (s *MigrationSuite) TestMigration(c *gc.C) {
	mig1, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := s.State2.Migration(mig1.Id())
	c.Check(err, jc.ErrorIsNil)
	c.Check(mig1.Id(), gc.Equals, mig2.Id())
	c.Check(mig2.StartTime().IsZero(), jc.IsFalse)
	c.Check(mig2.StartTime().Before(s.Clock.Now()), jc.IsTrue)
}

func (s *MigrationSuite) TestMigrationNotFound(c *gc.C) {
	_, err := s.State2.Migration("does not exist")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, "migration not found")
}

func (s *MigrationSuite) TestRefresh(c *gc.C) {
	mig1, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := s.State2.LatestMigration()
	c.Assert(err, jc.ErrorIsNil)

	err = mig1.SetPhase(migration.IMPORT)
	c.Assert(err, jc.ErrorIsNil)

	assertPhase(c, mig2, migration.QUIESCE)
	err = mig2.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertPhase(c, mig2, migration.IMPORT)
}

func (s *MigrationSuite) TestSuccessfulPhaseTransitions(c *gc.C) {
	st := s.State2

	mig, err := st.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig, gc.NotNil)

	mig2, err := st.LatestMigration()
	c.Assert(err, jc.ErrorIsNil)

	phases := migration.SuccessfulMigrationPhases()

	var successTime time.Time
	for _, phase := range phases[:len(phases)-1] {
		err := mig.SetPhase(phase)
		c.Assert(err, jc.ErrorIsNil)

		assertPhase(c, mig, phase)
		c.Check(mig.PhaseChangedTime().IsZero(), jc.IsFalse)
		c.Assert(mig.PhaseChangedTime().Before(s.Clock.Now()), jc.IsTrue)

		// Check success timestamp is set only when SUCCESS is
		// reached.
		if phase < migration.SUCCESS {
			c.Assert(mig.SuccessTime().IsZero(), jc.IsTrue)
		} else {
			if phase == migration.SUCCESS {
				successTime = s.Clock.Now()
			}
			if successTime.IsZero() {
				c.Assert(mig.SuccessTime().IsZero(), jc.IsTrue)
			} else {
				c.Assert(mig.SuccessTime().IsZero(), jc.IsFalse)
				c.Assert(mig.SuccessTime().Before(successTime), jc.IsTrue)
			}
		}

		// Check still marked as active.
		assertMigrationActive(c, s.State2)
		c.Assert(mig.EndTime().IsZero(), jc.IsTrue)

		// Ensure change was peristed.
		c.Assert(mig2.Refresh(), jc.ErrorIsNil)
		assertPhase(c, mig2, phase)

		s.Clock.Advance(time.Millisecond)
	}

	// Now move to the final phase (DONE) and ensure fields are set as
	// expected.
	err = mig.SetPhase(migration.DONE)
	c.Assert(err, jc.ErrorIsNil)
	assertPhase(c, mig, migration.DONE)
	s.assertMigrationCleanedUp(c, mig)
}

func (s *MigrationSuite) TestABORTCleanup(c *gc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.Clock.Advance(time.Millisecond)
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	s.Clock.Advance(time.Millisecond)
	c.Assert(mig.SetPhase(migration.ABORTDONE), jc.ErrorIsNil)

	s.assertMigrationCleanedUp(c, mig)

	// Model should be set back to active.
	model, err := s.State2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeNone)
}

func (s *MigrationSuite) TestREAPFAILEDCleanup(c *gc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	// Advance the migration to REAPFAILED.
	phases := []migration.Phase{
		migration.IMPORT,
		migration.PROCESSRELATIONS,
		migration.VALIDATION,
		migration.SUCCESS,
		migration.LOGTRANSFER,
		migration.REAP,
		migration.REAPFAILED,
	}
	for _, phase := range phases {
		s.Clock.Advance(time.Millisecond)
		c.Assert(mig.SetPhase(phase), jc.ErrorIsNil)
	}

	s.assertMigrationCleanedUp(c, mig)
}

func (s *MigrationSuite) assertMigrationCleanedUp(c *gc.C, mig state.ModelMigration) {
	c.Check(mig.PhaseChangedTime().IsZero(), jc.IsFalse)
	c.Assert(mig.PhaseChangedTime().Before(s.Clock.Now()), jc.IsTrue)
	c.Check(mig.EndTime().IsZero(), jc.IsFalse)
	c.Assert(mig.EndTime().Before(s.Clock.Now()), jc.IsTrue)
	assertMigrationNotActive(c, s.State2)
}

func (s *MigrationSuite) TestIllegalPhaseTransition(c *gc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	err = mig.SetPhase(migration.SUCCESS)
	c.Check(err, gc.ErrorMatches, "illegal phase change: QUIESCE -> SUCCESS")
}

func (s *MigrationSuite) TestPhaseChangeRace(c *gc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig, gc.Not(gc.IsNil))

	defer state.SetBeforeHooks(c, s.State2, func() {
		mig, err := s.State2.LatestMigration()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mig.SetPhase(migration.IMPORT), jc.ErrorIsNil)
	}).Check()

	err = mig.SetPhase(migration.IMPORT)
	c.Assert(err, gc.ErrorMatches, "phase already changed")
	assertPhase(c, mig, migration.QUIESCE)

	// After a refresh it the phase change should be ok.
	c.Assert(mig.Refresh(), jc.ErrorIsNil)
	err = mig.SetPhase(migration.IMPORT)
	c.Assert(err, jc.ErrorIsNil)
	assertPhase(c, mig, migration.IMPORT)
}

func (s *MigrationSuite) TestStatusMessage(c *gc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig, gc.Not(gc.IsNil))

	mig2, err := s.State2.LatestMigration()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mig.StatusMessage(), gc.Equals, "starting")
	c.Check(mig2.StatusMessage(), gc.Equals, "starting")

	err = mig.SetStatusMessage("foo bar")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mig.StatusMessage(), gc.Equals, "foo bar")

	c.Assert(mig2.Refresh(), jc.ErrorIsNil)
	c.Check(mig2.StatusMessage(), gc.Equals, "foo bar")
}

func (s *MigrationSuite) TestWatchForMigration(c *gc.C) {
	// Start watching for migration.
	w, wc := s.createMigrationWatcher(c, s.State2)
	wc.AssertOneChange()

	// Create the migration - should be reported.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Mere phase changes should not be reported.
	c.Check(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	wc.AssertNoChange()

	// Ending the migration should be reported.
	c.Check(mig.SetPhase(migration.ABORTDONE), jc.ErrorIsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *MigrationSuite) TestWatchForMigrationInProgress(c *gc.C) {
	// Create a migration.
	_, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.State2.ModelUUID())

	// Start watching for a migration - the in progress one should be reported.
	_, wc := s.createMigrationWatcher(c, s.State2)
	wc.AssertOneChange()
}

func (s *MigrationSuite) TestWatchForMigrationMultiModel(c *gc.C) {
	_, wc2 := s.createMigrationWatcher(c, s.State2)
	wc2.AssertOneChange()

	// Create another hosted model to migrate and watch for
	// migrations.
	State3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { State3.Close() })
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, State3.ModelUUID())
	_, wc3 := s.createMigrationWatcher(c, State3)
	wc3.AssertOneChange()

	// Create a migration for 2.
	_, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc2.AssertOneChange()
	wc3.AssertNoChange()

	// Create a migration for 3.
	_, err = State3.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc2.AssertNoChange()
	wc3.AssertOneChange()
}

func (s *MigrationSuite) createMigrationWatcher(c *gc.C, st *state.State) (
	state.NotifyWatcher, statetesting.NotifyWatcherC,
) {
	w := st.WatchForMigration()
	s.AddCleanup(func(c *gc.C) { statetesting.AssertStop(c, w) })
	return w, statetesting.NewNotifyWatcherC(c, w)
}

func (s *MigrationSuite) TestWatchMigrationStatus(c *gc.C) {
	w, wc := s.createStatusWatcher(c, s.State2)
	wc.AssertOneChange() // Initial event.

	// Create a migration.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// End it.
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	wc.AssertOneChange()
	c.Assert(mig.SetPhase(migration.ABORTDONE), jc.ErrorIsNil)
	wc.AssertOneChange()

	// Start another.
	mig2, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Change phase.
	c.Assert(mig2.SetPhase(migration.IMPORT), jc.ErrorIsNil)
	wc.AssertOneChange()

	// End it.
	c.Assert(mig2.SetPhase(migration.ABORT), jc.ErrorIsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *MigrationSuite) TestWatchMigrationStatusPreexisting(c *gc.C) {
	// Create an aborted migration.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)

	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.State2.ModelUUID())

	_, wc := s.createStatusWatcher(c, s.State2)
	wc.AssertOneChange()
}

func (s *MigrationSuite) TestWatchMigrationStatusMultiModel(c *gc.C) {
	_, wc2 := s.createStatusWatcher(c, s.State2)
	wc2.AssertOneChange() // initial event

	// Create another hosted model to migrate and watch for
	// migrations.
	State3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { State3.Close() })
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, State3.ModelUUID())

	_, wc3 := s.createStatusWatcher(c, State3)
	wc3.AssertOneChange() // initial event

	// Create a migration for 2.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc2.AssertOneChange()
	wc3.AssertNoChange()

	// Create a migration for 3.
	_, err = State3.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	wc2.AssertNoChange()
	wc3.AssertOneChange()

	// Update the migration for 2.
	err = mig.SetPhase(migration.ABORT)
	c.Assert(err, jc.ErrorIsNil)
	wc2.AssertOneChange()
	wc3.AssertNoChange()
}

func (s *MigrationSuite) TestMinionReports(c *gc.C) {
	// Create some machines and units to report with.
	factory2 := factory.NewFactory(s.State2, s.StatePool)
	m0 := factory2.MakeMachine(c, nil)
	u0 := factory2.MakeUnit(c, &factory.UnitParams{Machine: m0})
	m1 := factory2.MakeMachine(c, nil)
	m2 := factory2.MakeMachine(c, nil)

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	const phase = migration.QUIESCE
	c.Assert(mig.SubmitMinionReport(m0.Tag(), phase, true), jc.ErrorIsNil)
	c.Assert(mig.SubmitMinionReport(m1.Tag(), phase, false), jc.ErrorIsNil)
	c.Assert(mig.SubmitMinionReport(u0.Tag(), phase, true), jc.ErrorIsNil)

	reports, err := mig.MinionReports()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reports.Succeeded, jc.SameContents, []names.Tag{m0.Tag(), u0.Tag()})
	c.Check(reports.Failed, jc.SameContents, []names.Tag{m1.Tag()})
	c.Check(reports.Unknown, jc.SameContents, []names.Tag{m2.Tag()})
}

func (s *MigrationSuite) TestMinionReportsCAASLegacy(c *gc.C) {
	// Create some machines and units to report with.
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	factory2 := factory.NewFactory(st, s.StatePool)
	ch := factory2.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	a0 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a0", Charm: ch})
	a1 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a1", Charm: ch})
	a2 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a2", Charm: ch})

	mig, err := st.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	const phase = migration.QUIESCE
	c.Assert(mig.SubmitMinionReport(a0.Tag(), phase, true), jc.ErrorIsNil)
	c.Assert(mig.SubmitMinionReport(a1.Tag(), phase, false), jc.ErrorIsNil)

	reports, err := mig.MinionReports()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reports.Succeeded, jc.SameContents, []names.Tag{a0.Tag()})
	c.Check(reports.Failed, jc.SameContents, []names.Tag{a1.Tag()})
	c.Check(reports.Unknown, jc.SameContents, []names.Tag{a2.Tag()})
}

func (s *MigrationSuite) TestMinionReportsCAASEmbedded(c *gc.C) {
	// Create some machines and units to report with.
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	factory2 := factory.NewFactory(st, s.StatePool)
	ch := factory2.MakeCharmV2(c, &factory.CharmParams{
		Name:   "snappass-test",
		Series: "quantal",
	})
	a0 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a0", Charm: ch})
	u1a0, err := a0.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id0")})
	c.Assert(err, jc.ErrorIsNil)
	a1 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a1", Charm: ch})
	u1a1, err := a1.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id1")})
	c.Assert(err, jc.ErrorIsNil)
	a2 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a2", Charm: ch})
	u1a2, err := a2.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id2")})
	c.Assert(err, jc.ErrorIsNil)

	mig, err := st.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	const phase = migration.QUIESCE
	c.Assert(mig.SubmitMinionReport(u1a0.Tag(), phase, true), jc.ErrorIsNil)
	c.Assert(mig.SubmitMinionReport(u1a1.Tag(), phase, false), jc.ErrorIsNil)

	reports, err := mig.MinionReports()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reports.Succeeded, jc.SameContents, []names.Tag{u1a0.Tag()})
	c.Check(reports.Failed, jc.SameContents, []names.Tag{u1a1.Tag()})
	c.Check(reports.Unknown, jc.SameContents, []names.Tag{u1a2.Tag()})
}

func (s *MigrationSuite) TestDuplicateMinionReportsSameSuccess(c *gc.C) {
	// It should be OK for a minion report to arrive more than once
	// for the same migration, agent and phase as long as the value of
	// "success" is the same.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	tag := names.NewMachineTag("42")
	c.Check(mig.SubmitMinionReport(tag, migration.QUIESCE, true), jc.ErrorIsNil)
	c.Check(mig.SubmitMinionReport(tag, migration.QUIESCE, true), jc.ErrorIsNil)
}

func (s *MigrationSuite) TestDuplicateMinionReportsDifferingSuccess(c *gc.C) {
	// It is not OK for a minion report to arrive more than once for
	// the same migration, agent and phase when the "success" value
	// changes.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	tag := names.NewMachineTag("42")
	c.Check(mig.SubmitMinionReport(tag, migration.QUIESCE, true), jc.ErrorIsNil)
	err = mig.SubmitMinionReport(tag, migration.QUIESCE, false)
	c.Check(err, gc.ErrorMatches,
		fmt.Sprintf("conflicting reports received for %s/QUIESCE/machine-42", mig.Id()))
}

func (s *MigrationSuite) TestMinionReportWithOldPhase(c *gc.C) {
	// It is OK for a report to arrive for even a migration has moved
	// on.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	// Get another reference to the same migration.
	migalt, err := s.State2.LatestMigration()
	c.Assert(err, jc.ErrorIsNil)

	// Confirm that there's no reports when starting.
	reports, err := mig.MinionReports()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reports.Succeeded, gc.HasLen, 0)

	// Advance the migration
	c.Assert(mig.SetPhase(migration.IMPORT), jc.ErrorIsNil)

	// Submit minion report for the old phase.
	tag := names.NewMachineTag("42")
	c.Assert(mig.SubmitMinionReport(tag, migration.QUIESCE, true), jc.ErrorIsNil)

	// The report should still have been recorded.
	reports, err = migalt.MinionReports()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reports.Succeeded, jc.SameContents, []names.Tag{tag})
}

func (s *MigrationSuite) TestMinionReportWithInactiveMigration(c *gc.C) {
	// Create a migration.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	// Get another reference to the same migration.
	migalt, err := s.State2.LatestMigration()
	c.Assert(err, jc.ErrorIsNil)

	// Abort the migration.
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	c.Assert(mig.SetPhase(migration.ABORTDONE), jc.ErrorIsNil)

	// Confirm that there's no reports when starting.
	reports, err := mig.MinionReports()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reports.Succeeded, gc.HasLen, 0)

	// Submit a minion report for it.
	tag := names.NewMachineTag("42")
	c.Assert(mig.SubmitMinionReport(tag, migration.QUIESCE, true), jc.ErrorIsNil)

	// The report should still have been recorded.
	reports, err = migalt.MinionReports()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reports.Succeeded, jc.SameContents, []names.Tag{tag})
}

func (s *MigrationSuite) TestWatchMinionReports(c *gc.C) {
	mig, wc := s.createMigAndWatchReports(c, s.State2)
	wc.AssertOneChange() // initial event

	// A report should trigger the watcher.
	c.Assert(mig.SubmitMinionReport(names.NewMachineTag("0"), migration.QUIESCE, true), jc.ErrorIsNil)
	wc.AssertOneChange()

	// A report for a different phase shouldn't trigger the watcher.
	c.Assert(mig.SubmitMinionReport(names.NewMachineTag("1"), migration.IMPORT, true), jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *MigrationSuite) TestWatchMinionReportsMultiModel(c *gc.C) {
	mig, wc := s.createMigAndWatchReports(c, s.State2)
	wc.AssertOneChange() // initial event

	State3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { State3.Close() })
	mig3, wc3 := s.createMigAndWatchReports(c, State3)
	wc3.AssertOneChange() // initial event

	// Ensure the correct watchers are triggered.
	c.Assert(mig.SubmitMinionReport(names.NewMachineTag("0"), migration.QUIESCE, true), jc.ErrorIsNil)
	wc.AssertOneChange()
	wc3.AssertNoChange()

	c.Assert(mig3.SubmitMinionReport(names.NewMachineTag("0"), migration.QUIESCE, true), jc.ErrorIsNil)
	wc.AssertNoChange()
	wc3.AssertOneChange()
}

func (s *MigrationSuite) TestModelUserAccess(c *gc.C) {
	model, err := s.State2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeNone)

	// Get users that had access to the model before the migration
	modelUsers, err := model.Users()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(modelUsers), gc.Not(gc.Equals), 0)

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	for _, modelUser := range modelUsers {
		c.Logf("check that migration doc lists user %q having permission %q", modelUser.UserTag, modelUser.Access)
		perm := mig.ModelUserAccess(modelUser.UserTag)
		c.Assert(perm, gc.Equals, modelUser.Access)
	}

	// Querying for any other user should yield permission.NoAccess
	perm := mig.ModelUserAccess(names.NewUserTag("bogus"))
	c.Assert(perm, gc.Equals, permission.NoAccess)
}

func (s *MigrationSuite) createStatusWatcher(c *gc.C, st *state.State) (
	state.NotifyWatcher, statetesting.NotifyWatcherC,
) {
	s.WaitForModelWatchersIdle(c, st.ModelUUID())
	w := st.WatchMigrationStatus()
	s.AddCleanup(func(c *gc.C) { statetesting.AssertStop(c, w) })
	return w, statetesting.NewNotifyWatcherC(c, w)
}

func (s *MigrationSuite) createMigAndWatchReports(c *gc.C, st *state.State) (
	state.ModelMigration, statetesting.NotifyWatcherC,
) {
	mig, err := st.CreateMigration(s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, st.ModelUUID())

	w, err := mig.WatchMinionReports()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { statetesting.AssertStop(c, w) })
	wc := statetesting.NewNotifyWatcherC(c, w)

	return mig, wc
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
	isActive, err := st.IsMigrationActive()
	c.Assert(err, jc.ErrorIsNil)
	return isActive
}

func checkIdAndAttempt(c *gc.C, mig state.ModelMigration, expected int) {
	c.Check(mig.Id(), gc.Equals, fmt.Sprintf("%s:%d", mig.ModelUUID(), expected))
	c.Check(mig.Attempt(), gc.Equals, expected)
}
