// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"reflect"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	migration "github.com/juju/juju/core/envmigration"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type EnvMigrationSuite struct {
	ConnSuite
	State2  *state.State
	clock   *coretesting.Clock
	stdArgs state.EnvMigrationArgs
}

var _ = gc.Suite(new(EnvMigrationSuite))

func (s *EnvMigrationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	// Create a hosted environment to migrated.
	s.State2 = s.Factory.MakeEnvironment(c, nil)
	s.AddCleanup(func(*gc.C) { s.State2.Close() })

	s.clock = coretesting.NewClock(time.Now().Truncate(time.Second))

	// Plausible migration arguments to test with.
	s.stdArgs = state.EnvMigrationArgs{
		Owner:            "owner",
		TargetController: "uuid",
		TargetAPIAddresses: []network.HostPort{
			{network.Address{Value: "1.2.3.4"}, 5555},
			{network.Address{Value: "4.3.2.1"}, 6666},
		},
		TargetUser:     "user",
		TargetPassword: "password",
		Clock:          s.clock,
	}
}

func (s *EnvMigrationSuite) TestCreate(c *gc.C) {
	apiAddrs := s.stdArgs.TargetAPIAddresses
	mig, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mig.EnvUUID(), gc.Equals, s.State2.EnvironUUID())
	c.Assert(mig.Id(), gc.Equals, mig.EnvUUID()+":0")

	c.Assert(mig.StartTime(), gc.Equals, s.clock.Now())

	c.Assert(mig.SuccessTime().IsZero(), jc.IsTrue)
	c.Assert(mig.EndTime().IsZero(), jc.IsTrue)
	c.Assert(mig.StatusMessage(), gc.Equals, "")
	c.Assert(mig.Owner(), gc.Equals, "owner")
	c.Assert(mig.TargetController(), gc.Equals, "uuid")

	assertPhase(c, mig, migration.QUIESCE)
	c.Assert(mig.PhaseChangedTime(), gc.Equals, mig.StartTime())

	outApiAddrs, err := mig.TargetAPIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outApiAddrs, gc.HasLen, len(apiAddrs))
	for i, inAddr := range apiAddrs {
		outAddr := outApiAddrs[i]
		c.Assert(inAddr.Address.Value, gc.Equals, outAddr.Address.Value)
		c.Assert(inAddr.Port, gc.Equals, outAddr.Port)
	}

	username, password := mig.TargetAuthInfo()
	c.Assert(username, gc.Equals, "user")
	c.Assert(password, gc.Equals, "password")

	assertEnvMigActive(c, s.State2)
}

func (s *EnvMigrationSuite) TestIdSequencesAreIndependent(c *gc.C) {
	st2 := s.State2
	st3 := s.Factory.MakeEnvironment(c, nil)
	s.AddCleanup(func(*gc.C) { st3.Close() })

	mig2, err := state.CreateEnvMigration(st2, s.stdArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig2.Id(), gc.Equals, st2.EnvironUUID()+":0")

	mig3, err := state.CreateEnvMigration(st3, s.stdArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig3.Id(), gc.Equals, st3.EnvironUUID()+":0")
}

func (s *EnvMigrationSuite) TestIdSequencesIncrement(c *gc.C) {
	createAndAbort := func() string {
		mig, err := state.CreateEnvMigration(s.State2, s.stdArgs)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
		return mig.Id()
	}

	envUUID := s.State2.EnvironUUID()
	c.Assert(createAndAbort(), gc.Equals, envUUID+":0")
	c.Assert(createAndAbort(), gc.Equals, envUUID+":1")
	c.Assert(createAndAbort(), gc.Equals, envUUID+":2")
}

func (s *EnvMigrationSuite) TestIdSequencesIncrementOnlyWhenNecessary(c *gc.C) {
	// Ensure that sequence numbers aren't "used up" unnecessarily
	// when the create txn is going to fail.
	envUUID := s.State2.EnvironUUID()

	mig, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig.Id(), gc.Equals, envUUID+":0")

	// This attempt will fail because a migration is already in
	// progress.
	_, err = state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(err, gc.ErrorMatches, ".+already in progress")

	// Now abort the migration and create another. The Id sequence
	// should have only incremented by 1.
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)

	mig, err = state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mig.Id(), gc.Equals, envUUID+":1")
}

func (s *EnvMigrationSuite) TestCreateWithMissingArgs(c *gc.C) {
	typ := reflect.TypeOf(s.stdArgs)
	numFields := typ.NumField()
	for i := 0; i < numFields; i++ {
		name := typ.Field(i).Name
		if name == "Clock" { // Clock is allowed to be empty
			continue
		}

		// Copy the args and clear a field by setting it to its zero value.
		args := s.stdArgs
		field := reflect.ValueOf(&args).Elem().Field(i)
		field.Set(reflect.Zero(field.Type()))

		// Ensure that CreateEnvMigration complains that the field is missing.
		mig, err := state.CreateEnvMigration(s.State2, args)
		c.Assert(mig, gc.IsNil)
		c.Assert(err, gc.ErrorMatches, fmt.Sprintf("argument missing: %s", name))
	}
}

func (s *EnvMigrationSuite) TestCreateWithControllerEnv(c *gc.C) {
	mig, err := state.CreateEnvMigration(
		s.State, // This is the State for the controller
		s.stdArgs,
	)
	c.Assert(mig, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "controllers can't be migrated")
}

func (s *EnvMigrationSuite) TestCreateFailsForMigratedEnvs(c *gc.C) {
	c.Skip("XXX todo")
}

func (s *EnvMigrationSuite) TestCreateMigrationInProgress(c *gc.C) {
	mig, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(mig, gc.Not(gc.IsNil))
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(mig2, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "failed to create migration: already in progress")
}

func (s *EnvMigrationSuite) TestCreateMigrationRace(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State2, func() {
		mig, err := state.CreateEnvMigration(s.State2, s.stdArgs)
		c.Assert(mig, gc.Not(gc.IsNil))
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	mig, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(mig, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "failed to create migration: already in progress")
}

func (s *EnvMigrationSuite) TestGet(c *gc.C) {
	mig1, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := state.GetEnvMigration(s.State2, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mig1.Id(), gc.Equals, mig2.Id())
}

func (s *EnvMigrationSuite) TestGetNotExist(c *gc.C) {
	mig, err := state.GetEnvMigration(s.State2, s.clock)
	c.Assert(mig, gc.IsNil)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *EnvMigrationSuite) TestGetsLatestAttempt(c *gc.C) {
	envUUID := s.State2.EnvironUUID()

	for i := 0; i < 10; i++ {
		_, err := state.CreateEnvMigration(s.State2, s.stdArgs)
		c.Assert(err, jc.ErrorIsNil)

		mig, err := state.GetEnvMigration(s.State2, s.clock)
		c.Assert(mig.Id(), gc.Equals, fmt.Sprintf("%s:%d", envUUID, i))

		c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	}
}

func (s *EnvMigrationSuite) TestRefresh(c *gc.C) {
	mig1, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := state.GetEnvMigration(s.State2, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	err = mig1.SetPhase(migration.READONLY)
	c.Assert(err, jc.ErrorIsNil)

	assertPhase(c, mig2, migration.QUIESCE)
	err = mig2.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertPhase(c, mig2, migration.READONLY)
}

func (s *EnvMigrationSuite) TestSuccessfulPhaseTransitions(c *gc.C) {
	st := s.State2

	mig, err := state.CreateEnvMigration(st, s.stdArgs)
	c.Assert(mig, gc.Not(gc.IsNil))
	c.Assert(err, jc.ErrorIsNil)

	mig2, err := state.GetEnvMigration(st, s.clock)
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

		// Check success timestamp is set only when EnvMigSUCCESS is
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
		assertEnvMigActive(c, s.State2)
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
	c.Assert(mig.PhaseChangedTime(), gc.Equals, s.clock.Now())
	c.Assert(mig.EndTime(), gc.Equals, s.clock.Now())
	assertEnvMigNotActive(c, s.State2)
}

func (s *EnvMigrationSuite) TestABORTCleanup(c *gc.C) {
	c.Skip("XXX to do")
}

func (s *EnvMigrationSuite) TestREAPFAILEDCleanup(c *gc.C) {
	c.Skip("XXX to do")
}

func (s *EnvMigrationSuite) TestIllegalPhaseTransition(c *gc.C) {
	mig, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(err, jc.ErrorIsNil)

	err = mig.SetPhase(migration.SUCCESS)
	c.Assert(err, gc.ErrorMatches, "failed to update phase: illegal change: QUIESCE -> SUCCESS")
}

func (s *EnvMigrationSuite) TestPhaseChangeWithStaleInstance1(c *gc.C) {
	mig, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(err, jc.ErrorIsNil)

	// Make mig stale by changing the phase with another instance.
	mig2, err := state.GetEnvMigration(s.State2, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	err = mig2.SetPhase(migration.READONLY)
	c.Assert(err, jc.ErrorIsNil)

	// Setting to READONLY when the phase is already READONLY should be ok.
	err = mig.SetPhase(migration.READONLY)
	c.Assert(err, jc.ErrorIsNil)
	assertPhase(c, mig, migration.READONLY)
}

func (s *EnvMigrationSuite) TestPhaseChangeWithStaleInstance2(c *gc.C) {
	mig, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(err, jc.ErrorIsNil)

	// Make mig stale by changing the phase with another instance. The
	// phase is changed to a terminal phase so that any future phase
	// change (via any EnvMigration instance) should fail.
	mig2, err := state.GetEnvMigration(s.State2, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	err = mig2.SetPhase(migration.ABORT)
	c.Assert(err, jc.ErrorIsNil)

	// Setting to READONLY when the phase is already READONLY should be ok.
	err = mig.SetPhase(migration.READONLY)
	c.Assert(err, gc.ErrorMatches, "failed to update phase: illegal change: ABORT -> READONLY")
	assertPhase(c, mig, migration.ABORT)
}

func (s *EnvMigrationSuite) TestPhaseChangeRace(c *gc.C) {
	mig, err := state.CreateEnvMigration(s.State2, s.stdArgs)
	c.Assert(mig, gc.Not(gc.IsNil))

	defer state.SetBeforeHooks(c, s.State2, func() {
		mig, err := state.GetEnvMigration(s.State2, s.clock)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mig.SetPhase(migration.READONLY), jc.ErrorIsNil)
		c.Assert(mig.SetPhase(migration.PRECHECK), jc.ErrorIsNil)
	}).Check()

	err = mig.SetPhase(migration.READONLY)
	c.Assert(err, gc.ErrorMatches, "failed to update phase: illegal change: PRECHECK -> READONLY")
	assertPhase(c, mig, migration.PRECHECK)
}

func assertPhase(c *gc.C, mig *state.EnvMigration, phase migration.Phase) {
	actualPhase, err := mig.Phase()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actualPhase, gc.Equals, phase)
}

func assertEnvMigActive(c *gc.C, st *state.State) {
	c.Assert(isEnvMigrationActive(c, st), jc.IsTrue)
}

func assertEnvMigNotActive(c *gc.C, st *state.State) {
	c.Assert(isEnvMigrationActive(c, st), jc.IsFalse)
}

func isEnvMigrationActive(c *gc.C, st *state.State) bool {
	isActive, err := state.IsEnvMigrationActive(st, st.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)
	return isActive
}
