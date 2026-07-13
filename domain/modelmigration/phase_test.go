// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/domain/modelmigration"
)

type phaseSuite struct{}

func TestPhaseSuite(t *testing.T) {
	tc.Run(t, &phaseSuite{})
}

// TestPhaseRoundTrip asserts every persisted phase converts to its domain
// value and back, and that the domain value matches the primary key seeded in
// domain/schema/controller/sql/0031-model-migration.PATCH.sql exactly.
func (s *phaseSuite) TestPhaseRoundTrip(c *tc.C) {
	expected := map[migration.Phase]modelmigration.Phase{
		migration.QUIESCE:     modelmigration.PhaseQuiesce,
		migration.IMPORT:      modelmigration.PhaseImport,
		migration.VALIDATION:  modelmigration.PhaseValidation,
		migration.SUCCESS:     modelmigration.PhaseSuccess,
		migration.LOGTRANSFER: modelmigration.PhaseLogTransfer,
		migration.REAP:        modelmigration.PhaseReap,
		migration.REAPFAILED:  modelmigration.PhaseReapFailed,
		migration.DONE:        modelmigration.PhaseDone,
		migration.ABORT:       modelmigration.PhaseAbort,
		migration.ABORTDONE:   modelmigration.PhaseAbortDone,
	}
	// The ids seeded in the lookup table, guarded here so the constants can
	// never silently drift from the schema.
	expectedIDs := map[migration.Phase]int{
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
	for phase, domainPhase := range expected {
		got, err := modelmigration.PhaseFromCoreMigrationPhase(phase)
		c.Check(err, tc.ErrorIsNil, tc.Commentf("phase %v", phase))
		c.Check(got, tc.Equals, domainPhase, tc.Commentf("phase %v", phase))
		c.Check(int(got), tc.Equals, expectedIDs[phase], tc.Commentf("phase %v", phase))

		back, err := domainPhase.CoreMigrationPhase()
		c.Check(err, tc.ErrorIsNil, tc.Commentf("phase %v", phase))
		c.Check(back, tc.Equals, phase, tc.Commentf("phase %v", phase))
	}
}

// TestPhaseFromCoreRejectsNonPersisted asserts the code-only sentinels have no
// persisted representation.
func (s *phaseSuite) TestPhaseFromCoreRejectsNonPersisted(c *tc.C) {
	for _, phase := range []migration.Phase{
		migration.UNKNOWN,
		migration.NONE,
		migration.Phase(-1),
	} {
		_, err := modelmigration.PhaseFromCoreMigrationPhase(phase)
		c.Check(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("phase %v", phase))
	}
}

// TestCoreMigrationPhaseRejectsUnknownID asserts ids with no corresponding
// phase are rejected rather than silently mapped to a sentinel.
func (s *phaseSuite) TestCoreMigrationPhaseRejectsUnknownID(c *tc.C) {
	for _, id := range []int{0, -1, 11, 999} {
		_, err := modelmigration.Phase(id).CoreMigrationPhase()
		c.Check(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("id %d", id))
	}
}
