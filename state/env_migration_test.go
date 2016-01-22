// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	migration "github.com/juju/juju/core/envmigration"
	"github.com/juju/juju/state"
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
	s.State2 = s.Factory.MakeEnvironment(c, nil)
	s.AddCleanup(func(*gc.C) { s.State2.Close() })

	// Plausible migration arguments to test with.
	s.stdSpec = state.ModelMigrationSpec{
		InitiatedBy: "admin",
		TargetInfo: state.ModelMigTargetInfo{
			ControllerTag: names.NewEnvironTag(s.State.EnvironUUID()),
			Addrs:         []string{"1.2.3.4:5555", "4.3.2.1:6666"},
			CACert:        "cert",
			EntityTag:     names.NewUserTag("user"),
			Password:      "password",
		},
	}
}

func (s *ModelMigrationSuite) TestCreate(c *gc.C) {
	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mig.ModelUUID(), gc.Equals, s.State2.EnvironUUID())
	c.Check(mig.Id(), gc.Equals, mig.ModelUUID()+":0")

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
	st3 := s.Factory.MakeEnvironment(c, nil)
	s.AddCleanup(func(*gc.C) { st3.Close() })

	mig2, err := state.CreateModelMigration(st2, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mig2.Id(), gc.Equals, st2.EnvironUUID()+":0")

	mig3, err := state.CreateModelMigration(st3, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mig3.Id(), gc.Equals, st3.EnvironUUID()+":0")
}

func (s *ModelMigrationSuite) TestIdSequencesIncrement(c *gc.C) {
	createAndAbort := func() string {
		mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
		return mig.Id()
	}

	envUUID := s.State2.EnvironUUID()
	c.Check(createAndAbort(), gc.Equals, envUUID+":0")
	c.Check(createAndAbort(), gc.Equals, envUUID+":1")
	c.Check(createAndAbort(), gc.Equals, envUUID+":2")
}

func (s *ModelMigrationSuite) TestIdSequencesIncrementOnlyWhenNecessary(c *gc.C) {
	// Ensure that sequence numbers aren't "used up" unnecessarily
	// when the create txn is going to fail.
	envUUID := s.State2.EnvironUUID()

	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mig.Id(), gc.Equals, envUUID+":0")

	// This attempt will fail because a migration is already in
	// progress.
	_, err = state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(err, gc.ErrorMatches, ".+already in progress")

	// Now abort the migration and create another. The Id sequence
	// should have only incremented by 1.
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)

	mig, err = state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mig.Id(), gc.Equals, envUUID+":1")
}

func (s *ModelMigrationSuite) TestSpecValidation(c *gc.C) {
	tests := []struct {
		label        string
		tweakSpec    func(*state.ModelMigrationSpec)
		errorPattern string
	}{{
		"empty InitiatedBy",
		func(spec *state.ModelMigrationSpec) {
			spec.InitiatedBy = ""
		},
		"empty InitiatedBy not valid",
	}, {
		"empty ControllerTag",
		func(spec *state.ModelMigrationSpec) {
			spec.TargetInfo.ControllerTag = names.NewEnvironTag("fooo")
		},
		"ControllerTag not valid",
	}, {
		"invalid ControllerTag",
		func(spec *state.ModelMigrationSpec) {
			spec.TargetInfo.ControllerTag = names.NewEnvironTag("")
		},
		"ControllerTag not valid",
	}, {
		"nil Addrs",
		func(spec *state.ModelMigrationSpec) {
			spec.TargetInfo.Addrs = nil
		},
		"nil Addrs not valid",
	}, {
		"empty Addrs",
		func(spec *state.ModelMigrationSpec) {
			spec.TargetInfo.Addrs = []string{}
		},
		"empty Addrs not valid",
	}, {
		"invalid Addrs",
		func(spec *state.ModelMigrationSpec) {
			spec.TargetInfo.Addrs = []string{"1.2.3.4:555", "abc"}
		},
		`"abc" in Addrs not valid`,
	}, {
		"CACert",
		func(spec *state.ModelMigrationSpec) {
			spec.TargetInfo.CACert = ""
		},
		"empty CACert not valid",
	}, {
		"EntityTag",
		func(spec *state.ModelMigrationSpec) {
			spec.TargetInfo.EntityTag = names.NewMachineTag("")
		},
		"empty EntityTag not valid",
	}, {
		"Password",
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
		mig, err := state.CreateModelMigration(s.State2, spec)
		c.Check(mig, gc.IsNil)
		c.Check(errors.IsNotValid(err), jc.IsTrue)
		c.Check(err, gc.ErrorMatches, test.errorPattern)
	}
}

func (s *ModelMigrationSuite) TestCreateWithControllerModel(c *gc.C) {
	mig, err := state.CreateModelMigration(
		s.State, // This is the State for the controller
		s.stdSpec,
	)
	c.Check(mig, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "controllers can't be migrated")
}

func (s *ModelMigrationSuite) TestCreateMigrationInProgress(c *gc.C) {
	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(mig, gc.Not(gc.IsNil))
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Check(mig2, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "failed to create migration: already in progress")
}

func (s *ModelMigrationSuite) TestCreateMigrationRace(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State2, func() {
		mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
		c.Assert(mig, gc.Not(gc.IsNil))
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Check(mig, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "failed to create migration: already in progress")
}

func (s *ModelMigrationSuite) TestGet(c *gc.C) {
	mig1, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := state.GetModelMigration(s.State2)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mig1.Id(), gc.Equals, mig2.Id())
}

func (s *ModelMigrationSuite) TestGetNotExist(c *gc.C) {
	mig, err := state.GetModelMigration(s.State2)
	c.Check(mig, gc.IsNil)
	c.Check(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ModelMigrationSuite) TestGetsLatestAttempt(c *gc.C) {
	envUUID := s.State2.EnvironUUID()

	for i := 0; i < 10; i++ {
		c.Logf("loop %d", i)
		_, err := state.CreateModelMigration(s.State2, s.stdSpec)
		c.Assert(err, jc.ErrorIsNil)

		mig, err := state.GetModelMigration(s.State2)
		c.Check(mig.Id(), gc.Equals, fmt.Sprintf("%s:%d", envUUID, i))

		c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	}
}

func (s *ModelMigrationSuite) TestRefresh(c *gc.C) {
	mig1, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := state.GetModelMigration(s.State2)
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

	mig, err := state.CreateModelMigration(st, s.stdSpec)
	c.Assert(mig, gc.Not(gc.IsNil))
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := state.GetModelMigration(st)
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
	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.clock.Advance(time.Millisecond)
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)

	s.assertMigrationCleanedUp(c, mig)
}

func (s *ModelMigrationSuite) TestREAPFAILEDCleanup(c *gc.C) {
	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
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

func (s *ModelMigrationSuite) assertMigrationCleanedUp(c *gc.C, mig *state.ModelMigration) {
	c.Assert(mig.PhaseChangedTime(), gc.Equals, s.clock.Now())
	c.Assert(mig.EndTime(), gc.Equals, s.clock.Now())
	assertMigrationNotActive(c, s.State2)
}

func (s *ModelMigrationSuite) TestIllegalPhaseTransition(c *gc.C) {
	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	err = mig.SetPhase(migration.SUCCESS)
	c.Check(err, gc.ErrorMatches, "failed to update phase: illegal change: QUIESCE -> SUCCESS")
}

func (s *ModelMigrationSuite) TestPhaseChangeWithStaleInstance1(c *gc.C) {
	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	// Make mig stale by changing the phase with another instance.
	mig2, err := state.GetModelMigration(s.State2)
	c.Assert(err, jc.ErrorIsNil)
	err = mig2.SetPhase(migration.READONLY)
	c.Assert(err, jc.ErrorIsNil)

	// Setting to READONLY when the phase is already READONLY should be ok.
	err = mig.SetPhase(migration.READONLY)
	c.Assert(err, jc.ErrorIsNil)
	assertPhase(c, mig, migration.READONLY)
}

func (s *ModelMigrationSuite) TestPhaseChangeWithStaleInstance2(c *gc.C) {
	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(err, jc.ErrorIsNil)

	// Make mig stale by changing the phase with another instance. The
	// phase is changed to a terminal phase so that any future phase
	// change (via any ModelMigration instance) should fail.
	mig2, err := state.GetModelMigration(s.State2)
	c.Assert(err, jc.ErrorIsNil)
	err = mig2.SetPhase(migration.ABORT)
	c.Assert(err, jc.ErrorIsNil)

	// Setting to READONLY when the phase is already READONLY should be ok.
	err = mig.SetPhase(migration.READONLY)
	c.Assert(err, gc.ErrorMatches, "failed to update phase: illegal change: ABORT -> READONLY")
	assertPhase(c, mig, migration.ABORT)
}

func (s *ModelMigrationSuite) TestPhaseChangeRace(c *gc.C) {
	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(mig, gc.Not(gc.IsNil))

	defer state.SetBeforeHooks(c, s.State2, func() {
		mig, err := state.GetModelMigration(s.State2)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mig.SetPhase(migration.READONLY), jc.ErrorIsNil)
		c.Assert(mig.SetPhase(migration.PRECHECK), jc.ErrorIsNil)
	}).Check()

	err = mig.SetPhase(migration.READONLY)
	c.Assert(err, gc.ErrorMatches, "failed to update phase: illegal change: PRECHECK -> READONLY")
	assertPhase(c, mig, migration.PRECHECK)
}

func (s *ModelMigrationSuite) TestStatusMessage(c *gc.C) {
	mig, err := state.CreateModelMigration(s.State2, s.stdSpec)
	c.Assert(mig, gc.Not(gc.IsNil))

	mig2, err := state.GetModelMigration(s.State2)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mig.StatusMessage(), gc.Equals, "")
	c.Check(mig2.StatusMessage(), gc.Equals, "")

	err = mig.SetStatusMessage("foo bar")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mig.StatusMessage(), gc.Equals, "foo bar")

	c.Assert(mig2.Refresh(), jc.ErrorIsNil)
	c.Check(mig2.StatusMessage(), gc.Equals, "foo bar")
}

func assertPhase(c *gc.C, mig *state.ModelMigration, phase migration.Phase) {
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
	isActive, err := state.IsModelMigrationActive(st, st.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)
	return isActive
}
