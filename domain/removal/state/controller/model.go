// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmigration"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// ModelExists returns true if a model exists with the input UUID.
// This uses the *model* database table, not the *controller* model table.
// The model table with one row should exist until the model is removed.
func (st *State) ModelExists(ctx context.Context, mUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	modelUUID := entityUUID{UUID: mUUID}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   model
WHERE  uuid = $entityUUID.uuid`, modelUUID)
	if err != nil {
		return false, errors.Errorf("preparing model exists query: %w", err)
	}

	var modelExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, modelUUID).Get(&modelUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running model exists query: %w", err)
		}

		modelExists = true
		return nil
	})

	return modelExists, errors.Capture(err)
}

// EnsureModelNotAlive ensures that there is no model identified
// by the input model UUID, that is still alive. This does not cascade, as
// it is only used to set the model life to dying.
func (st *State) EnsureModelNotAlive(ctx context.Context, modelUUID string, force bool) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	eUUID := entityUUID{
		UUID: modelUUID,
	}

	updateModelLife, err := st.Prepare(`UPDATE model SET life_id = 1 WHERE uuid = $entityUUID.uuid AND life_id = 0`, eUUID)
	if err != nil {
		return errors.Errorf("preparing update model life query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Update the model life to dying.
		if err := tx.Query(ctx, updateModelLife, eUUID).Run(); err != nil {
			return errors.Errorf("setting model life to dying: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("ensuring model %q is not alive: %w", modelUUID, err)
	}

	return nil
}

// GetModelLife retrieves the life state of a model.
func (st *State) GetModelLife(ctx context.Context, mUUID string) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var life life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		life, err = st.getModelLife(ctx, tx, mUUID)

		return errors.Capture(err)
	})

	return life, errors.Capture(err)
}

// IsMigratingModel returns whether the model with the input UUID is currently
// migrating.
func (st *State) IsMigratingModel(ctx context.Context, mUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	var isMigrating bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		isMigrating, err = st.isModelMigrating(ctx, tx, mUUID)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return false, errors.Errorf("checking if model %q is migrating: %w", mUUID, err)
	}

	return isMigrating, nil
}

// MarkMigratingModelAsDead force-kills a migrating model as part of a
// target-side import abort (the v7/legacy Abort facade path), and, when the
// model still carries an import claim in the importing phase, transitions that
// claim to the aborting phase in the *same transaction*.
//
// Transitioning the claim to aborting atomically with the mark-dead is what
// prevents a concurrent activation from resurrecting a just-killed model.
// Activation compare-and-sets the claim importing->activating before it flips
// the model row to activated (domain/modelmigration ... SetImportPhaseActivating,
// domain/model ... Activate, which has no life check). If the abort only marked
// the model dead and left the claim importing, a stale master driving Activate
// could still win that CAS and activate the dead model - a split brain where the
// model is both dead and active. Moving the claim to aborting here makes the
// activation's own CAS refuse (ErrActivationAborting), so the two operations are
// mutually exclusive on the claim row.
//
// The claim is deliberately left in the aborting phase: the generic model
// teardown (removeBasicModelData, driven by the undertaker once the model is
// dead) deletes it once the model has been reaped and its database dropped. A
// claim that has already crossed the activation point of no return (activating)
// is refused with MigrationImportPastImporting - an activated model must not be
// torn down by the abort path.
//
// It does not check the current life state before killing, as migrating models
// can be either alive or dying; it is idempotent once the model is dead.
func (st *State) MarkMigratingModelAsDead(ctx context.Context, mUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	modelUUID := entityUUID{UUID: mUUID}
	markDeadStmt, err := st.Prepare(`
UPDATE model
SET    life_id = 2
WHERE  uuid = $entityUUID.uuid`, modelUUID)
	if err != nil {
		return errors.Errorf("preparing migrating model life update: %w", err)
	}
	// CAS the import claim importing -> aborting. Phase-type ids are resolved by
	// name so the statement does not depend on their numeric values.
	claimToAbortingStmt, err := st.Prepare(`
WITH aborting_phase AS (
    SELECT id FROM model_migration_import_phase_type WHERE type = 'aborting'
),
importing_phase AS (
    SELECT id FROM model_migration_import_phase_type WHERE type = 'importing'
)
UPDATE model_migration_import
SET    phase_type_id = (SELECT id FROM aborting_phase),
       updated_at    = DATETIME('now', 'utc')
WHERE  model_uuid = $entityUUID.uuid
AND    phase_type_id = (SELECT id FROM importing_phase)`, modelUUID)
	if err != nil {
		return errors.Errorf("preparing import claim abort transition: %w", err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if l, err := st.getModelLife(ctx, tx, mUUID); err != nil {
			return errors.Errorf("getting migrating model life: %w", err)
		} else if l == life.Dead {
			return nil
		}

		hasClaim, phase, err := st.migratingImportPhase(ctx, tx, mUUID)
		if err != nil {
			return errors.Errorf("checking if model is migrating: %w", err)
		}
		if !hasClaim {
			return errors.Errorf("model is not migrating")
		}

		switch phase {
		case string(modelmigration.ImportPhaseImporting):
			// Take the abort lock on the claim before killing the model, so a
			// concurrent activation's importing->activating CAS can no longer
			// win.
			var outcome sqlair.Outcome
			if err := tx.Query(ctx, claimToAbortingStmt, modelUUID).Get(&outcome); err != nil {
				return errors.Errorf("transitioning import claim to aborting: %w", err)
			}
			if affected, err := outcome.Result().RowsAffected(); err != nil {
				return errors.Capture(err)
			} else if affected == 0 {
				// The read above saw importing; a zero-row CAS means the phase
				// changed concurrently (an activation raced in between the read
				// and the update). Refuse rather than kill a model that is being
				// activated.
				return errors.Errorf(
					"model %q migration import left the importing phase concurrently: %w",
					mUUID, removalerrors.MigrationImportPastImporting)
			}
		case string(modelmigration.ImportPhaseAborting):
			// A retried abort: the claim is already aborting (this call, or an
			// earlier one, took the lock). Re-kill the model idempotently and
			// leave the claim for the undertaker to reap.
		default:
			// activating (or any unexpected phase): the model has crossed the
			// activation point of no return and must not be torn down here.
			return errors.Errorf(
				"model %q migration import is %q: %w",
				mUUID, phase, removalerrors.MigrationImportPastImporting)
		}

		if err := tx.Query(ctx, markDeadStmt, modelUUID).Run(); err != nil {
			return errors.Errorf("marking migrating model as dead: %w", err)
		}
		return nil
	}))
}

// MarkModelAsDead marks the model with the input UUID as dead.
func (st *State) MarkModelAsDead(ctx context.Context, mUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	modelUUID := entityUUID{UUID: mUUID}
	updateStmt, err := st.Prepare(`
UPDATE model
SET    life_id = 2
WHERE  uuid = $entityUUID.uuid
AND    life_id = 1`, modelUUID)
	if err != nil {
		return errors.Errorf("preparing model life update: %w", err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if l, err := st.getModelLife(ctx, tx, mUUID); err != nil {
			return errors.Errorf("getting model life: %w", err)
		} else if l == life.Dead {
			return nil
		} else if l == life.Alive {
			return removalerrors.EntityStillAlive
		}

		err := tx.Query(ctx, updateStmt, modelUUID).Run()
		if err != nil {
			return errors.Errorf("marking model as dead: %w", err)
		}

		return nil
	}))
}

// DeleteModel deletes all artifacts associated with a model.
func (st *State) DeleteModel(ctx context.Context, mUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	modelUUIDParam := entityUUID{UUID: mUUID}

	// Prepare query for deleting model row.
	deleteModelStmt, err := st.Prepare(`
DELETE FROM model 
WHERE uuid = $entityUUID.uuid;
`, modelUUIDParam)
	if err != nil {
		return errors.Capture(err)
	}

	// Delete the model from the namespace list. This prevents the model from
	// coming back alive again. The DB accessor should ensure that if it's
	// not in the namespace list, then it cannot be created again.
	deleteNamespaceStmt, err := st.Prepare(`
DELETE FROM namespace_list
WHERE namespace = $entityUUID.uuid;
`, modelUUIDParam)
	if err != nil {
		return errors.Capture(err)
	}

	deletePermissionsStmt, err := st.Prepare(`
DELETE FROM permission
WHERE grant_on = $entityUUID.uuid;
`, modelUUIDParam)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		mLife, err := st.getModelLife(ctx, tx, modelUUIDParam.UUID)
		if err != nil {
			return errors.Errorf("getting model life: %w", err)
		} else if mLife == life.Alive {
			return errors.Errorf("cannot delete model %q, model is still alive", modelUUIDParam.UUID).
				Add(removalerrors.EntityStillAlive)
		}

		// We should not of got here, even with force. The model must be dead
		// before it can be deleted.
		if mLife == life.Dying {
			return errors.Errorf("waiting for model to be dead before deletion").
				Add(removalerrors.RemovalJobIncomplete)
		}

		// Delete the model's basic data in one shot.
		if err := st.removeBasicModelData(ctx, tx, modelUUIDParam.UUID); err != nil {
			return errors.Errorf("removing basic model data: %w", err)
		}

		// Delete the model permissions.
		if err := tx.Query(ctx, deletePermissionsStmt, modelUUIDParam).Run(); err != nil {
			return errors.Errorf("deleting model permissions: %w", err)
		}

		// Delete the model row.
		if err := tx.Query(ctx, deleteModelStmt, modelUUIDParam).Run(); err != nil {
			return errors.Errorf("deleting model: %w", err)
		}

		// Ensure the model is dead and can't come back alive.
		if err := tx.Query(ctx, deleteNamespaceStmt, modelUUIDParam).Run(); err != nil {
			return errors.Errorf("deleting model from namespace list: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("deleting model: %w", err)
	}
	return nil
}

// GetModelUUIDs retrieves the UUIDs of all models in the controller.
func (st *State) GetModelUUIDs(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   model;
`, entityUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing get model UUIDs query: %w", err)
	}

	var modelUUIDs []entityUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).GetAll(&modelUUIDs); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running get model UUIDs query: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting model UUIDs: %w", err)
	}

	return transform.Slice(modelUUIDs, func(e entityUUID) string {
		return e.UUID
	}), nil
}

func (st *State) getModelLife(ctx context.Context, tx *sqlair.TX, mUUID string) (life.Life, error) {
	var model entityLife
	modelUUID := entityUUID{UUID: mUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   model
WHERE  uuid = $entityUUID.uuid;`, model, modelUUID)
	if err != nil {
		return -1, errors.Errorf("preparing model life query: %w", err)
	}

	err = tx.Query(ctx, stmt, modelUUID).Get(&model)
	if errors.Is(err, sqlair.ErrNoRows) {
		return -1, modelerrors.NotFound
	} else if err != nil {
		return -1, errors.Errorf("running model life query: %w", err)
	}

	return life.Life(model.Life), nil
}

func (st *State) isModelMigrating(ctx context.Context, tx *sqlair.TX, mUUID string) (bool, error) {
	modelUUID := entityUUID{UUID: mUUID}
	checkStmt, err := st.Prepare(`
SELECT COUNT(uuid) AS &count.count
FROM   model_migration_import
WHERE  model_uuid = $entityUUID.uuid;`, modelUUID, count{})
	if err != nil {
		return false, errors.Errorf("preparing is migrating model query: %w", err)
	}

	var result count
	err = tx.Query(ctx, checkStmt, modelUUID).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Errorf("running is migrating model query: %w", err)
	}

	return result.Count > 0, nil
}

// migratingImportPhase returns the v8 import claim phase for the model, and
// whether a claim exists at all. A missing claim reports hasClaim=false.
func (st *State) migratingImportPhase(ctx context.Context, tx *sqlair.TX, mUUID string) (hasClaim bool, phase string, err error) {
	modelUUID := entityUUID{UUID: mUUID}
	stmt, err := st.Prepare(`
SELECT mmipt.type AS &migrationImportPhase.phase
FROM   model_migration_import AS mmi
JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
WHERE  mmi.model_uuid = $entityUUID.uuid;`, modelUUID, migrationImportPhase{})
	if err != nil {
		return false, "", errors.Errorf("preparing migrating import phase query: %w", err)
	}

	var row migrationImportPhase
	err = tx.Query(ctx, stmt, modelUUID).Get(&row)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, "", nil
	} else if err != nil {
		return false, "", errors.Errorf("running migrating import phase query: %w", err)
	}
	return true, row.Phase, nil
}

func (st *State) removeBasicModelData(ctx context.Context, tx *sqlair.TX, mUUID string) error {
	modelUUIDRec := entityUUID{UUID: mUUID}

	tables := []string{
		"DELETE FROM model_namespace WHERE model_uuid = $entityUUID.uuid",
		"DELETE FROM model_secret_backend WHERE model_uuid = $entityUUID.uuid",
		"DELETE FROM secret_backend_reference WHERE model_uuid = $entityUUID.uuid",
		"DELETE FROM model_authorized_keys WHERE model_uuid = $entityUUID.uuid",
		"DELETE FROM model_last_login WHERE model_uuid = $entityUUID.uuid",
		// The two import companion tables are keyed by the import claim UUID
		// and FK onto model_migration_import. They must be deleted before the
		// claim row itself, otherwise the parent delete fails an enforced
		// foreign-key constraint when an import had recorded offer permissions
		// or external controllers.
		//
		// The claim (and its companions) is deleted here when it is:
		//   - importing: a legacy v4-v7 abort (whose claims are always
		//     importing) or normal destruction of a model that never migrated
		//     (no claim, so these are no-ops); or
		//   - aborting AND no model-database deletion is still staged for the
		//     model's namespace: a v7/legacy abort marked the model dead and
		//     took the claim's abort lock (MarkMigratingModelAsDead), and this
		//     undertaker-driven teardown owns releasing it. The staged-deletion
		//     guard preserves the v8 invariant that an aborting claim outlives a
		//     proven model-database drop: if a v8 finalizer has staged the drop,
		//     it owns the claim and the generic path must not release it early.
		// An activating claim is never released here - it is owned by the v8
		// activation finalizer.
		`WITH deletable_claim AS (
		     SELECT mmi.uuid
		     FROM   model_migration_import AS mmi
		     JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
		     WHERE  mmi.model_uuid = $entityUUID.uuid
		     AND    (mmipt.type = 'importing'
		         OR (mmipt.type = 'aborting'
		             AND NOT EXISTS (SELECT 1 FROM model_database_deletion mdd
		                             WHERE mdd.namespace = $entityUUID.uuid)))
		 )
		 DELETE FROM model_migration_import_offer
		 WHERE migration_uuid IN (SELECT uuid FROM deletable_claim)`,
		`WITH deletable_claim AS (
		     SELECT mmi.uuid
		     FROM   model_migration_import AS mmi
		     JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
		     WHERE  mmi.model_uuid = $entityUUID.uuid
		     AND    (mmipt.type = 'importing'
		         OR (mmipt.type = 'aborting'
		             AND NOT EXISTS (SELECT 1 FROM model_database_deletion mdd
		                             WHERE mdd.namespace = $entityUUID.uuid)))
		 )
		 DELETE FROM model_migration_import_external_controller_model
		 WHERE migration_uuid IN (SELECT uuid FROM deletable_claim)`,
		`WITH deletable_claim AS (
		     SELECT mmi.uuid
		     FROM   model_migration_import AS mmi
		     JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
		     WHERE  mmi.model_uuid = $entityUUID.uuid
		     AND    (mmipt.type = 'importing'
		         OR (mmipt.type = 'aborting'
		             AND NOT EXISTS (SELECT 1 FROM model_database_deletion mdd
		                             WHERE mdd.namespace = $entityUUID.uuid)))
		 )
		 DELETE FROM model_migration_import
		 WHERE model_uuid = $entityUUID.uuid
		 AND uuid IN (SELECT uuid FROM deletable_claim)`,
	}

	for _, table := range tables {
		stmt, err := st.Prepare(table, modelUUIDRec)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, modelUUIDRec).Run(); err != nil {
			return errors.Errorf("deleting reference to model in table %q: %w", table, err)
		}
	}
	return nil
}
