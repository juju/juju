// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
)

// These methods implement the target-side v8 import abort claim lifecycle: the
// importing -> aborting phase transition, the scan the abort reconciler uses to
// find outstanding claims, the namespace-registration predicate that decides
// whether the model dqlite database still needs dropping, and the abort
// finalization that deletes the durable claim once cleanup is provably
// complete. The compensation of the controller-data rows themselves
// (permissions, authorized keys, leadership, model identity, ...) is owned by
// the per-domain services the v8 abort driver calls; this package only owns the
// migration bookkeeping tables.

// SetImportPhaseAborting transitions the model_migration_import claim for
// modelUUID from importing to aborting, bumping updated_at. It is idempotent
// when the claim is already aborting. It returns
// [modelmigrationerrors.ErrAbortActivating] when the claim is activating (the
// activation point of no return has been crossed and the model may not be torn
// down), [modelmigrationerrors.ErrImportNotFound] when no claim exists, and
// [modelmigrationerrors.ErrPhaseTransitionInvalid] when the phase changed
// concurrently.
func (s *State) SetImportPhaseAborting(ctx context.Context, modelUUID string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	phases := importPhaseNames{
		Target: string(modelmigration.ImportPhaseAborting),
		Source: string(modelmigration.ImportPhaseImporting),
	}
	updateStmt, err := s.Prepare(`
UPDATE model_migration_import
SET    phase_type_id = (SELECT id FROM model_migration_import_phase_type WHERE type = $importPhaseNames.target),
       updated_at    = DATETIME('now', 'utc')
WHERE  model_uuid = $modelUUIDArg.model_uuid
AND    phase_type_id = (SELECT id FROM model_migration_import_phase_type WHERE type = $importPhaseNames.source)
`, mUUID, phases)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		claim, err := s.getImportClaim(ctx, tx, modelUUID)
		if err != nil {
			return errors.Capture(err)
		}
		switch claim.Phase {
		case modelmigration.ImportPhaseAborting:
			return nil // idempotent: already aborting
		case modelmigration.ImportPhaseActivating:
			return errors.Capture(modelmigrationerrors.ErrAbortActivating)
		}

		// Phase is importing; CAS to aborting.
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, updateStmt, mUUID, phases).Get(&outcome); err != nil {
			return errors.Errorf("transitioning import to aborting: %w", err)
		}
		affected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if affected == 0 {
			// Concurrent phase change won the race.
			return errors.Errorf(
				"import phase changed concurrently: %w",
				modelmigrationerrors.ErrPhaseTransitionInvalid,
			)
		}
		return nil
	})
}

// GetAllImportClaims returns a snapshot of every outstanding
// model_migration_import claim. The abort reconciler scans this to find claims
// in the aborting phase to finalize and stale importing/activating claims to
// warn about. The table holds at most one row per migrating model and is
// small, so a full scan is cheap.
func (s *State) GetAllImportClaims(ctx context.Context) ([]modelmigration.ImportClaimStatus, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT mmi.model_uuid AS &importClaimStatusRow.model_uuid,
       mmi.source_migration_uuid AS &importClaimStatusRow.source_migration_uuid,
       mmipt.type AS &importClaimStatusRow.phase_type,
       strftime('%Y-%m-%dT%H:%M:%fZ', mmi.updated_at) AS &importClaimStatusRow.updated_at
FROM   model_migration_import AS mmi
JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
`, importClaimStatusRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []importClaimStatusRow
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("getting all import claims: %w", err)
	}

	claims := make([]modelmigration.ImportClaimStatus, 0, len(rows))
	for _, row := range rows {
		updatedAt, err := time.Parse(time.RFC3339, row.UpdatedAt)
		if err != nil {
			return nil, errors.Errorf(
				"parsing import updated_at for model %q: %w", row.ModelUUID, err)
		}
		claims = append(claims, modelmigration.ImportClaimStatus{
			ModelUUID:           row.ModelUUID,
			SourceMigrationUUID: row.SourceMigrationUUID,
			Phase:               modelmigration.ImportPhase(row.PhaseType),
			UpdatedAt:           updatedAt,
		})
	}
	return claims, nil
}

// IsImportNamespaceRegistered reports whether the model's dqlite namespace is
// still registered in namespace_list. The abort reconciler uses this to decide
// whether the model database still needs dropping before finalization: a
// registered namespace means the database may still exist and must be deleted
// while it can still be reopened, whereas an absent registration means the
// database was never created (or already dropped) and finalization can proceed.
func (s *State) IsImportNamespaceRegistered(ctx context.Context, modelUUID string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	arg := namespaceArg{Namespace: modelUUID}
	stmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   namespace_list
WHERE  namespace = $namespaceArg.namespace
LIMIT  1
`, countResult{}, arg)
	if err != nil {
		return false, errors.Capture(err)
	}

	var registered bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		registered, err = rowExists(ctx, tx, stmt, arg)
		return err
	})
	if err != nil {
		return false, errors.Errorf(
			"checking namespace registration for model %q: %w", modelUUID, err)
	}
	return registered, nil
}

// FinalizeAbortedImport deletes the model_migration_import claim and its
// FK-dependent companion rows (model_migration_import_offer,
// model_migration_import_external_controller_model) along with the model's
// namespace_list registration, in a single transaction, once abort cleanup is
// provably complete. It asserts, in the same transaction, that the claim is in
// the aborting phase and that the controller model identity row and its
// model_namespace row are both gone; if either still exists it returns
// [modelmigrationerrors.ErrAbortNotFinalizable] and makes no deletions, leaving
// the claim in aborting for a later retry. It is idempotent when no claim
// exists (already finalized). Returns
// [modelmigrationerrors.ErrPhaseTransitionInvalid] when a claim exists but is
// not aborting.
//
// The model dqlite database must already have been dropped by the caller before
// this runs: deleting the namespace_list registration here removes the last
// handle through which the database could be reopened, so dropping it
// afterwards would be impossible. The abort reconciler enforces that ordering.
func (s *State) FinalizeAbortedImport(ctx context.Context, modelUUID string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	nsArg := namespaceArg{Namespace: modelUUID}

	modelStmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   model AS m
WHERE  m.uuid = $modelUUIDArg.model_uuid
LIMIT  1
`, countResult{}, mUUID)
	if err != nil {
		return errors.Capture(err)
	}
	modelNamespaceStmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   model_namespace AS mn
WHERE  mn.model_uuid = $modelUUIDArg.model_uuid
LIMIT  1
`, countResult{}, mUUID)
	if err != nil {
		return errors.Capture(err)
	}
	deleteNamespaceListStmt, err := s.Prepare(`
DELETE FROM namespace_list
WHERE  namespace = $namespaceArg.namespace
`, nsArg)
	if err != nil {
		return errors.Capture(err)
	}
	deleteOffersStmt, err := s.Prepare(`
DELETE FROM model_migration_import_offer
WHERE  migration_uuid IN (
       SELECT uuid FROM model_migration_import WHERE model_uuid = $modelUUIDArg.model_uuid)
`, mUUID)
	if err != nil {
		return errors.Capture(err)
	}
	deleteECMStmt, err := s.Prepare(`
DELETE FROM model_migration_import_external_controller_model
WHERE  migration_uuid IN (
       SELECT uuid FROM model_migration_import WHERE model_uuid = $modelUUIDArg.model_uuid)
`, mUUID)
	if err != nil {
		return errors.Capture(err)
	}
	deleteClaimStmt, err := s.Prepare(`
DELETE FROM model_migration_import
WHERE  model_uuid = $modelUUIDArg.model_uuid
`, mUUID)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		claim, err := s.getImportClaim(ctx, tx, modelUUID)
		if errors.Is(err, modelmigrationerrors.ErrImportNotFound) {
			return nil // idempotent: already finalized
		}
		if err != nil {
			return errors.Capture(err)
		}
		if claim.Phase != modelmigration.ImportPhaseAborting {
			return errors.Errorf(
				"import claim is %q, expected aborting: %w",
				claim.Phase, modelmigrationerrors.ErrPhaseTransitionInvalid,
			)
		}

		// Finalization predicates: the controller model identity row and its
		// namespace mapping must both be gone before the claim can be released.
		if modelExists, err := rowExists(ctx, tx, modelStmt, mUUID); err != nil {
			return errors.Errorf("checking model row for model %q: %w", modelUUID, err)
		} else if modelExists {
			return errors.Errorf(
				"model %q identity row still present: %w",
				modelUUID, modelmigrationerrors.ErrAbortNotFinalizable)
		}
		if nsExists, err := rowExists(ctx, tx, modelNamespaceStmt, mUUID); err != nil {
			return errors.Errorf("checking model namespace for model %q: %w", modelUUID, err)
		} else if nsExists {
			return errors.Errorf(
				"model %q namespace mapping still present: %w",
				modelUUID, modelmigrationerrors.ErrAbortNotFinalizable)
		}

		// Cleanup is proven complete. Remove the namespace registration and the
		// claim (companions first, since their FKs onto the claim do not
		// cascade).
		if err := tx.Query(ctx, deleteNamespaceListStmt, nsArg).Run(); err != nil {
			return errors.Errorf("deleting namespace registration: %w", err)
		}
		if err := tx.Query(ctx, deleteOffersStmt, mUUID).Run(); err != nil {
			return errors.Errorf("deleting import offer companions: %w", err)
		}
		if err := tx.Query(ctx, deleteECMStmt, mUUID).Run(); err != nil {
			return errors.Errorf("deleting import external controller companions: %w", err)
		}
		if err := tx.Query(ctx, deleteClaimStmt, mUUID).Run(); err != nil {
			return errors.Errorf("deleting aborted import claim: %w", err)
		}
		return nil
	})
}
