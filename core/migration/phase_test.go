// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/internal/testing"
)

type PhaseSuite struct {
	coretesting.BaseSuite
}

func TestPhaseSuite(t *testing.T) {
	tc.Run(t, new(PhaseSuite))
}

func (s *PhaseSuite) TestUNKNOWN(c *tc.C) {
	// 0 should be UNKNOWN to guard against uninitialised struct
	// fields.
	c.Check(migration.Phase(0), tc.Equals, migration.UNKNOWN)
}

func (s *PhaseSuite) TestStringValid(c *tc.C) {
	c.Check(migration.IMPORT.String(), tc.Equals, "IMPORT")
	c.Check(migration.UNKNOWN.String(), tc.Equals, "UNKNOWN")
	c.Check(migration.ABORT.String(), tc.Equals, "ABORT")
}

func (s *PhaseSuite) TestInvalid(c *tc.C) {
	c.Check(migration.Phase(-1).String(), tc.Equals, "UNKNOWN")
	c.Check(migration.Phase(9999).String(), tc.Equals, "UNKNOWN")
}

func (s *PhaseSuite) TestParseValid(c *tc.C) {
	phase, ok := migration.ParsePhase("REAP")
	c.Check(phase, tc.Equals, migration.REAP)
	c.Check(ok, tc.IsTrue)
}

func (s *PhaseSuite) TestParseInvalid(c *tc.C) {
	phase, ok := migration.ParsePhase("foo")
	c.Check(phase, tc.Equals, migration.UNKNOWN)
	c.Check(ok, tc.IsFalse)
}

func (s *PhaseSuite) TestIsTerminal(c *tc.C) {
	c.Check(migration.QUIESCE.IsTerminal(), tc.IsFalse)
	c.Check(migration.SUCCESS.IsTerminal(), tc.IsFalse)
	c.Check(migration.ABORT.IsTerminal(), tc.IsFalse)
	c.Check(migration.ABORTDONE.IsTerminal(), tc.IsTrue)
	c.Check(migration.REAPFAILED.IsTerminal(), tc.IsTrue)
	c.Check(migration.DONE.IsTerminal(), tc.IsTrue)
}

func (s *PhaseSuite) TestIsRunning(c *tc.C) {
	c.Check(migration.UNKNOWN.IsRunning(), tc.IsFalse)
	c.Check(migration.NONE.IsRunning(), tc.IsFalse)

	c.Check(migration.QUIESCE.IsRunning(), tc.IsTrue)
	c.Check(migration.IMPORT.IsRunning(), tc.IsTrue)
	c.Check(migration.PROCESSRELATIONS.IsRunning(), tc.IsTrue)
	c.Check(migration.SUCCESS.IsRunning(), tc.IsTrue)

	c.Check(migration.LOGTRANSFER.IsRunning(), tc.IsFalse)
	c.Check(migration.REAP.IsRunning(), tc.IsFalse)
	c.Check(migration.REAPFAILED.IsRunning(), tc.IsFalse)
	c.Check(migration.DONE.IsRunning(), tc.IsFalse)
	c.Check(migration.ABORT.IsRunning(), tc.IsFalse)
	c.Check(migration.ABORTDONE.IsRunning(), tc.IsFalse)
}

func (s *PhaseSuite) TestCanTransitionTo(c *tc.C) {
	c.Check(migration.QUIESCE.CanTransitionTo(migration.SUCCESS), tc.IsFalse)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.ABORT), tc.IsTrue)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.IMPORT), tc.IsTrue)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.PROCESSRELATIONS), tc.IsFalse)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.Phase(-1)), tc.IsFalse)
	c.Check(migration.ABORT.CanTransitionTo(migration.QUIESCE), tc.IsFalse)

	// The new migration path skips the retired PROCESSRELATIONS phase: IMPORT
	// transitions directly to VALIDATION. The PROCESSRELATIONS edge is retained
	// transitionally for the legacy worker.
	c.Check(migration.IMPORT.CanTransitionTo(migration.VALIDATION), tc.IsTrue)
	c.Check(migration.IMPORT.CanTransitionTo(migration.PROCESSRELATIONS), tc.IsTrue)
	c.Check(migration.IMPORT.CanTransitionTo(migration.ABORT), tc.IsTrue)
	c.Check(migration.IMPORT.CanTransitionTo(migration.SUCCESS), tc.IsFalse)
}

// TestPhasePersistedIDRoundTrip asserts every phase that is stored in the
// model_migration_phase lookup table converts to its seeded id and back. The
// ids must match the seed in
// domain/schema/controller/sql/0031-model-migration.PATCH.sql exactly.
func (s *PhaseSuite) TestPhasePersistedIDRoundTrip(c *tc.C) {
	expected := map[migration.Phase]int{
		migration.QUIESCE:     1,
		migration.IMPORT:      2,
		migration.VALIDATION:  3,
		migration.SUCCESS:     4,
		migration.LOGTRANSFER: 5,
		migration.REAP:        6,
		migration.REAPFAILED:  7,
		migration.DONE:        8,
		migration.ABORT:       9,
		migration.ABORTDONE:   10,
	}
	for phase, id := range expected {
		gotID, err := migration.PhasePersistedID(phase)
		c.Check(err, tc.ErrorIsNil, tc.Commentf("phase %v", phase))
		c.Check(gotID, tc.Equals, id, tc.Commentf("phase %v", phase))

		gotPhase, err := migration.PhaseFromPersistedID(id)
		c.Check(err, tc.ErrorIsNil, tc.Commentf("id %d", id))
		c.Check(gotPhase, tc.Equals, phase, tc.Commentf("id %d", id))
	}
}

// TestPhasePersistedIDRejectsNonPersisted asserts the code-only sentinels and
// the retired PROCESSRELATIONS phase have no persisted representation. This is
// the reconciliation guard that keeps the Go enum and the SQL lookup in sync
// while PROCESSRELATIONS still exists in the enum.
func (s *PhaseSuite) TestPhasePersistedIDRejectsNonPersisted(c *tc.C) {
	for _, phase := range []migration.Phase{
		migration.UNKNOWN,
		migration.NONE,
		migration.PROCESSRELATIONS,
		migration.Phase(-1),
	} {
		_, err := migration.PhasePersistedID(phase)
		c.Check(err, tc.ErrorIs, migration.ErrPhaseNotPersisted, tc.Commentf("phase %v", phase))
	}
}

// TestPhaseFromPersistedIDRejectsUnknown asserts ids with no corresponding
// phase are rejected rather than silently mapped to a sentinel.
func (s *PhaseSuite) TestPhaseFromPersistedIDRejectsUnknown(c *tc.C) {
	for _, id := range []int{0, -1, 11, 999} {
		_, err := migration.PhaseFromPersistedID(id)
		c.Check(err, tc.ErrorIs, migration.ErrPhaseNotPersisted, tc.Commentf("id %d", id))
	}
}
