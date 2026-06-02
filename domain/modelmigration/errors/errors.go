// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// ErrMigrationAlreadyActive indicates that the model already has an
	// in-progress export migration (a row in model_migration_export with a
	// NULL end_time). Only one active export per model is permitted.
	ErrMigrationAlreadyActive = errors.ConstError("model already has an active migration")

	// ErrMigrationNotFound indicates that no active export migration exists for
	// the model.
	ErrMigrationNotFound = errors.ConstError("migration not found")

	// ErrPhaseTransitionInvalid indicates that a requested migration phase
	// transition is not permitted from the migration's current phase, or that
	// the migration's phase changed concurrently (optimistic-lock conflict).
	ErrPhaseTransitionInvalid = errors.ConstError("invalid migration phase transition")

	// ErrExternalControllerConflict indicates that an external controller with
	// the same UUID already exists with different connection details (addresses
	// or CA certificate). The migration must not silently overwrite the live
	// record.
	ErrExternalControllerConflict = errors.ConstError("external controller already exists with different details")
)
