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

	// ErrConflictingMinionReport indicates that a minion submitted a report for
	// a (migration, phase, entity) triple that already has a report with a
	// different success value. Reports are idempotent for an identical value but
	// must never silently overwrite a conflicting one.
	ErrConflictingMinionReport = errors.ConstError("conflicting minion report")

	// ErrSourceControllerNoAPIAddresses indicates the source controller exposes
	// no usable API addresses for a target controller to dial back during model
	// activation, so the migration cannot complete.
	ErrSourceControllerNoAPIAddresses = errors.ConstError("source controller has no usable API addresses")

	// ErrImportNotFound indicates that no target-side import
	// (model_migration_import row) exists for the model.
	ErrImportNotFound = errors.ConstError("import not found")

	// ErrImportClaimExists indicates that a target-side import claim
	// (model_migration_import row) already exists for the model. The caller
	// should read the existing claim's phase to report the correct
	// AlreadyExists wording (cleanup/activation in progress, or a duplicate
	// importing claim).
	ErrImportClaimExists = errors.ConstError("import claim already exists")

	// ErrImportNotImporting indicates that a target-side import claim exists
	// for the model but has moved past the importing phase (activating or
	// aborting), so a controller-data write group must stop without writing.
	ErrImportNotImporting = errors.ConstError("import claim is not in the importing phase")

	// ErrExternalControllerMismatch indicates that a third-party external
	// controller or external model referenced by a v8 import envelope already
	// exists on the target with different connection details. A v8 import
	// never overwrites live CMR connection data.
	ErrExternalControllerMismatch = errors.ConstError("external controller details do not match")

	// ErrActivationAborting indicates that an activation was attempted on an
	// import claim that is in the aborting phase. Activation is not possible
	// once abort has begun; the caller must wait for abort to complete and
	// retry the full import from the source controller.
	ErrActivationAborting = errors.ConstError("cannot activate import: cleanup already in progress")

	// ErrModelNotRedirected indicates that no completed migration redirect
	// exists for the model. A staged-but-incomplete redirect (completed_at IS
	// NULL) is not yet active and returns the same error.
	ErrModelNotRedirected = errors.ConstError("model not redirected")
)
