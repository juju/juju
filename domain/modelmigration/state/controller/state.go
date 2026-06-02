// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"encoding/json"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"gopkg.in/macaroon.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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

// InsertExport records a new export migration attempt for a model. It ensures
// the target external_controller row exists (compare-or-insert), inserts the
// model_migration_export row in the QUIESCE phase with its companion
// model_migration_export_target_auth credentials, and seeds the phase history.
//
// If the model already has an active (non-ended) export migration the unique
// partial index rejects the insert and the error is reported as
// [modelmigrationerrors.ErrMigrationAlreadyActive].
func (s *State) InsertExport(ctx context.Context, spec modelmigration.MigrationSpec) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	quiesceID, err := migration.PhasePersistedID(migration.QUIESCE)
	if err != nil {
		return errors.Capture(err)
	}

	macaroonsJSON, err := marshalMacaroons(spec.Target.Macaroons)
	if err != nil {
		return errors.Errorf("marshalling target macaroons: %w", err)
	}

	now := s.clock.Now().UTC()

	export := migrationExport{
		UUID:                 spec.MigrationUUID,
		ModelUUID:            spec.ModelUUID,
		TargetControllerUUID: spec.Target.ControllerUUID,
		CurrentPhaseID:       quiesceID,
		PhaseChangedAt:       now,
		StartTime:            now,
	}
	auth := migrationTargetAuth{
		MigrationUUID:          spec.MigrationUUID,
		ExternalControllerUUID: spec.Target.ControllerUUID,
		TargetUser:             spec.Target.User,
		TargetMacaroons:        macaroonsJSON,
		TargetToken:            spec.Target.Token,
		TargetSkipUserChecks:   spec.Target.SkipUserChecks,
	}
	phaseEntry := migrationPhaseEntry{
		MigrationUUID: spec.MigrationUUID,
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

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.ensureExternalControllerMatchesOrInsert(ctx, tx, modelmigration.ExternalControllerInfo{
			UUID:      spec.Target.ControllerUUID,
			Alias:     spec.Target.ControllerAlias,
			CACert:    spec.Target.CACert,
			Addresses: spec.Target.Addrs,
		}); err != nil {
			return errors.Capture(err)
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

// EnsureExternalControllerMatchesOrInsert inserts the external controller and
// its addresses if absent, no-ops if an identical record already exists, and
// returns [modelmigrationerrors.ErrExternalControllerConflict] if a controller with
// the same UUID already exists with a different CA certificate or addresses.
//
// It never blindly overwrites the connection details of a shared
// external_controller row.
func (s *State) EnsureExternalControllerMatchesOrInsert(ctx context.Context, info modelmigration.ExternalControllerInfo) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.ensureExternalControllerMatchesOrInsert(ctx, tx, info)
	})
}

// ensureExternalControllerMatchesOrInsert is the transaction-scoped
// implementation shared by InsertExport and the public method.
func (s *State) ensureExternalControllerMatchesOrInsert(
	ctx context.Context, tx *sqlair.TX, info modelmigration.ExternalControllerInfo,
) error {
	ctrlUUID := entityUUID{UUID: info.UUID}

	selectCACertStmt, err := s.Prepare(`
SELECT &externalControllerCACert.ca_cert
FROM   external_controller
WHERE  uuid = $entityUUID.uuid
`, ctrlUUID, externalControllerCACert{})
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

	var existing externalControllerCACert
	err = tx.Query(ctx, selectCACertStmt, ctrlUUID).Get(&existing)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("looking up external controller %q: %w", info.UUID, err)
	}

	if errors.Is(err, sqlair.ErrNoRows) {
		// No existing controller: insert the controller row and its addresses.
		return s.insertExternalController(ctx, tx, info)
	}

	// Existing controller: the CA certificate and addresses must match exactly.
	if existing.CACert != info.CACert {
		return errors.Errorf(
			"external controller %q exists with a different CA certificate: %w",
			info.UUID, modelmigrationerrors.ErrExternalControllerConflict,
		)
	}

	var existingAddrs []addressValue
	if err := tx.Query(ctx, selectAddrsStmt, ctrlUUID).GetAll(&existingAddrs); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("looking up external controller %q addresses: %w", info.UUID, err)
	}
	if !addressesMatch(existingAddrs, info.Addresses) {
		return errors.Errorf(
			"external controller %q exists with different addresses: %w",
			info.UUID, modelmigrationerrors.ErrExternalControllerConflict,
		)
	}
	return nil
}

// insertExternalController inserts a new external_controller row together with
// its addresses.
func (s *State) insertExternalController(
	ctx context.Context, tx *sqlair.TX, info modelmigration.ExternalControllerInfo,
) error {
	controller := externalControllerUpsert{
		UUID:   info.UUID,
		Alias:  info.Alias,
		CACert: info.CACert,
	}
	insertCtrlStmt, err := s.Prepare(`
INSERT INTO external_controller (*) VALUES ($externalControllerUpsert.*)
`, controller)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, insertCtrlStmt, controller).Run(); err != nil {
		return errors.Errorf("inserting external controller %q: %w", info.UUID, err)
	}

	if len(info.Addresses) == 0 {
		return nil
	}

	insertAddrStmt, err := s.Prepare(`
INSERT INTO external_controller_address (*) VALUES ($externalControllerAddress.*)
`, externalControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	for _, addr := range info.Addresses {
		addrUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		row := externalControllerAddress{
			UUID:           addrUUID.String(),
			ControllerUUID: info.UUID,
			Address:        addr,
		}
		if err := tx.Query(ctx, insertAddrStmt, row).Run(); err != nil {
			return errors.Errorf("inserting external controller %q address: %w", info.UUID, err)
		}
	}
	return nil
}

// GetActiveExport returns the active (non-ended) export migration for the given
// model, including the reconstructed target connection details. If no active
// export exists [modelmigrationerrors.ErrMigrationNotFound] is returned.
func (s *State) GetActiveExport(ctx context.Context, modelUUID string) (modelmigration.Migration, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return modelmigration.Migration{}, errors.Capture(err)
	}

	mUUID := modelUUIDArg{ModelUUID: modelUUID}

	selectExportStmt, err := s.Prepare(`
SELECT &migrationExport.*
FROM   model_migration_export
WHERE  model_uuid = $modelUUIDArg.model_uuid
AND    end_time IS NULL
`, mUUID, migrationExport{})
	if err != nil {
		return modelmigration.Migration{}, errors.Capture(err)
	}

	var (
		export migrationExport
		auth   migrationTargetAuth
		ctrl   externalControllerUpsert
		addrs  []addressValue
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, selectExportStmt, mUUID).Get(&export)
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
SELECT &externalControllerUpsert.*
FROM   external_controller
WHERE  uuid = $entityUUID.uuid
`, ctrlUUID, externalControllerUpsert{})
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
		return modelmigration.Migration{}, errors.Capture(err)
	}

	phase, err := migration.PhaseFromPersistedID(export.CurrentPhaseID)
	if err != nil {
		return modelmigration.Migration{}, errors.Capture(err)
	}

	macaroons, err := unmarshalMacaroons(auth.TargetMacaroons)
	if err != nil {
		return modelmigration.Migration{}, errors.Errorf("unmarshalling target macaroons: %w", err)
	}

	addresses := make([]string, len(addrs))
	for i, a := range addrs {
		addresses[i] = a.Address
	}

	return modelmigration.Migration{
		UUID:             export.UUID,
		Phase:            phase,
		PhaseChangedTime: export.PhaseChangedAt,
		Target: migration.TargetInfo{
			ControllerUUID:  export.TargetControllerUUID,
			ControllerAlias: ctrl.Alias,
			Addrs:           addresses,
			CACert:          ctrl.CACert,
			User:            auth.TargetUser,
			Macaroons:       macaroons,
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

	migUUID := entityUUID{UUID: migrationUUID}
	selectPhaseStmt, err := s.Prepare(`
SELECT &currentPhase.current_phase_id
FROM   model_migration_export
WHERE  uuid = $entityUUID.uuid
AND    end_time IS NULL
`, migUUID, currentPhase{})
	if err != nil {
		return errors.Capture(err)
	}

	updateStmt, err := s.Prepare(`
UPDATE model_migration_export
SET    current_phase_id = $phaseUpdate.new_phase_id,
       phase_changed_at = $phaseUpdate.phase_changed_at
WHERE  uuid = $phaseUpdate.uuid
AND    current_phase_id = $phaseUpdate.expected_phase_id
`, phaseUpdate{})
	if err != nil {
		return errors.Capture(err)
	}

	// Reaching a terminal phase ends the export: stamp end_time in the same
	// update so the model no longer has an active migration.
	updateEndStmt, err := s.Prepare(`
UPDATE model_migration_export
SET    current_phase_id = $phaseUpdate.new_phase_id,
       phase_changed_at = $phaseUpdate.phase_changed_at,
       end_time = $phaseUpdate.phase_changed_at
WHERE  uuid = $phaseUpdate.uuid
AND    current_phase_id = $phaseUpdate.expected_phase_id
`, phaseUpdate{})
	if err != nil {
		return errors.Capture(err)
	}

	now := s.clock.Now().UTC()
	phaseEntry := migrationPhaseEntry{
		MigrationUUID: migrationUUID,
		PhaseID:       newPhaseID,
		ChangedAt:     now,
	}
	insertPhaseStmt, err := s.Prepare(`
INSERT INTO model_migration_export_phase (*) VALUES ($migrationPhaseEntry.*)
`, phaseEntry)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var cur currentPhase
		err := tx.Query(ctx, selectPhaseStmt, migUUID).Get(&cur)
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
			PhaseChangedAt:  now,
		}
		stmt := updateStmt
		if newPhase.IsTerminal() {
			stmt = updateEndStmt
		}
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, stmt, update).Get(&outcome); err != nil {
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

		if err := tx.Query(ctx, insertPhaseStmt, phaseEntry).Run(); err != nil {
			return errors.Errorf("recording phase history for migration %q: %w", migrationUUID, err)
		}
		return nil
	})
}

// SetStatusMessage appends a free-form status message to the export migration's
// status history.
func (s *State) SetStatusMessage(ctx context.Context, migrationUUID, message string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	statusUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	status := migrationStatus{
		UUID:          statusUUID.String(),
		MigrationUUID: migrationUUID,
		Message:       message,
		RecordedAt:    s.clock.Now().UTC(),
	}
	stmt, err := s.Prepare(`
INSERT INTO model_migration_export_status (*) VALUES ($migrationStatus.*)
`, status)
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, status).Run(); err != nil {
			return errors.Errorf("inserting status message for migration %q: %w", migrationUUID, err)
		}
		return nil
	})
}

// InsertMinionReport records a phase report from a single minion agent. The
// (migration, phase, entity) triple is unique, so a repeated report for the
// same agent and phase overwrites the previous success value.
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
	// A minion may resubmit a report for the same phase; update the recorded
	// success/timestamp on conflict rather than failing the unique constraint.
	stmt, err := s.Prepare(`
INSERT INTO model_migration_export_minion_sync (*) VALUES ($migrationMinionSync.*)
ON CONFLICT (migration_uuid, phase_id, entity_key) DO UPDATE SET
    success = excluded.success,
    reported_at = excluded.reported_at
`, report)
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, report).Run(); err != nil {
			return errors.Errorf("inserting minion report for migration %q: %w", migrationUUID, err)
		}
		return nil
	})
}

// AggregateMinionReports returns the succeeded and failed entity keys reported
// by minions for the given migration and phase.
func (s *State) AggregateMinionReports(
	ctx context.Context, migrationUUID string, phase migration.Phase,
) (modelmigration.MinionReports, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return modelmigration.MinionReports{}, errors.Capture(err)
	}

	phaseID, err := migration.PhasePersistedID(phase)
	if err != nil {
		return modelmigration.MinionReports{}, errors.Errorf("converting phase %q: %w", phase, err)
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
		return modelmigration.MinionReports{}, errors.Capture(err)
	}

	var rows []minionReportRow
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, migUUID, phaseArg).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying minion reports for migration %q: %w", migrationUUID, err)
		}
		return nil
	})
	if err != nil {
		return modelmigration.MinionReports{}, errors.Capture(err)
	}

	reports := modelmigration.MinionReports{Phase: phase}
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
// phase and an end_time, and appends the terminal phase to the phase history.
// Once ended, the model no longer has an active export migration.
func (s *State) MarkExportEnded(ctx context.Context, migrationUUID string, terminalPhase migration.Phase) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	phaseID, err := migration.PhasePersistedID(terminalPhase)
	if err != nil {
		return errors.Errorf("converting phase %q: %w", terminalPhase, err)
	}

	now := s.clock.Now().UTC()
	end := endExport{
		UUID:           migrationUUID,
		PhaseID:        phaseID,
		PhaseChangedAt: now,
		EndTime:        now,
	}
	updateStmt, err := s.Prepare(`
UPDATE model_migration_export
SET    current_phase_id = $endExport.current_phase_id,
       phase_changed_at = $endExport.phase_changed_at,
       end_time = $endExport.end_time
WHERE  uuid = $endExport.uuid
AND    end_time IS NULL
`, end)
	if err != nil {
		return errors.Capture(err)
	}

	phaseEntry := migrationPhaseEntry{
		MigrationUUID: migrationUUID,
		PhaseID:       phaseID,
		ChangedAt:     now,
	}
	insertPhaseStmt, err := s.Prepare(`
INSERT INTO model_migration_export_phase (*) VALUES ($migrationPhaseEntry.*)
ON CONFLICT (migration_uuid, phase_id) DO NOTHING
`, phaseEntry)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, updateStmt, end).Get(&outcome); err != nil {
			return errors.Errorf("ending migration %q: %w", migrationUUID, err)
		}
		affected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if affected == 0 {
			return errors.Errorf("no active migration %q: %w", migrationUUID, modelmigrationerrors.ErrMigrationNotFound)
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
	exportStmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   model_migration_export
WHERE  model_uuid = $modelUUIDArg.model_uuid
AND    end_time IS NULL
`, mUUID, countResult{})
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
		if err := tx.Query(ctx, exportStmt, mUUID).Get(&exportCount); err != nil {
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

// marshalMacaroons serialises a slice of macaroon slices to the JSON form
// stored in model_migration_export_target_auth.target_macaroons.
func marshalMacaroons(macaroons []macaroon.Slice) (string, error) {
	if len(macaroons) == 0 {
		return "", nil
	}
	b, err := json.Marshal(macaroons)
	if err != nil {
		return "", errors.Capture(err)
	}
	return string(b), nil
}

// unmarshalMacaroons reverses marshalMacaroons.
func unmarshalMacaroons(data string) ([]macaroon.Slice, error) {
	if data == "" {
		return nil, nil
	}
	var macaroons []macaroon.Slice
	if err := json.Unmarshal([]byte(data), &macaroons); err != nil {
		return nil, errors.Capture(err)
	}
	return macaroons, nil
}

// addressesMatch reports whether the persisted addresses equal the supplied
// addresses, ignoring order.
func addressesMatch(existing []addressValue, supplied []string) bool {
	if len(existing) != len(supplied) {
		return false
	}
	have := make(map[string]int, len(existing))
	for _, a := range existing {
		have[a.Address]++
	}
	for _, a := range supplied {
		if have[a] == 0 {
			return false
		}
		have[a]--
	}
	return true
}
