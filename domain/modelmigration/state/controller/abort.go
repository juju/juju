// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/life"
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
WITH target_phase AS (
    SELECT id FROM model_migration_import_phase_type WHERE type = $importPhaseNames.target
),
source_phase AS (
    SELECT id FROM model_migration_import_phase_type WHERE type = $importPhaseNames.source
)
UPDATE model_migration_import
SET    phase_type_id = (SELECT id FROM target_phase),
       updated_at    = DATETIME('now', 'utc')
WHERE  model_uuid = $modelUUIDArg.model_uuid
AND    phase_type_id = (SELECT id FROM source_phase)
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
		if affected == 1 {
			return nil
		}
		// The read above saw importing, so the CAS should have matched.
		// This is unreachable under snapshot isolation: a concurrent
		// phase change on another node would fail the transaction at
		// commit time and the framework would retry, at which point the
		// read would see the new phase. Treat it as a defensive guard.
		return errors.Errorf(
			"import phase changed concurrently: %w",
			modelmigrationerrors.ErrPhaseTransitionInvalid,
		)
	})
}

// getAbortClaimCheck computes within tx, in a single round trip, the two
// predicates [State.StageAbortedModelDatabaseDeletion] and
// [State.FinalizeAbortedImport] assert on a model's import claim: whether a
// claim exists at all, and whether it is in the aborting phase. The
// discrimination is done in SQL (rather than filtering to an aborting-only
// row, or extracting the claim and testing its phase in Go) so each caller
// can map the outcome to its own contract: a missing claim is idempotent
// success for finalization but an error for staging, while a wrong-phase
// claim is a hard error for both. A phase-filtered query cannot tell those
// apart: a still-importing claim and a missing claim would both yield no
// rows, and finalization would silently succeed while the claim survives.
func (s *State) getAbortClaimCheck(
	ctx context.Context, tx *sqlair.TX, modelUUID string,
) (abortClaimCheck, error) {
	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	stmt, err := s.Prepare(`
WITH claim AS (
    SELECT mmipt.type AS phase_type
    FROM   model_migration_import AS mmi
    JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
    WHERE  mmi.model_uuid = $modelUUIDArg.model_uuid
)
SELECT EXISTS(SELECT 1 FROM claim) AS &abortClaimCheck.claim_exists,
       EXISTS(SELECT 1 FROM claim WHERE phase_type = 'aborting') AS &abortClaimCheck.claim_aborting
`, abortClaimCheck{}, mUUID)
	if err != nil {
		return abortClaimCheck{}, errors.Capture(err)
	}

	var check abortClaimCheck
	if err := tx.Query(ctx, stmt, mUUID).Get(&check); err != nil {
		return abortClaimCheck{}, errors.Errorf(
			"checking aborting import claim for model %q: %w", modelUUID, err)
	}
	return check, nil
}

// IsModelDying reports whether the model row for modelUUID exists and has left
// the alive state (it is dying or dead). The v8 abort driver uses this to
// detect a model that the generic (legacy) removal undertaker is already
// tearing down after a v7-protocol abort marked it dead and took the claim's
// abort lock: in that case the v8 abort compensation and model-database staging
// must stand aside, because the undertaker owns the teardown of the model, its
// database and the import claim. A missing model row reports false, so a v8
// abort re-drive after its own compensation has removed the model row still
// proceeds to finalization.
func (s *State) IsModelDying(ctx context.Context, modelUUID string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	arg := modelUUIDArg{ModelUUID: modelUUID}
	stmt, err := s.Prepare(`
SELECT m.life_id AS &modelLifeRow.life_id
FROM   model AS m
WHERE  m.uuid = $modelUUIDArg.model_uuid`, arg, modelLifeRow{})
	if err != nil {
		return false, errors.Capture(err)
	}

	var row modelLifeRow
	var found bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		found = false
		err := tx.Query(ctx, stmt, arg).Get(&row)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	if err != nil {
		return false, errors.Errorf("reading model life for %q: %w", modelUUID, err)
	}
	if !found {
		return false, nil
	}
	// Anything but alive (dying, dead) means the generic removal undertaker
	// has taken over.
	return life.Life(row.LifeID) != life.Alive, nil
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
		rows = nil
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
		var err error
		registered, err = rowExists(ctx, tx, stmt, arg)
		return err
	})
	if err != nil {
		return false, errors.Errorf(
			"checking namespace registration for model %q: %w", modelUUID, err)
	}
	return registered, nil
}

// StageAbortedModelDatabaseDeletion hands the aborted model's dqlite database
// off to the undertaker's model-database deleter, in a single transaction: it
// removes the model's namespace_list registration (so the deleter no longer
// treats the model as live and will actually drop the database) and stages a
// model_database_deletion row keyed by the model UUID. The undertaker drops the
// database asynchronously and removes the staged row on success;
// [State.FinalizeAbortedImport] then releases the claim once the staged row is
// gone.
//
// It asserts, in the same transaction, that the claim is in the aborting
// phase, so a live model's database can never be staged for deletion. It is
// idempotent: the namespace delete is a no-op once done, and an already-staged
// deletion is left untouched, so re-driving it (or racing another controller
// node) is safe. Returns
// [modelmigrationerrors.ErrImportNotFound] when no claim exists and
// [modelmigrationerrors.ErrPhaseTransitionInvalid] when the claim is not
// aborting.
func (s *State) StageAbortedModelDatabaseDeletion(ctx context.Context, modelUUID string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	nsArg := namespaceArg{Namespace: modelUUID}
	deleteNamespaceListStmt, err := s.Prepare(`
DELETE FROM namespace_list
WHERE  namespace = $namespaceArg.namespace
`, nsArg)
	if err != nil {
		return errors.Capture(err)
	}
	stagedDeletionStmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   model_database_deletion
WHERE  namespace = $namespaceArg.namespace
LIMIT  1
`, countResult{}, nsArg)
	if err != nil {
		return errors.Capture(err)
	}
	stageDeletionStmt, err := s.Prepare(`
INSERT INTO model_database_deletion (*)
VALUES ($modelDatabaseDeletion.*)
`, modelDatabaseDeletion{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		check, err := s.getAbortClaimCheck(ctx, tx, modelUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !check.ClaimExists {
			return errors.Errorf("model %q: %w", modelUUID, modelmigrationerrors.ErrImportNotFound)
		}
		if !check.ClaimAborting {
			return errors.Errorf(
				"import claim for model %q is not aborting: %w",
				modelUUID, modelmigrationerrors.ErrPhaseTransitionInvalid)
		}

		if err := tx.Query(ctx, deleteNamespaceListStmt, nsArg).Run(); err != nil {
			return errors.Errorf("deleting namespace registration: %w", err)
		}
		// The deletion is already staged; leave its created_at untouched rather
		// than rewriting a timestamp that would misreport when it was staged.
		if staged, err := rowExists(ctx, tx, stagedDeletionStmt, nsArg); err != nil {
			return errors.Errorf("checking staged model database deletion: %w", err)
		} else if staged {
			return nil
		}
		if err := tx.Query(ctx, stageDeletionStmt, modelDatabaseDeletion{
			Namespace: modelUUID,
			CreatedAt: s.clock.Now().UTC(),
		}).Run(); err != nil {
			return errors.Errorf("staging model database deletion: %w", err)
		}
		return nil
	})
}

// FinalizeAbortedImport deletes the model_migration_import claim and its
// FK-dependent companion rows (model_migration_import_offer,
// model_migration_import_external_controller_model) in a single transaction,
// once abort cleanup is provably complete. It asserts, in the same transaction,
// that the claim is in the aborting phase, that the controller model identity
// row and its model_namespace row are both gone, and that no
// model_database_deletion row is still staged for the model's namespace (which
// would mean the undertaker has not yet dropped the model database); if any of
// these does not hold it returns [modelmigrationerrors.ErrAbortNotFinalizable]
// and makes no deletions, leaving the claim in aborting for a later retry. It
// is idempotent when no claim exists (already finalized). Returns
// [modelmigrationerrors.ErrPhaseTransitionInvalid] when a claim exists but is
// not aborting.
//
// The model dqlite database is dropped out of band by the undertaker's
// model-database deleter after [State.StageAbortedModelDatabaseDeletion] has
// removed the namespace_list registration and staged the deletion. This method
// only releases the durable claim, and only once that drop is proven complete
// (the staged row is gone). The abort reconciler enforces that ordering.
func (s *State) FinalizeAbortedImport(ctx context.Context, modelUUID string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	mUUID := modelUUIDArg{ModelUUID: modelUUID}
	nsArg := namespaceArg{Namespace: modelUUID}

	finalizeChecksStmt, err := s.Prepare(`
WITH model_row AS (
    SELECT 1 FROM model WHERE uuid = $modelUUIDArg.model_uuid
),
model_namespace_row AS (
    SELECT 1 FROM model_namespace WHERE model_uuid = $modelUUIDArg.model_uuid
),
staged_deletion_row AS (
    SELECT 1 FROM model_database_deletion WHERE namespace = $namespaceArg.namespace
)
SELECT EXISTS(SELECT 1 FROM model_row)           AS &abortFinalizeChecks.model_exists,
       EXISTS(SELECT 1 FROM model_namespace_row) AS &abortFinalizeChecks.namespace_exists,
       EXISTS(SELECT 1 FROM staged_deletion_row) AS &abortFinalizeChecks.deletion_staged
`, abortFinalizeChecks{}, mUUID, nsArg)
	if err != nil {
		return errors.Capture(err)
	}
	deleteOffersStmt, err := s.Prepare(`
WITH claim AS (
    SELECT uuid FROM model_migration_import WHERE model_uuid = $modelUUIDArg.model_uuid
)
DELETE FROM model_migration_import_offer
WHERE  migration_uuid IN (SELECT uuid FROM claim)
`, mUUID)
	if err != nil {
		return errors.Capture(err)
	}
	deleteECMStmt, err := s.Prepare(`
WITH claim AS (
    SELECT uuid FROM model_migration_import WHERE model_uuid = $modelUUIDArg.model_uuid
)
DELETE FROM model_migration_import_external_controller_model
WHERE  migration_uuid IN (SELECT uuid FROM claim)
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
		check, err := s.getAbortClaimCheck(ctx, tx, modelUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !check.ClaimExists {
			return nil // idempotent: already finalized
		}
		if !check.ClaimAborting {
			return errors.Errorf(
				"import claim for model %q is not aborting: %w",
				modelUUID, modelmigrationerrors.ErrPhaseTransitionInvalid)
		}

		// Finalization predicates, read in a single round trip: the controller
		// model identity row and its namespace mapping must both be gone, and the
		// model database must have been dropped by the undertaker (its staged
		// deletion row cleared), before the claim can be released.
		var checks abortFinalizeChecks
		if err := tx.Query(ctx, finalizeChecksStmt, mUUID, nsArg).Get(&checks); err != nil {
			return errors.Errorf("checking finalization predicates for model %q: %w", modelUUID, err)
		}
		if checks.ModelExists {
			return errors.Errorf(
				"model %q identity row still present: %w",
				modelUUID, modelmigrationerrors.ErrAbortNotFinalizable)
		}
		if checks.NamespaceExists {
			return errors.Errorf(
				"model %q namespace mapping still present: %w",
				modelUUID, modelmigrationerrors.ErrAbortNotFinalizable)
		}
		if checks.DeletionStaged {
			return errors.Errorf(
				"model %q database deletion still pending: %w",
				modelUUID, modelmigrationerrors.ErrAbortNotFinalizable)
		}

		// Cleanup is proven complete. Remove the claim (companions first, since
		// their FKs onto the claim do not cascade).
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
