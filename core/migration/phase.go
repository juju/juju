// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"slices"

	"github.com/juju/juju/internal/errors"
)

// ErrPhaseNotPersisted indicates a phase has no representation in the
// model_migration_phase lookup table and therefore cannot be converted to or
// from a persisted phase id. This covers the code-only sentinels UNKNOWN and
// NONE, and the retired PROCESSRELATIONS phase, none of which are ever written
// to the database.
const ErrPhaseNotPersisted = errors.ConstError("migration phase is not persisted")

// persistedPhaseIDs maps each migration phase to the primary key it is stored
// under in the seeded model_migration_phase lookup table
// (domain/schema/controller/sql/0031-model-migration.PATCH.sql). It is the
// single source of truth for Go<->SQL phase conversion. The mapping is explicit
// and deliberately does NOT track the enum ordinal: the lookup omits the
// code-only sentinels UNKNOWN/NONE and the retired PROCESSRELATIONS phase, so
// the Go enum and the persisted ids must be reconciled by name/value here
// rather than by position.
var persistedPhaseIDs = map[Phase]int{
	QUIESCE:     1,
	IMPORT:      2,
	VALIDATION:  3,
	SUCCESS:     4,
	LOGTRANSFER: 5,
	REAP:        6,
	REAPFAILED:  7,
	DONE:        8,
	ABORT:       9,
	ABORTDONE:   10,
}

// Phase values specify model migration phases.
type Phase int

// Enumerate all possible migration phases.
const (
	UNKNOWN Phase = iota
	NONE
	QUIESCE
	IMPORT
	PROCESSRELATIONS
	VALIDATION
	SUCCESS
	LOGTRANSFER
	REAP
	REAPFAILED
	DONE
	ABORT
	ABORTDONE
)

var phaseNames = []string{
	"UNKNOWN", // To catch uninitialised fields.
	"NONE",    // For watchers to indicate there's never been a migration attempt.
	"QUIESCE",
	"IMPORT",
	"PROCESSRELATIONS",
	"VALIDATION",
	"SUCCESS",
	"LOGTRANSFER",
	"REAP",
	"REAPFAILED",
	"DONE",
	"ABORT",
	"ABORTDONE",
}

// Those phases are only used to get a complete successful round for testing purposes.
func SuccessfulMigrationPhases() []Phase {
	return []Phase{
		IMPORT,
		PROCESSRELATIONS,
		VALIDATION,
		SUCCESS,
		LOGTRANSFER,
		REAP,
		DONE,
	}
}

// String returns the name of an model migration phase constant.
func (p Phase) String() string {
	i := int(p)
	if i >= 0 && i < len(phaseNames) {
		return phaseNames[i]
	}
	return "UNKNOWN"
}

// CanTransitionTo returns true if the given phase is a valid next
// model migration phase.
func (p Phase) CanTransitionTo(targetPhase Phase) bool {
	nextPhases, exists := validTransitions[p]
	if !exists {
		return false
	}
	return slices.Contains(nextPhases, targetPhase)
}

// IsTerminal returns true if the phase is one which signifies the end
// of a migration.
func (p Phase) IsTerminal() bool {
	return slices.Contains(terminalPhases, p)
}

// IsRunning returns true if the phase indicates the migration is
// active and up to or at the SUCCESS phase. It returns false if the
// phase is one of the final cleanup phases or indicates an failed
// migration.
func (p Phase) IsRunning() bool {
	if p.IsTerminal() {
		return false
	}
	switch p {
	case QUIESCE, IMPORT, PROCESSRELATIONS, VALIDATION, SUCCESS:
		return true
	default:
		return false
	}
}

// IsPostSuccess returns true if the phase is one of the post SUCCESS phases,
// which allow the migration to complete successfully. They phases won't include
// ABORT or ABORTDONE, which are used for failed migrations.
func (p Phase) IsPostSuccess() bool {
	switch p {
	case LOGTRANSFER, REAP, REAPFAILED, DONE:
		return true
	default:
		return false
	}
}

// Define all possible phase transitions.
//
// The keys are the "from" states and the values enumerate the
// possible "to" states.
var validTransitions = map[Phase][]Phase{
	QUIESCE: {IMPORT, ABORT},
	// VALIDATION is the new-path successor of IMPORT: PROCESSRELATIONS is
	// retired (it has no actions attached and no persisted lookup row, see
	// PhasePersistedID). The PROCESSRELATIONS edges are retained transitionally
	// so the legacy migrationmaster worker, which still emits PROCESSRELATIONS,
	// keeps a non-terminal phase to walk through until it is rewritten to drive
	// the de-stubbed domain service directly. New-path callers go
	// IMPORT -> VALIDATION and never set PROCESSRELATIONS.
	IMPORT:           {VALIDATION, PROCESSRELATIONS, ABORT},
	PROCESSRELATIONS: {VALIDATION, ABORT},
	VALIDATION:       {SUCCESS, ABORT},
	SUCCESS:          {LOGTRANSFER},
	LOGTRANSFER:      {REAP},
	REAP:             {DONE, REAPFAILED},
	ABORT:            {ABORTDONE},
}

var terminalPhases []Phase

func init() {
	// Compute the terminal phases.
	for p := 0; p <= len(phaseNames); p++ {
		phase := Phase(p)
		if _, exists := validTransitions[phase]; !exists {
			terminalPhases = append(terminalPhases, phase)
		}
	}
}

// ParsePhase converts a string model migration phase name
// to its constant value.
func ParsePhase(target string) (Phase, bool) {
	for p, name := range phaseNames {
		if target == name {
			return Phase(p), true
		}
	}
	return UNKNOWN, false
}

// PhasePersistedID returns the primary key under which the phase is stored in
// the model_migration_phase lookup table. Phases that are never persisted
// (UNKNOWN, NONE and the retired PROCESSRELATIONS) return ErrPhaseNotPersisted.
// Callers that persist or read the current migration phase must use this
// conversion rather than the enum ordinal or Phase.String().
func PhasePersistedID(p Phase) (int, error) {
	id, ok := persistedPhaseIDs[p]
	if !ok {
		return 0, errors.Errorf("converting phase %q to persisted id: %w", p, ErrPhaseNotPersisted)
	}
	return id, nil
}

// PhaseFromPersistedID returns the phase stored under the given
// model_migration_phase primary key. Ids that have no corresponding phase
// return ErrPhaseNotPersisted.
func PhaseFromPersistedID(id int) (Phase, error) {
	for p, pid := range persistedPhaseIDs {
		if pid == id {
			return p, nil
		}
	}
	return UNKNOWN, errors.Errorf("converting persisted id %d to phase: %w", id, ErrPhaseNotPersisted)
}
