// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"

	coredatabase "github.com/juju/juju/core/database"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/cloudimagemetadata"
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

// GetActiveExportUUID returns the UUID of the active export migration for the
// given model. If no active export exists [modelmigrationerrors.ErrMigrationNotFound]
// is returned.
func (s *State) GetActiveExportUUID(ctx context.Context, modelUUID string) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	terminalIDs, err := terminalPhaseIDs()
	if err != nil {
		return "", errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT &entityUUID.uuid
FROM   model_migration_export
WHERE  model_uuid = $modelUUIDArg.model_uuid
AND    current_phase_id NOT IN (
       $terminalPhaseIDArgs.reap_failed_id,
       $terminalPhaseIDArgs.done_id,
       $terminalPhaseIDArgs.abort_done_id)
`, mUUID, terminalIDs, entityUUID{})
	if err != nil {
		return "", errors.Capture(err)
	}

	var result entityUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = entityUUID{}

		err := tx.Query(ctx, stmt, mUUID, terminalIDs).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("no active migration for model %q: %w", modelUUID, modelmigrationerrors.ErrMigrationNotFound)
		} else if err != nil {
			return errors.Errorf("querying active export for model %q: %w", modelUUID, err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return result.UUID, nil
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

type grantOnList []string
type uuidList []string
type nameList []string

// GetControllerModelInfo reads the controller-database records scoped to the
// given migrating model and returns them in target-portable semantic form.
// offerUUIDs are the model's hosted offer UUIDs, used to select offer-scoped
// permission rows; offererModels are the distinct (offerer controller, offerer
// model) pairs referenced by the model's remote applications, used to select
// the third-party external controllers. Both are read from the model database
// by the caller. All reads run in a single controller-database transaction.
func (s *State) GetControllerModelInfo(
	ctx context.Context,
	modelUUID string,
	offerUUIDs []string,
	offererModels []modelmigrationinternal.OffererModel,
) (modelmigration.ControllerModelInfo, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return modelmigration.ControllerModelInfo{}, errors.Capture(err)
	}

	var info modelmigration.ControllerModelInfo
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		info = modelmigration.ControllerModelInfo{}

		if info.ModelInfo, err = s.getModelIdentity(ctx, tx, modelUUID); err != nil {
			return errors.Capture(err)
		}
		if info.Permissions, err = s.getPermissions(ctx, tx, modelUUID, offerUUIDs); err != nil {
			return errors.Capture(err)
		}
		if info.ModelCredential, err = s.getModelCredential(ctx, tx, modelUUID); err != nil {
			return errors.Capture(err)
		}
		if info.AuthorizedKeys, err = s.getAuthorizedKeys(ctx, tx, modelUUID); err != nil {
			return errors.Capture(err)
		}
		names := modelUserNames(info.ModelInfo, info.Permissions, info.AuthorizedKeys)
		if info.Users, err = s.getUsers(ctx, tx, modelUUID, names); err != nil {
			return errors.Capture(err)
		}
		if info.SecretBackend, err = s.getModelSecretBackend(ctx, tx, modelUUID); err != nil {
			return errors.Capture(err)
		}
		if info.SecretBackendRefs, err = s.getSecretBackendRefs(ctx, tx, modelUUID); err != nil {
			return errors.Capture(err)
		}
		if info.Leaders, err = s.getApplicationLeadership(ctx, tx, modelUUID); err != nil {
			return errors.Capture(err)
		}
		if info.CloudImageMetadata, err = s.getCustomCloudImageMetadata(ctx, tx); err != nil {
			return errors.Capture(err)
		}
		if info.ExternalControllers, err = s.getExternalControllers(ctx, tx, offererModels); err != nil {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return modelmigration.ControllerModelInfo{}, errors.Capture(err)
	}

	return info, nil
}

// getModelIdentity reads the model's bootstrap identity with cloud, region,
// credential and life resolved to natural keys.
func (s *State) getModelIdentity(
	ctx context.Context, tx *sqlair.TX, modelUUID string,
) (modelmigration.ModelBootstrapInfo, error) {
	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	stmt, err := s.Prepare(`
SELECT m.uuid AS &modelIdentityRow.uuid,
       m.name AS &modelIdentityRow.name,
       m.qualifier AS &modelIdentityRow.qualifier,
       mt.type AS &modelIdentityRow.model_type,
       c.name AS &modelIdentityRow.cloud,
       cr.name AS &modelIdentityRow.cloud_region,
       cc.name AS &modelIdentityRow.credential_name,
       cco.name AS &modelIdentityRow.credential_owner,
       l.value AS &modelIdentityRow.life
FROM   model AS m
JOIN   model_type AS mt ON mt.id = m.model_type_id
JOIN   cloud AS c ON c.uuid = m.cloud_uuid
JOIN   life AS l ON l.id = m.life_id
LEFT JOIN cloud_region AS cr ON cr.uuid = m.cloud_region_uuid
LEFT JOIN cloud_credential AS cc ON cc.uuid = m.cloud_credential_uuid
LEFT JOIN user AS cco ON cco.uuid = cc.owner_uuid
WHERE  m.uuid = $modelUUIDArg.model_uuid
`, mUUID, modelIdentityRow{})
	if err != nil {
		return modelmigration.ModelBootstrapInfo{}, errors.Capture(err)
	}

	var identity modelIdentityRow
	if err := tx.Query(ctx, stmt, mUUID).Get(&identity); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return modelmigration.ModelBootstrapInfo{}, errors.Errorf("model %q not found", modelUUID)
		}
		return modelmigration.ModelBootstrapInfo{}, errors.Errorf("querying model identity: %w", err)
	}

	return modelmigration.ModelBootstrapInfo{
		UUID:            identity.UUID,
		Name:            identity.Name,
		Qualifier:       identity.Qualifier,
		Type:            identity.Type,
		Cloud:           identity.Cloud,
		CloudRegion:     derefString(identity.CloudRegion),
		CredentialName:  derefString(identity.CredentialName),
		CredentialOwner: derefString(identity.CredentialOwner),
		Life:            identity.Life,
	}, nil
}

// getPermissions reads the model permission grants and, when the model hosts
// offers, the offer permission grants in the same statement.
func (s *State) getPermissions(
	ctx context.Context, tx *sqlair.TX, modelUUID string, offerUUIDs []string,
) ([]modelmigration.ModelPermission, error) {
	mUUID := modelUUIDArg{ModelUUID: modelUUID}

	var (
		stmt *sqlair.Statement
		err  error
		args []any
	)
	if len(offerUUIDs) > 0 {
		stmt, err = s.Prepare(`
SELECT pot.type AS &permissionRow.object_type,
       p.grant_on AS &permissionRow.grant_on,
       u.name AS &permissionRow.subject_name,
       pat.type AS &permissionRow.access
FROM   permission AS p
JOIN   permission_object_type AS pot ON pot.id = p.object_type_id
JOIN   permission_access_type AS pat ON pat.id = p.access_type_id
JOIN   user AS u ON u.uuid = p.grant_to
WHERE  (pot.type = 'model' AND p.grant_on = $modelUUIDArg.model_uuid)
OR     (pot.type = 'offer' AND p.grant_on IN ($grantOnList[:]))
`, mUUID, permissionRow{}, grantOnList{})
		args = []any{mUUID, grantOnList(offerUUIDs)}
	} else {
		stmt, err = s.Prepare(`
SELECT pot.type AS &permissionRow.object_type,
       p.grant_on AS &permissionRow.grant_on,
       u.name AS &permissionRow.subject_name,
       pat.type AS &permissionRow.access
FROM   permission AS p
JOIN   permission_object_type AS pot ON pot.id = p.object_type_id
JOIN   permission_access_type AS pat ON pat.id = p.access_type_id
JOIN   user AS u ON u.uuid = p.grant_to
WHERE  pot.type = 'model' AND p.grant_on = $modelUUIDArg.model_uuid
`, mUUID, permissionRow{})
		args = []any{mUUID}
	}
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []permissionRow
	if err := getAll(ctx, tx, stmt, &rows, args...); err != nil {
		return nil, errors.Errorf("querying model permissions: %w", err)
	}

	perms := make([]modelmigration.ModelPermission, 0, len(rows))
	for _, p := range rows {
		perms = append(perms, modelmigration.ModelPermission{
			ObjectType:  p.ObjectType,
			GrantOn:     p.GrantOn,
			SubjectName: p.SubjectName,
			Access:      p.Access,
		})
	}
	return perms, nil
}

// getModelCredential reads the model's cloud credential by natural key
// together with its auth attributes, or nil when the model has no credential.
func (s *State) getModelCredential(
	ctx context.Context, tx *sqlair.TX, modelUUID string,
) (*modelmigration.ModelCloudCredential, error) {
	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	stmt, err := s.Prepare(`
SELECT vcc.cloud_name AS &credentialRow.cloud,
       vcc.owner_name AS &credentialRow.owner,
       vcc.name AS &credentialRow.name,
       vcc.auth_type AS &credentialRow.auth_type,
       vcc.revoked AS &credentialRow.revoked,
       vcc.invalid AS &credentialRow.invalid,
       vcc.invalid_reason AS &credentialRow.invalid_reason,
       cca."key" AS &credentialRow.attr_key,
       cca.value AS &credentialRow.attr_value
FROM   v_cloud_credential AS vcc
JOIN   model AS m ON m.cloud_credential_uuid = vcc.uuid
LEFT JOIN cloud_credential_attribute AS cca ON cca.cloud_credential_uuid = vcc.uuid
WHERE  m.uuid = $modelUUIDArg.model_uuid
`, mUUID, credentialRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []credentialRow
	if err := getAll(ctx, tx, stmt, &rows, mUUID); err != nil {
		return nil, errors.Errorf("querying model credential: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	first := rows[0]
	cred := &modelmigration.ModelCloudCredential{
		Cloud:         first.Cloud,
		Owner:         first.Owner,
		Name:          first.Name,
		AuthType:      first.AuthType,
		Revoked:       first.Revoked != nil && *first.Revoked,
		Invalid:       first.Invalid != nil && *first.Invalid,
		InvalidReason: derefString(first.InvalidReason),
	}
	for _, r := range rows {
		if r.AttrKey == nil {
			continue
		}
		if cred.Attributes == nil {
			cred.Attributes = make(map[string]string, len(rows))
		}
		cred.Attributes[*r.AttrKey] = derefString(r.AttrValue)
	}
	return cred, nil
}

// getAuthorizedKeys reads the SSH public keys authorised for the model, with
// their owners resolved to usernames.
func (s *State) getAuthorizedKeys(
	ctx context.Context, tx *sqlair.TX, modelUUID string,
) ([]modelmigration.ModelAuthorizedKey, error) {
	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	stmt, err := s.Prepare(`
SELECT u.name AS &authorizedKeyRow.username,
       vak.public_key AS &authorizedKeyRow.public_key
FROM   v_model_authorized_keys AS vak
JOIN   user AS u ON u.uuid = vak.user_uuid
WHERE  vak.model_uuid = $modelUUIDArg.model_uuid
`, mUUID, authorizedKeyRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []authorizedKeyRow
	if err := getAll(ctx, tx, stmt, &rows, mUUID); err != nil {
		return nil, errors.Errorf("querying authorized keys: %w", err)
	}

	keys := make([]modelmigration.ModelAuthorizedKey, 0, len(rows))
	for _, k := range rows {
		keys = append(keys, modelmigration.ModelAuthorizedKey{
			Username:  k.Username,
			PublicKey: k.PublicKey,
		})
	}
	return keys, nil
}

// getUsers reads the non-authentication profiles of the named users, with each
// user's last login against the model joined in. Removed users are ignored.
func (s *State) getUsers(
	ctx context.Context, tx *sqlair.TX, modelUUID string, names []string,
) ([]modelmigration.ModelUser, error) {
	if len(names) == 0 {
		return nil, nil
	}

	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	stmt, err := s.Prepare(`
SELECT u.name AS &userRow.name,
       u.display_name AS &userRow.display_name,
       cb.name AS &userRow.created_by,
       u.created_at AS &userRow.created_at,
       mll.time AS &userRow.last_login
FROM   user AS u
LEFT JOIN user AS cb ON cb.uuid = u.created_by_uuid
LEFT JOIN model_last_login AS mll
       ON mll.user_uuid = u.uuid AND mll.model_uuid = $modelUUIDArg.model_uuid
WHERE  u.removed = FALSE AND u.name IN ($nameList[:])
`, mUUID, userRow{}, nameList{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []userRow
	if err := getAll(ctx, tx, stmt, &rows, mUUID, nameList(names)); err != nil {
		return nil, errors.Errorf("querying model users: %w", err)
	}

	users := make([]modelmigration.ModelUser, 0, len(rows))
	for _, u := range rows {
		users = append(users, modelmigration.ModelUser{
			Name:        u.Name,
			DisplayName: derefString(u.DisplayName),
			CreatedBy:   derefString(u.CreatedBy),
			CreatedAt:   u.CreatedAt,
			LastLogin:   u.LastLogin,
		})
	}
	return users, nil
}

// getModelSecretBackend reads the secret backend the model uses, resolved to
// its name and type, or nil when the model uses the default backend.
func (s *State) getModelSecretBackend(
	ctx context.Context, tx *sqlair.TX, modelUUID string,
) (*modelmigration.ModelSecretBackend, error) {
	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	stmt, err := s.Prepare(`
SELECT sb.name AS &modelSecretBackendRow.name,
       sbt.type AS &modelSecretBackendRow.backend_type
FROM   model_secret_backend AS msb
JOIN   secret_backend AS sb ON sb.uuid = msb.secret_backend_uuid
JOIN   secret_backend_type AS sbt ON sbt.id = sb.backend_type_id
WHERE  msb.model_uuid = $modelUUIDArg.model_uuid
`, mUUID, modelSecretBackendRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var row modelSecretBackendRow
	if err := tx.Query(ctx, stmt, mUUID).Get(&row); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Errorf("querying model secret backend: %w", err)
	}
	return &modelmigration.ModelSecretBackend{
		Name:        row.Name,
		BackendType: row.BackendType,
	}, nil
}

// getSecretBackendRefs reads the mapping of the model's secret revisions to
// their backends, by backend name.
func (s *State) getSecretBackendRefs(
	ctx context.Context, tx *sqlair.TX, modelUUID string,
) ([]modelmigration.SecretBackendReference, error) {
	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	stmt, err := s.Prepare(`
SELECT sb.name AS &secretBackendRefRow.backend_name,
       sbr.secret_revision_uuid AS &secretBackendRefRow.secret_revision_uuid,
       COALESCE(sbr.secret_id, sbr.secret_revision_uuid) AS &secretBackendRefRow.secret_id
FROM   secret_backend_reference AS sbr
JOIN   secret_backend AS sb ON sb.uuid = sbr.secret_backend_uuid
WHERE  sbr.model_uuid = $modelUUIDArg.model_uuid
`, mUUID, secretBackendRefRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []secretBackendRefRow
	if err := getAll(ctx, tx, stmt, &rows, mUUID); err != nil {
		return nil, errors.Errorf("querying secret backend references: %w", err)
	}

	refs := make([]modelmigration.SecretBackendReference, 0, len(rows))
	for _, r := range rows {
		refs = append(refs, modelmigration.SecretBackendReference{
			BackendName:        r.BackendName,
			SecretRevisionUUID: r.SecretRevisionUUID,
			SecretID:           r.SecretID,
		})
	}
	return refs, nil
}

// getApplicationLeadership reads the application-leadership holders for the
// model. Leadership is the only lease state that travels with a migration:
// lease times are stale by import, pins are not migrated, and
// singular-controller leases name source controller nodes.
func (s *State) getApplicationLeadership(
	ctx context.Context, tx *sqlair.TX, modelUUID string,
) ([]modelmigration.ApplicationLeadership, error) {
	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	leaseType := leaseTypeArg{Type: corelease.ApplicationLeadershipNamespace}
	stmt, err := s.Prepare(`
SELECT l.name AS &leadershipRow.name,
       l.holder AS &leadershipRow.holder
FROM   lease AS l
JOIN   lease_type AS lt ON lt.id = l.lease_type_id
WHERE  l.model_uuid = $modelUUIDArg.model_uuid AND lt.type = $leaseTypeArg.type
`, mUUID, leaseType, leadershipRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []leadershipRow
	if err := getAll(ctx, tx, stmt, &rows, mUUID, leaseType); err != nil {
		return nil, errors.Errorf("querying application leadership: %w", err)
	}

	leaders := make([]modelmigration.ApplicationLeadership, 0, len(rows))
	for _, l := range rows {
		leaders = append(leaders, modelmigration.ApplicationLeadership{
			Application: derefString(l.Name),
			Leader:      derefString(l.Holder),
		})
	}
	return leaders, nil
}

// getCustomCloudImageMetadata reads the user-defined cloud image metadata rows
// that must be recreated on the target, with the architecture resolved to its
// name. Cached/provider-derived rows are not migrated.
func (s *State) getCustomCloudImageMetadata(
	ctx context.Context, tx *sqlair.TX,
) ([]modelmigration.CloudImageMetadata, error) {
	source := cloudImageMetadataSource{Source: cloudimagemetadata.CustomSource}
	stmt, err := s.Prepare(`
SELECT cim.stream AS &cloudImageMetadataRow.stream,
       cim.region AS &cloudImageMetadataRow.region,
       cim.version AS &cloudImageMetadataRow.version,
       a.name AS &cloudImageMetadataRow.arch,
       cim.virt_type AS &cloudImageMetadataRow.virt_type,
       cim.root_storage_type AS &cloudImageMetadataRow.root_storage_type,
       cim.root_storage_size AS &cloudImageMetadataRow.root_storage_size,
       cim.source AS &cloudImageMetadataRow.source,
       COALESCE(cim.priority, 0) AS &cloudImageMetadataRow.priority,
       cim.image_id AS &cloudImageMetadataRow.image_id,
       cim.created_at AS &cloudImageMetadataRow.created_at
FROM   cloud_image_metadata AS cim
JOIN   architecture AS a ON a.id = cim.architecture_id
WHERE  cim.source = $cloudImageMetadataSource.source
`, cloudImageMetadataRow{}, source)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []cloudImageMetadataRow
	if err := getAll(ctx, tx, stmt, &rows, source); err != nil {
		return nil, errors.Errorf("querying cloud image metadata: %w", err)
	}

	metadata := make([]modelmigration.CloudImageMetadata, 0, len(rows))
	for _, m := range rows {
		metadata = append(metadata, modelmigration.CloudImageMetadata{
			Stream:          m.Stream,
			Region:          m.Region,
			Version:         m.Version,
			Arch:            m.Arch,
			VirtType:        m.VirtType,
			RootStorageType: m.RootStorageType,
			RootStorageSize: m.RootStorageSize,
			Source:          m.Source,
			Priority:        m.Priority,
			ImageID:         m.ImageID,
			CreatedAt:       m.CreatedAt,
		})
	}
	return metadata, nil
}

// getExternalControllers reads the third-party external controllers selected
// by the model's offerer pairs, with their addresses and the consumed model
// UUIDs. It errors when the controller database cannot substantiate a
// third-party controller/model reference.
func (s *State) getExternalControllers(
	ctx context.Context, tx *sqlair.TX, offererModels []modelmigrationinternal.OffererModel,
) ([]modelmigration.ExternalController, error) {
	controllerUUIDs := distinctControllerUUIDs(offererModels)
	if len(controllerUUIDs) == 0 {
		return nil, nil
	}

	ctrlStmt, err := s.Prepare(`
SELECT &externalControllerRow.*
FROM   external_controller
WHERE  uuid IN ($uuidList[:])
`, externalControllerRow{}, uuidList{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	addrStmt, err := s.Prepare(`
SELECT controller_uuid AS &externalControllerAddressRow.controller_uuid,
       address AS &externalControllerAddressRow.address
FROM   external_controller_address
WHERE  controller_uuid IN ($uuidList[:])
`, externalControllerAddressRow{}, uuidList{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	modelStmt, err := s.Prepare(`
SELECT controller_uuid AS &externalModelRow.controller_uuid,
       uuid AS &externalModelRow.model_uuid
FROM   external_model
WHERE  controller_uuid IN ($uuidList[:])
`, externalModelRow{}, uuidList{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	uuids := uuidList(controllerUUIDs)
	var ctrls []externalControllerRow
	if err := getAll(ctx, tx, ctrlStmt, &ctrls, uuids); err != nil {
		return nil, errors.Errorf("querying external controllers: %w", err)
	}
	var addrs []externalControllerAddressRow
	if err := getAll(ctx, tx, addrStmt, &addrs, uuids); err != nil {
		return nil, errors.Errorf("querying external controller addresses: %w", err)
	}
	var extModels []externalModelRow
	if err := getAll(ctx, tx, modelStmt, &extModels, uuids); err != nil {
		return nil, errors.Errorf("querying external models: %w", err)
	}

	matched, err := matchingExternalModels(offererModels, ctrls, extModels)
	if err != nil {
		return nil, errors.Capture(err)
	}

	consumed := make(map[string][]string, len(matched))
	for _, model := range matched {
		consumed[model.ControllerUUID] = append(consumed[model.ControllerUUID], model.ModelUUID)
	}
	addrByController := make(map[string][]string, len(addrs))
	for _, a := range addrs {
		addrByController[a.ControllerUUID] = append(addrByController[a.ControllerUUID], a.Address)
	}

	controllers := make([]modelmigration.ExternalController, 0, len(ctrls))
	for _, ec := range ctrls {
		controllers = append(controllers, modelmigration.ExternalController{
			UUID:           ec.UUID,
			Alias:          derefString(ec.Alias),
			CACert:         ec.CACert,
			Addresses:      addrByController[ec.UUID],
			ConsumedModels: consumed[ec.UUID],
		})
	}
	return controllers, nil
}

// modelUserNames returns the distinct usernames whose profiles must travel
// with the model so the target can resolve them on import: the model
// qualifier, the credential owner, permission subjects and authorized-key
// owners. First-seen order is preserved.
func modelUserNames(
	identity modelmigration.ModelBootstrapInfo,
	perms []modelmigration.ModelPermission,
	authKeys []modelmigration.ModelAuthorizedKey,
) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}

	add(identity.Qualifier)
	add(identity.CredentialOwner)
	for _, p := range perms {
		add(p.SubjectName)
	}
	for _, k := range authKeys {
		add(k.Username)
	}
	return out
}

// getAll is a small helper that runs a prepared statement collecting all rows,
// tolerating ErrNoRows (treated as an empty result).
func getAll[T any](ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement, dest *[]T, args ...any) error {
	err := tx.Query(ctx, stmt, args...).GetAll(dest)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return err
	}
	return nil
}

// distinctControllerUUIDs returns the distinct controller UUIDs referenced by
// the supplied offerer-model pairs, preserving first-seen order.
func distinctControllerUUIDs(offererModels []modelmigrationinternal.OffererModel) []string {
	seen := make(map[string]struct{}, len(offererModels))
	var out []string
	for _, om := range offererModels {
		if _, ok := seen[om.ControllerUUID]; ok {
			continue
		}
		seen[om.ControllerUUID] = struct{}{}
		out = append(out, om.ControllerUUID)
	}
	return out
}

// matchingExternalModels returns the external_model rows selected by the model
// DB offerer pairs and errors when the controller DB cannot substantiate a
// third-party controller/model reference.
func matchingExternalModels(
	offererModels []modelmigrationinternal.OffererModel,
	extControllers []externalControllerRow,
	extModels []externalModelRow,
) ([]externalModelRow, error) {
	if len(offererModels) == 0 {
		return nil, nil
	}

	controllers := make(map[string]struct{}, len(extControllers))
	for _, ctrl := range extControllers {
		controllers[ctrl.UUID] = struct{}{}
	}
	models := make(map[externalModelKey]externalModelRow, len(extModels))
	for _, model := range extModels {
		models[externalModelKey{
			controllerUUID: model.ControllerUUID,
			modelUUID:      model.ModelUUID,
		}] = model
	}

	seen := make(map[externalModelKey]struct{}, len(offererModels))
	matched := make([]externalModelRow, 0, len(offererModels))
	for _, offerer := range offererModels {
		key := externalModelKey{
			controllerUUID: offerer.ControllerUUID,
			modelUUID:      offerer.ModelUUID,
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		if _, ok := controllers[offerer.ControllerUUID]; !ok {
			return nil, errors.Errorf(
				"external controller %q for offerer model %q not found",
				offerer.ControllerUUID, offerer.ModelUUID,
			)
		}
		model, ok := models[key]
		if !ok {
			return nil, errors.Errorf(
				"external model %q for controller %q not found",
				offerer.ModelUUID, offerer.ControllerUUID,
			)
		}
		matched = append(matched, model)
	}
	return matched, nil
}

// derefString returns the pointed-to string or empty when nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
