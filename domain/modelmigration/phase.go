// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/internal/errors"
)

// Phase is the database representation of an export migration phase. Its values
// mirror the primary keys seeded into the model_migration_phase lookup table
// (domain/schema/controller/sql/0031-model-migration.PATCH.sql). The code-only
// sentinels migration.UNKNOWN and migration.NONE are never persisted and have
// no value here.
type Phase int

const (
	// PhaseQuiesce mirrors migration.QUIESCE.
	PhaseQuiesce Phase = 1
	// PhaseImport mirrors migration.IMPORT.
	PhaseImport Phase = 2
	// PhaseValidation mirrors migration.VALIDATION.
	PhaseValidation Phase = 3
	// PhaseSuccess mirrors migration.SUCCESS.
	PhaseSuccess Phase = 4
	// PhaseLogTransfer mirrors migration.LOGTRANSFER.
	PhaseLogTransfer Phase = 5
	// PhaseReap mirrors migration.REAP.
	PhaseReap Phase = 6
	// PhaseReapFailed mirrors migration.REAPFAILED.
	PhaseReapFailed Phase = 7
	// PhaseDone mirrors migration.DONE.
	PhaseDone Phase = 8
	// PhaseAbort mirrors migration.ABORT.
	PhaseAbort Phase = 9
	// PhaseAbortDone mirrors migration.ABORTDONE.
	PhaseAbortDone Phase = 10
)

// PhaseFromCoreMigrationPhase converts a wire-level [migration.Phase] into its
// persisted domain [Phase]. Phases that are never persisted (migration.UNKNOWN
// and migration.NONE) return an error satisfying [coreerrors.NotValid].
func PhaseFromCoreMigrationPhase(p migration.Phase) (Phase, error) {
	switch p {
	case migration.QUIESCE:
		return PhaseQuiesce, nil
	case migration.IMPORT:
		return PhaseImport, nil
	case migration.VALIDATION:
		return PhaseValidation, nil
	case migration.SUCCESS:
		return PhaseSuccess, nil
	case migration.LOGTRANSFER:
		return PhaseLogTransfer, nil
	case migration.REAP:
		return PhaseReap, nil
	case migration.REAPFAILED:
		return PhaseReapFailed, nil
	case migration.DONE:
		return PhaseDone, nil
	case migration.ABORT:
		return PhaseAbort, nil
	case migration.ABORTDONE:
		return PhaseAbortDone, nil
	}
	return Phase(-1), errors.Errorf(
		"migration phase %q has no persisted representation", p,
	).Add(coreerrors.NotValid)
}

// CoreMigrationPhase returns the wire-level [migration.Phase] for this persisted
// [Phase]. Values that have no corresponding phase return an error satisfying
// [coreerrors.NotValid].
func (p Phase) CoreMigrationPhase() (migration.Phase, error) {
	switch p {
	case PhaseQuiesce:
		return migration.QUIESCE, nil
	case PhaseImport:
		return migration.IMPORT, nil
	case PhaseValidation:
		return migration.VALIDATION, nil
	case PhaseSuccess:
		return migration.SUCCESS, nil
	case PhaseLogTransfer:
		return migration.LOGTRANSFER, nil
	case PhaseReap:
		return migration.REAP, nil
	case PhaseReapFailed:
		return migration.REAPFAILED, nil
	case PhaseDone:
		return migration.DONE, nil
	case PhaseAbort:
		return migration.ABORT, nil
	case PhaseAbortDone:
		return migration.ABORTDONE, nil
	}
	return migration.UNKNOWN, errors.Errorf(
		"persisted migration phase id %d is not recognised", int(p),
	).Add(coreerrors.NotValid)
}
