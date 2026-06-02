// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State represents the access method for interacting with the controller
// database during model migration.
type State struct {
	*domain.StateBase
	clock clock.Clock
}

// New creates a new [State].
func New(factory coredatabase.TxnRunnerFactory, clock clock.Clock) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
	}
}

// DeleteModelImportingStatus removes the entry from the model_migration_import
// table in the controller database, indicating that the model import has
// completed or been aborted.
func (s *State) DeleteModelImportingStatus(ctx context.Context, modelUUID string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	mUUID := modelUUIDArg{ModelUUID: modelUUID}

	stmt, err := s.Prepare(`
DELETE FROM model_migration_import
WHERE model_uuid = $modelUUIDArg.model_uuid
	`, mUUID)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, mUUID).Run(); err != nil {
			return errors.Errorf("deleting importing status for model %q: %w", modelUUID, err)
		}
		return nil
	})
}

// GetControllerTargetVersion returns the target controller version in use by the
// cluster.
func (s *State) GetControllerTargetVersion(ctx context.Context) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var versionValue controllerTargetVersion
	stmt, err := s.Prepare(`
SELECT &controllerTargetVersion.*
FROM   controller
`,
		versionValue)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&versionValue)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("no controller target version has been previously set")
		}
		return err
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return versionValue.TargetVersion, nil
}

// NamespaceForWatchExport returns the changestream namespace that fires when an
// export migration starts or changes active/terminal state.
func (s *State) NamespaceForWatchExport() string {
	return "model_migration_export"
}

// NamespaceForWatchPhase returns the changestream namespace that fires on each
// export migration phase transition, keyed by model UUID.
func (s *State) NamespaceForWatchPhase() string {
	return "model_migration_export_phase"
}

// NamespaceForWatchMinionSync returns the changestream namespace that fires
// when a minion sync report changes, keyed by migration UUID.
func (s *State) NamespaceForWatchMinionSync() string {
	return "model_migration_export_minion_sync"
}

// InsertExport records a new export migration attempt for a model. It ensures
// the target external_controller row exists, inserts the
// model_migration_export row in the QUIESCE phase with its companion
// model_migration_export_target_auth credentials, and seeds the phase history.
//
// If the model already has an active export migration, the error is reported as
// [modelmigrationerrors.ErrMigrationAlreadyActive].
func (s *State) InsertExport(ctx context.Context, spec modelmigrationinternal.MigrationSpec) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	quiesceID, err := migration.PhasePersistedID(migration.QUIESCE)
	if err != nil {
		return errors.Capture(err)
	}

	now := s.clock.Now().UTC()
	terminalIDs, err := terminalPhaseIDs()
	if err != nil {
		return errors.Capture(err)
	}
	mUUID := modelUUIDArg{ModelUUID: spec.ModelUUID}

	export := migrationExport{
		UUID:                 spec.MigrationUUID,
		ModelUUID:            spec.ModelUUID,
		TargetControllerUUID: spec.TargetControllerUUID,
		CurrentPhaseID:       quiesceID,
		UpdatedAt:            now,
		StartTime:            now,
	}
	auth := migrationTargetAuth{
		MigrationUUID:          spec.MigrationUUID,
		ExternalControllerUUID: spec.TargetControllerUUID,
		TargetUser:             spec.TargetUser,
		TargetMacaroons:        spec.TargetMacaroons,
		TargetToken:            spec.TargetToken,
		TargetSkipUserChecks:   spec.TargetSkipUserChecks,
	}
	phaseEntry := migrationPhaseEntry{
		MigrationUUID: spec.MigrationUUID,
		ModelUUID:     spec.ModelUUID,
		PhaseID:       quiesceID,
		ChangedAt:     now,
	}

	insertExportStmt, err := s.Prepare(`
INSERT INTO model_migration_export (*) VALUES ($migrationExport.*)
`, export)
	if err != nil {
		return errors.Capture(err)
	}
	insertAuthStmt, err := s.Prepare(`
INSERT INTO model_migration_export_target_auth (*) VALUES ($migrationTargetAuth.*)
`, auth)
	if err != nil {
		return errors.Capture(err)
	}
	insertPhaseStmt, err := s.Prepare(`
INSERT INTO model_migration_export_phase (*) VALUES ($migrationPhaseEntry.*)
`, phaseEntry)
	if err != nil {
		return errors.Capture(err)
	}
	countActiveStmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   model_migration_export
WHERE  model_uuid = $modelUUIDArg.model_uuid
AND    current_phase_id NOT IN (
       $terminalPhaseIDArgs.reap_failed_id,
       $terminalPhaseIDArgs.done_id,
       $terminalPhaseIDArgs.abort_done_id)
`, mUUID, terminalIDs, countResult{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.ensureExternalControllerSaved(ctx, tx, externalControllerInfo{
			UUID:   spec.TargetControllerUUID,
			Alias:  spec.TargetControllerAlias,
			CACert: spec.TargetCACert,
		}, spec.TargetAddrs); err != nil {
			return errors.Capture(err)
		}

		var activeCount countResult
		if err := tx.Query(ctx, countActiveStmt, mUUID, terminalIDs).Get(&activeCount); err != nil {
			return errors.Errorf("counting active exports for model %q: %w", spec.ModelUUID, err)
		}
		if activeCount.Count > 0 {
			return errors.Errorf(
				"model %q already has an active migration: %w",
				spec.ModelUUID, modelmigrationerrors.ErrMigrationAlreadyActive,
			)
		}

		if err := tx.Query(ctx, insertExportStmt, export).Run(); err != nil {
			if database.IsErrConstraintUnique(err) {
				return errors.Errorf(
					"model %q already has an active migration: %w",
					spec.ModelUUID, modelmigrationerrors.ErrMigrationAlreadyActive,
				)
			}
			return errors.Errorf("inserting export migration: %w", err)
		}
		if err := tx.Query(ctx, insertAuthStmt, auth).Run(); err != nil {
			return errors.Errorf("inserting export target auth: %w", err)
		}
		if err := tx.Query(ctx, insertPhaseStmt, phaseEntry).Run(); err != nil {
			return errors.Errorf("inserting export phase history: %w", err)
		}
		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// ensureExternalControllerSaved inserts the external controller and its
// addresses if absent, no-ops if an identical record already exists, and
// updates mutable connection details when they differ. External controller
// addresses and CA certificates may change in normal CMR redirect flows, so
// source migrations must preserve 3.6's ability to refresh this shared record.
func (s *State) ensureExternalControllerSaved(
	ctx context.Context, tx *sqlair.TX,
	info externalControllerInfo,
	addrs []modelmigrationinternal.ExternalControllerAddress,
) error {
	ctrlUUID := entityUUID{UUID: info.UUID}

	selectCtrlStmt, err := s.Prepare(`
SELECT &externalControllerInfo.*
FROM   external_controller
WHERE  uuid = $entityUUID.uuid
`, ctrlUUID, externalControllerInfo{})
	if err != nil {
		return errors.Capture(err)
	}
	selectAddrsStmt, err := s.Prepare(`
SELECT &addressValue.address
FROM   external_controller_address
WHERE  controller_uuid = $entityUUID.uuid
`, ctrlUUID, addressValue{})
	if err != nil {
		return errors.Capture(err)
	}

	var existing externalControllerInfo
	err = tx.Query(ctx, selectCtrlStmt, ctrlUUID).Get(&existing)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("looking up external controller %q: %w", info.UUID, err)
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		return s.insertExternalController(ctx, tx, info, addrs)
	}

	var existingAddrs []addressValue
	if err := tx.Query(ctx, selectAddrsStmt, ctrlUUID).GetAll(&existingAddrs); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("looking up external controller %q addresses: %w", info.UUID, err)
	}
	if existing.Alias == info.Alias &&
		existing.CACert == info.CACert &&
		addressesMatch(existingAddrs, addrs) {
		return nil
	}
	return s.updateExternalController(ctx, tx, info, addrs)
}

// insertExternalController inserts a new external_controller row together with
// its addresses.
func (s *State) insertExternalController(
	ctx context.Context, tx *sqlair.TX,
	info externalControllerInfo,
	addrs []modelmigrationinternal.ExternalControllerAddress,
) error {
	insertCtrlStmt, err := s.Prepare(`
INSERT INTO external_controller (*) VALUES ($externalControllerInfo.*)
`, info)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, insertCtrlStmt, info).Run(); err != nil {
		return errors.Errorf("inserting external controller %q: %w", info.UUID, err)
	}
	return s.insertExternalControllerAddresses(ctx, tx, info.UUID, addrs)
}

func (s *State) updateExternalController(
	ctx context.Context, tx *sqlair.TX,
	info externalControllerInfo,
	addrs []modelmigrationinternal.ExternalControllerAddress,
) error {
	updateCtrlStmt, err := s.Prepare(`
UPDATE external_controller
SET    alias = $externalControllerInfo.alias,
       ca_cert = $externalControllerInfo.ca_cert
WHERE  uuid = $externalControllerInfo.uuid
`, info)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, updateCtrlStmt, info).Run(); err != nil {
		return errors.Errorf("updating external controller %q: %w", info.UUID, err)
	}

	ctrlUUID := entityUUID{UUID: info.UUID}
	deleteAddrsStmt, err := s.Prepare(`
DELETE FROM external_controller_address
WHERE  controller_uuid = $entityUUID.uuid
`, ctrlUUID)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteAddrsStmt, ctrlUUID).Run(); err != nil {
		return errors.Errorf("deleting external controller %q addresses: %w", info.UUID, err)
	}
	return s.insertExternalControllerAddresses(ctx, tx, info.UUID, addrs)
}

func (s *State) insertExternalControllerAddresses(
	ctx context.Context, tx *sqlair.TX,
	controllerUUID string,
	addrs []modelmigrationinternal.ExternalControllerAddress,
) error {
	if len(addrs) == 0 {
		return nil
	}

	insertAddrStmt, err := s.Prepare(`
INSERT INTO external_controller_address (*) VALUES ($externalControllerAddress.*)
`, externalControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	for _, addr := range addrs {
		if addr.UUID == "" {
			return errors.Errorf("external controller %q address %q is missing a UUID", controllerUUID, addr.Address)
		}
		row := externalControllerAddress{
			UUID:           addr.UUID,
			ControllerUUID: controllerUUID,
			Address:        addr.Address,
		}
		if err := tx.Query(ctx, insertAddrStmt, row).Run(); err != nil {
			return errors.Errorf("inserting external controller %q address: %w", controllerUUID, err)
		}
	}
	return nil
}

// GetActiveExport returns the active export migration for the given
// model, including the reconstructed target connection details. If no active
// export exists [modelmigrationerrors.ErrMigrationNotFound] is returned.
func (s *State) GetActiveExport(ctx context.Context, modelUUID string) (modelmigrationinternal.Migration, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return modelmigrationinternal.Migration{}, errors.Capture(err)
	}

	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	terminalIDs, err := terminalPhaseIDs()
	if err != nil {
		return modelmigrationinternal.Migration{}, errors.Capture(err)
	}

	selectExportStmt, err := s.Prepare(`
SELECT &migrationExport.*
FROM   model_migration_export
WHERE  model_uuid = $modelUUIDArg.model_uuid
AND    current_phase_id NOT IN (
       $terminalPhaseIDArgs.reap_failed_id,
       $terminalPhaseIDArgs.done_id,
       $terminalPhaseIDArgs.abort_done_id)
`, mUUID, terminalIDs, migrationExport{})
	if err != nil {
		return modelmigrationinternal.Migration{}, errors.Capture(err)
	}

	var (
		export migrationExport
		auth   migrationTargetAuth
		ctrl   externalControllerInfo
		addrs  []addressValue
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		export = migrationExport{}
		auth = migrationTargetAuth{}
		ctrl = externalControllerInfo{}
		addrs = nil

		err := tx.Query(ctx, selectExportStmt, mUUID, terminalIDs).Get(&export)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("no active migration for model %q: %w", modelUUID, modelmigrationerrors.ErrMigrationNotFound)
		} else if err != nil {
			return errors.Errorf("querying active export for model %q: %w", modelUUID, err)
		}

		migUUID := migrationUUIDArg{MigrationUUID: export.UUID}
		selectAuthStmt, err := s.Prepare(`
SELECT &migrationTargetAuth.*
FROM   model_migration_export_target_auth
WHERE  migration_uuid = $migrationUUIDArg.migration_uuid
`, migUUID, migrationTargetAuth{})
		if err != nil {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, selectAuthStmt, migUUID).Get(&auth); err != nil &&
			!errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying target auth for migration %q: %w", export.UUID, err)
		}

		ctrlUUID := entityUUID{UUID: export.TargetControllerUUID}
		selectCtrlStmt, err := s.Prepare(`
SELECT &externalControllerInfo.*
FROM   external_controller
WHERE  uuid = $entityUUID.uuid
`, ctrlUUID, externalControllerInfo{})
		if err != nil {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, selectCtrlStmt, ctrlUUID).Get(&ctrl); err != nil &&
			!errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying target controller %q: %w", export.TargetControllerUUID, err)
		}

		selectAddrsStmt, err := s.Prepare(`
SELECT &addressValue.address
FROM   external_controller_address
WHERE  controller_uuid = $entityUUID.uuid
`, ctrlUUID, addressValue{})
		if err != nil {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, selectAddrsStmt, ctrlUUID).GetAll(&addrs); err != nil &&
			!errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying target controller %q addresses: %w", export.TargetControllerUUID, err)
		}
		return nil
	})
	if err != nil {
		return modelmigrationinternal.Migration{}, errors.Capture(err)
	}

	phase, err := migration.PhaseFromPersistedID(export.CurrentPhaseID)
	if err != nil {
		return modelmigrationinternal.Migration{}, errors.Capture(err)
	}

	addresses := make([]string, len(addrs))
	for i, a := range addrs {
		addresses[i] = a.Address
	}

	return modelmigrationinternal.Migration{
		UUID:             export.UUID,
		Phase:            phase,
		PhaseChangedTime: export.UpdatedAt,
		Target: modelmigrationinternal.TargetInfo{
			ControllerUUID:  export.TargetControllerUUID,
			ControllerAlias: ctrl.Alias,
			Addrs:           addresses,
			CACert:          ctrl.CACert,
			User:            auth.TargetUser,
			Macaroons:       auth.TargetMacaroons,
			Token:           auth.TargetToken,
			SkipUserChecks:  auth.TargetSkipUserChecks,
		},
	}, nil
}

// SetPhase transitions the export migration to a new phase. The transition is
// validated against [migration.Phase.CanTransitionTo] inside the transaction
// and applied with optimistic locking on the previously-observed phase, so a
// concurrent caller cannot race two callers into a skipped or doubled phase.
//
// An invalid transition or a lost optimistic-lock race returns
// [modelmigrationerrors.ErrPhaseTransitionInvalid].
func (s *State) SetPhase(ctx context.Context, migrationUUID string, newPhase migration.Phase) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	newPhaseID, err := migration.PhasePersistedID(newPhase)
	if err != nil {
		return errors.Errorf("converting phase %q: %w", newPhase, err)
	}

	terminalIDs, err := terminalPhaseIDs()
	if err != nil {
		return errors.Capture(err)
	}
	migUUID := entityUUID{UUID: migrationUUID}
	selectPhaseStmt, err := s.Prepare(`
SELECT &currentPhase.*
FROM   model_migration_export
WHERE  uuid = $entityUUID.uuid
AND    current_phase_id NOT IN (
       $terminalPhaseIDArgs.reap_failed_id,
       $terminalPhaseIDArgs.done_id,
       $terminalPhaseIDArgs.abort_done_id)
`, migUUID, terminalIDs, currentPhase{})
	if err != nil {
		return errors.Capture(err)
	}

	updateStmt, err := s.Prepare(`
UPDATE model_migration_export
SET    current_phase_id = $phaseUpdate.new_phase_id,
       updated_at = $phaseUpdate.updated_at
WHERE  uuid = $phaseUpdate.uuid
AND    current_phase_id = $phaseUpdate.expected_phase_id
`, phaseUpdate{})
	if err != nil {
		return errors.Capture(err)
	}

	now := s.clock.Now().UTC()
	insertPhaseStmt, err := s.Prepare(`
INSERT INTO model_migration_export_phase (*) VALUES ($migrationPhaseEntry.*)
`, migrationPhaseEntry{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var cur currentPhase
		err := tx.Query(ctx, selectPhaseStmt, migUUID, terminalIDs).Get(&cur)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("no active migration %q: %w", migrationUUID, modelmigrationerrors.ErrMigrationNotFound)
		} else if err != nil {
			return errors.Errorf("reading current phase for migration %q: %w", migrationUUID, err)
		}

		curPhase, err := migration.PhaseFromPersistedID(cur.CurrentPhaseID)
		if err != nil {
			return errors.Capture(err)
		}
		if curPhase == newPhase {
			// Idempotent no-op: already in the requested phase.
			return nil
		}
		// This service-level invariant is deliberately enforced here too:
		// read, validation, and optimistic write must be one transaction.
		if !curPhase.CanTransitionTo(newPhase) {
			return errors.Errorf(
				"cannot transition migration %q from %q to %q: %w",
				migrationUUID, curPhase, newPhase, modelmigrationerrors.ErrPhaseTransitionInvalid,
			)
		}

		update := phaseUpdate{
			UUID:            migrationUUID,
			NewPhaseID:      newPhaseID,
			ExpectedPhaseID: cur.CurrentPhaseID,
			UpdatedAt:       now,
		}
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, updateStmt, update).Get(&outcome); err != nil {
			return errors.Errorf("updating phase for migration %q: %w", migrationUUID, err)
		}
		affected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if affected != 1 {
			return errors.Errorf(
				"migration %q phase changed concurrently: %w",
				migrationUUID, modelmigrationerrors.ErrPhaseTransitionInvalid,
			)
		}

		phaseEntry := migrationPhaseEntry{
			MigrationUUID: migrationUUID,
			ModelUUID:     cur.ModelUUID,
			PhaseID:       newPhaseID,
			ChangedAt:     now,
		}
		if err := tx.Query(ctx, insertPhaseStmt, phaseEntry).Run(); err != nil {
			return errors.Errorf("recording phase history for migration %q: %w", migrationUUID, err)
		}
		return nil
	})
}

// SetStatusMessage records the current free-form status message for an export
// migration.
func (s *State) SetStatusMessage(ctx context.Context, migrationUUID, message string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	status := migrationStatus{
		MigrationUUID: migrationUUID,
		Message:       message,
		RecordedAt:    s.clock.Now().UTC(),
	}
	countStmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   model_migration_export_status
WHERE  migration_uuid = $migrationStatus.migration_uuid
`, status, countResult{})
	if err != nil {
		return errors.Capture(err)
	}
	insertStmt, err := s.Prepare(`
INSERT INTO model_migration_export_status (*) VALUES ($migrationStatus.*)
`, status)
	if err != nil {
		return errors.Capture(err)
	}
	updateStmt, err := s.Prepare(`
UPDATE model_migration_export_status
SET    message = $migrationStatus.message,
       recorded_at = $migrationStatus.recorded_at
WHERE  migration_uuid = $migrationStatus.migration_uuid
`, status)
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var count countResult
		if err := tx.Query(ctx, countStmt, status).Get(&count); err != nil {
			return errors.Errorf("looking up status message for migration %q: %w", migrationUUID, err)
		}
		if count.Count == 0 {
			if err := tx.Query(ctx, insertStmt, status).Run(); err != nil {
				return errors.Errorf("inserting status message for migration %q: %w", migrationUUID, err)
			}
			return nil
		}
		if err := tx.Query(ctx, updateStmt, status).Run(); err != nil {
			return errors.Errorf("updating status message for migration %q: %w", migrationUUID, err)
		}
		return nil
	})
}

// InsertMinionReport records a phase report from a single minion agent. The
// (migration, phase, entity) triple is unique. A minion may resubmit a report
// for the same phase: an identical success value is an idempotent no-op, but a
// conflicting one is rejected with [modelmigrationerrors.ErrConflictingMinionReport]
// rather than silently overwriting the recorded result. This mirrors the legacy
// (3.6) behaviour and preserves the migration master's view of each minion's
// outcome for a phase.
func (s *State) InsertMinionReport(
	ctx context.Context, migrationUUID string, phase migration.Phase, entityKey string, success bool,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	phaseID, err := migration.PhasePersistedID(phase)
	if err != nil {
		return errors.Errorf("converting phase %q: %w", phase, err)
	}

	report := migrationMinionSync{
		MigrationUUID: migrationUUID,
		PhaseID:       phaseID,
		EntityKey:     entityKey,
		Success:       success,
		ReportedAt:    s.clock.Now().UTC(),
	}
	selectStmt, err := s.Prepare(`
SELECT &minionReportRow.*
FROM   model_migration_export_minion_sync
WHERE  migration_uuid = $migrationMinionSync.migration_uuid
AND    phase_id = $migrationMinionSync.phase_id
AND    entity_key = $migrationMinionSync.entity_key
`, report, minionReportRow{})
	if err != nil {
		return errors.Capture(err)
	}
	insertStmt, err := s.Prepare(`
INSERT INTO model_migration_export_minion_sync (*) VALUES ($migrationMinionSync.*)
`, report)
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var existing minionReportRow
		err := tx.Query(ctx, selectStmt, report).Get(&existing)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("reading existing minion report for migration %q: %w", migrationUUID, err)
		}
		if err == nil {
			if existing.Success != success {
				return errors.Errorf(
					"minion report for migration %q phase %q entity %q already recorded with a different result: %w",
					migrationUUID, phase, entityKey, modelmigrationerrors.ErrConflictingMinionReport,
				)
			}
			return nil
		}

		if err := tx.Query(ctx, insertStmt, report).Run(); err == nil {
			return nil
		} else if !database.IsErrConstraintUnique(err) {
			return errors.Errorf("inserting minion report for migration %q: %w", migrationUUID, err)
		}

		// Another transaction inserted the row after our read; compare the
		// recorded value so concurrent identical resubmits stay idempotent.
		if err := tx.Query(ctx, selectStmt, report).Get(&existing); err != nil {
			return errors.Errorf("reading concurrently inserted minion report for migration %q: %w", migrationUUID, err)
		}
		if existing.Success != success {
			return errors.Errorf(
				"minion report for migration %q phase %q entity %q already recorded with a different result: %w",
				migrationUUID, phase, entityKey, modelmigrationerrors.ErrConflictingMinionReport,
			)
		}
		return nil
	})
}

// AggregateMinionReports returns the succeeded and failed entity keys reported
// by minions for the given migration and phase.
func (s *State) AggregateMinionReports(
	ctx context.Context, migrationUUID string, phase migration.Phase,
) (modelmigrationinternal.MinionReports, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return modelmigrationinternal.MinionReports{}, errors.Capture(err)
	}

	phaseID, err := migration.PhasePersistedID(phase)
	if err != nil {
		return modelmigrationinternal.MinionReports{}, errors.Errorf("converting phase %q: %w", phase, err)
	}

	migUUID := migrationUUIDArg{MigrationUUID: migrationUUID}
	phaseArg := phaseIDArg{PhaseID: phaseID}
	stmt, err := s.Prepare(`
SELECT &minionReportRow.*
FROM   model_migration_export_minion_sync
WHERE  migration_uuid = $migrationUUIDArg.migration_uuid
AND    phase_id = $phaseIDArg.phase_id
`, migUUID, phaseArg, minionReportRow{})
	if err != nil {
		return modelmigrationinternal.MinionReports{}, errors.Capture(err)
	}

	var rows []minionReportRow
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt, migUUID, phaseArg).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying minion reports for migration %q: %w", migrationUUID, err)
		}
		return nil
	})
	if err != nil {
		return modelmigrationinternal.MinionReports{}, errors.Capture(err)
	}

	reports := modelmigrationinternal.MinionReports{Phase: phase}
	for _, row := range rows {
		if row.Success {
			reports.Succeeded = append(reports.Succeeded, row.EntityKey)
		} else {
			reports.Failed = append(reports.Failed, row.EntityKey)
		}
	}
	return reports, nil
}

// MarkExportEnded marks the export migration as ended by recording its terminal
// phase and appending the terminal phase to the phase history. Once ended, the
// model no longer has an active export migration.
func (s *State) MarkExportEnded(ctx context.Context, migrationUUID string, terminalPhase migration.Phase) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	phaseID, err := migration.PhasePersistedID(terminalPhase)
	if err != nil {
		return errors.Errorf("converting phase %q: %w", terminalPhase, err)
	}
	if !terminalPhase.IsTerminal() {
		return errors.Errorf(
			"cannot end migration %q with non-terminal phase %q: %w",
			migrationUUID, terminalPhase, modelmigrationerrors.ErrPhaseTransitionInvalid,
		)
	}
	terminalIDs, err := terminalPhaseIDs()
	if err != nil {
		return errors.Capture(err)
	}

	now := s.clock.Now().UTC()
	end := endExport{
		UUID:      migrationUUID,
		PhaseID:   phaseID,
		UpdatedAt: now,
	}
	selectExportStmt, err := s.Prepare(`
SELECT &currentPhase.*
FROM   model_migration_export
WHERE  uuid = $endExport.uuid
AND    current_phase_id NOT IN (
       $terminalPhaseIDArgs.reap_failed_id,
       $terminalPhaseIDArgs.done_id,
       $terminalPhaseIDArgs.abort_done_id)
`, end, terminalIDs, currentPhase{})
	if err != nil {
		return errors.Capture(err)
	}
	updateStmt, err := s.Prepare(`
UPDATE model_migration_export
SET    current_phase_id = $endExport.current_phase_id,
       updated_at = $endExport.updated_at
WHERE  uuid = $endExport.uuid
AND    current_phase_id NOT IN (
       $terminalPhaseIDArgs.reap_failed_id,
       $terminalPhaseIDArgs.done_id,
       $terminalPhaseIDArgs.abort_done_id)
`, end, terminalIDs)
	if err != nil {
		return errors.Capture(err)
	}

	insertPhaseStmt, err := s.Prepare(`
INSERT INTO model_migration_export_phase (*) VALUES ($migrationPhaseEntry.*)
`, migrationPhaseEntry{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var cur currentPhase
		if err := tx.Query(ctx, selectExportStmt, end, terminalIDs).Get(&cur); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("no active migration %q: %w", migrationUUID, modelmigrationerrors.ErrMigrationNotFound)
			}
			return errors.Errorf("reading export migration %q: %w", migrationUUID, err)
		}

		var outcome sqlair.Outcome
		if err := tx.Query(ctx, updateStmt, end, terminalIDs).Get(&outcome); err != nil {
			return errors.Errorf("ending migration %q: %w", migrationUUID, err)
		}
		affected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if affected == 0 {
			return errors.Errorf("no active migration %q: %w", migrationUUID, modelmigrationerrors.ErrMigrationNotFound)
		}
		phaseEntry := migrationPhaseEntry{
			MigrationUUID: migrationUUID,
			ModelUUID:     cur.ModelUUID,
			PhaseID:       phaseID,
			ChangedAt:     now,
		}
		if err := tx.Query(ctx, insertPhaseStmt, phaseEntry).Run(); err != nil {
			return errors.Errorf("recording terminal phase for migration %q: %w", migrationUUID, err)
		}
		return nil
	})
}

// GetMigrationMode derives the migration mode for the model: exporting if it
// has an active export migration, importing if a target import claim exists,
// otherwise none.
func (s *State) GetMigrationMode(ctx context.Context, modelUUID string) (modelmigration.MigrationMode, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return modelmigration.MigrationModeNone, errors.Capture(err)
	}

	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	terminalIDs, err := terminalPhaseIDs()
	if err != nil {
		return modelmigration.MigrationModeNone, errors.Capture(err)
	}
	exportStmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   model_migration_export
WHERE  model_uuid = $modelUUIDArg.model_uuid
AND    current_phase_id NOT IN (
       $terminalPhaseIDArgs.reap_failed_id,
       $terminalPhaseIDArgs.done_id,
       $terminalPhaseIDArgs.abort_done_id)
`, mUUID, terminalIDs, countResult{})
	if err != nil {
		return modelmigration.MigrationModeNone, errors.Capture(err)
	}
	importStmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   model_migration_import
WHERE  model_uuid = $modelUUIDArg.model_uuid
`, mUUID, countResult{})
	if err != nil {
		return modelmigration.MigrationModeNone, errors.Capture(err)
	}

	var mode modelmigration.MigrationMode
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var exportCount countResult
		if err := tx.Query(ctx, exportStmt, mUUID, terminalIDs).Get(&exportCount); err != nil {
			return errors.Errorf("counting active exports for model %q: %w", modelUUID, err)
		}
		if exportCount.Count > 0 {
			mode = modelmigration.MigrationModeExporting
			return nil
		}
		var importCount countResult
		if err := tx.Query(ctx, importStmt, mUUID).Get(&importCount); err != nil {
			return errors.Errorf("counting imports for model %q: %w", modelUUID, err)
		}
		if importCount.Count > 0 {
			mode = modelmigration.MigrationModeImporting
			return nil
		}
		mode = modelmigration.MigrationModeNone
		return nil
	})
	if err != nil {
		return modelmigration.MigrationModeNone, errors.Capture(err)
	}
	return mode, nil
}

// addressesMatch reports whether the persisted addresses equal the supplied
// addresses, ignoring order.
func addressesMatch(existing []addressValue, supplied []modelmigrationinternal.ExternalControllerAddress) bool {
	if len(existing) != len(supplied) {
		return false
	}
	have := make(map[string]int, len(existing))
	for _, a := range existing {
		have[a.Address]++
	}
	for _, a := range supplied {
		if have[a.Address] == 0 {
			return false
		}
		have[a.Address]--
	}
	return true
}

func terminalPhaseIDs() (terminalPhaseIDArgs, error) {
	reapFailedID, err := migration.PhasePersistedID(migration.REAPFAILED)
	if err != nil {
		return terminalPhaseIDArgs{}, errors.Capture(err)
	}
	doneID, err := migration.PhasePersistedID(migration.DONE)
	if err != nil {
		return terminalPhaseIDArgs{}, errors.Capture(err)
	}
	abortDoneID, err := migration.PhasePersistedID(migration.ABORTDONE)
	if err != nil {
		return terminalPhaseIDArgs{}, errors.Capture(err)
	}
	return terminalPhaseIDArgs{
		ReapFailedID: reapFailedID,
		DoneID:       doneID,
		AbortDoneID:  abortDoneID,
	}, nil
}
