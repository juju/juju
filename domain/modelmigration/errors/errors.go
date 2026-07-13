// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// ErrMigrationAlreadyActive indicates that the model already has an
	// in-progress export migration (a row in model_migration_export whose
	// current phase is not terminal). Only one active export per model is
	// permitted.
	ErrMigrationAlreadyActive = errors.ConstError("model already has an active migration")

	// ErrMigrationNotFound indicates that no active export migration exists for
	// the model.
	ErrMigrationNotFound = errors.ConstError("migration not found")

	// ErrPhaseTransitionInvalid indicates that a requested migration phase
	// transition is not permitted from the migration's current phase, or that
	// the migration's phase changed concurrently (optimistic-lock conflict).
	ErrPhaseTransitionInvalid = errors.ConstError("invalid migration phase transition")

	// ErrExternalControllerConflict indicates that an external controller record
	// cannot be reconciled with supplied migration details.
	ErrExternalControllerConflict = errors.ConstError("external controller already exists with different details")

	// ErrConflictingMinionReport indicates that a minion submitted a report for
	// a (migration, phase, entity) triple that already has a report with a
	// different success value. Reports are idempotent for an identical value but
	// must never silently overwrite a conflicting one.
	ErrConflictingMinionReport = errors.ConstError("conflicting minion report")

	// ErrSourceControllerNoAPIAddresses indicates the source controller exposes
	// no usable API addresses for a target controller to dial back during model
	// activation, so the migration cannot complete.
	ErrSourceControllerNoAPIAddresses = errors.ConstError("source controller has no usable API addresses")

	// ErrModelNotRedirected indicates that no completed migration redirect
	// exists for the model. A staged-but-incomplete redirect (completed_at IS
	// NULL) is not yet active and returns the same error.
	ErrModelNotRedirected = errors.ConstError("model not redirected")
)
