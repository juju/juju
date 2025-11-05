// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// RelationExists returns true if a relation exists with the input UUID. Returns
// false (with an error) if the relation is a cross model relation.
//
// The following error types can be expected to be returned:
//   - [removalerrors.RelationIsCrossModel] if the relation is a cross model
//     relation.
func (st *State) RelationExists(ctx context.Context, rUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	relationUUID := entityUUID{UUID: rUUID}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   relation
WHERE  uuid = $entityUUID.uuid`, relationUUID)
	if err != nil {
		return false, errors.Errorf("preparing relation exists query: %w", err)
	}

	checkCharmSourcesStmt, err := st.Prepare(`
SELECT COUNT(cs.name) AS &count.count
FROM   charm_source AS cs
JOIN   charm AS c ON cs.id = c.source_id
JOIN   application AS a ON c.uuid = a.charm_uuid
JOIN   application_endpoint AS ae ON a.uuid = ae.application_uuid
JOIN   relation_endpoint AS re ON ae.uuid = re.endpoint_uuid
WHERE  re.relation_uuid = $entityUUID.uuid
AND    cs.name = 'cmr'
	`, count{}, relationUUID)
	if err != nil {
		return false, errors.Errorf("preparing relation exists query: %w", err)
	}

	var (
		relationExists bool
		cmrEpCount     count
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, relationUUID).Get(&relationUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running relation exists query: %w", err)
		}

		err = tx.Query(ctx, checkCharmSourcesStmt, relationUUID).Get(&cmrEpCount)
		if err != nil {
			return errors.Errorf("running relation exists query: %w", err)
		}

		relationExists = true
		return nil
	})

	if cmrEpCount.Count > 0 {
		return false, errors.Errorf("relation %q is a cross model relation", rUUID).
			Add(removalerrors.RelationIsCrossModel)
	}

	return relationExists, errors.Capture(err)
}

// EnsureRelationNotAlive ensures that there is no relation
// identified by the input UUID, that is still alive.
func (st *State) EnsureRelationNotAlive(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	relationUUID := entityUUID{UUID: rUUID}
	stmt, err := st.Prepare(`
UPDATE relation
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation life update: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, relationUUID).Run()
		if err != nil {
			return errors.Errorf("advancing relation life: %w", err)
		}
		return nil
	}))
}

// RelationScheduleRemoval schedules a removal job for the relation with the
// input UUID, qualified with the input force boolean.
// We don't care if the relation does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) RelationScheduleRemoval(
	ctx context.Context, removalUUID, relUUID string, force bool, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.RelationJob),
		EntityUUID:    relUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing relation removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling relation removal: %w", err)
		}
		return nil
	}))
}

// GetRelationLife returns the life of the relation with the input UUID.
func (st *State) GetRelationLife(ctx context.Context, rUUID string) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var relationLife entityLife
	relationUUID := entityUUID{UUID: rUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   relation
WHERE  uuid = $entityUUID.uuid`, relationLife, relationUUID)
	if err != nil {
		return -1, errors.Errorf("preparing relation life query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, relationUUID).Get(&relationLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		} else if err != nil {
			return errors.Errorf("running relation life query: %w", err)
		}

		return nil
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return life.Life(relationLife.Life), nil
}

// UnitNamesInScope returns the names of units in the scope of the relation
// with the input UUID. Does not return synthetic units (i.e. units that
// represent units in another model, for the purpose of modelling CMRs),
// since synthetic units are not meaningfully in scope in the context of
// this model.
func (st *State) UnitNamesInScope(ctx context.Context, rUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type uName struct {
		Name string `db:"name"`
	}

	relationUUID := entityUUID{UUID: rUUID}

	stmt, err := st.Prepare(`
SELECT u.name AS &uName.name 
FROM   relation_endpoint re 
JOIN   relation_unit ru ON re.uuid = ru.relation_endpoint_uuid
JOIN   unit u on ru.unit_uuid = u.uuid 
JOIN   charm c ON u.charm_uuid = c.uuid
JOIN   charm_source cs ON c.source_id = cs.id
WHERE  re.relation_uuid = $entityUUID.uuid
AND    cs.name != 'cmr'`, uName{}, relationUUID)
	if err != nil {
		return nil, errors.Errorf("preparing relation scopes query: %w", err)
	}

	var inScope []uName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, relationUUID).GetAll(&inScope)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running relation scopes query: %w", err)
		}

		return nil
	})

	return transform.Slice(inScope, func(n uName) string { return n.Name }), errors.Capture(err)
}

// DeleteRelationUnits deletes all relation unit records and their associated
// settings for a relation. It effectively departs all units from the scope of
// the input relation immediately.
// This does not write unit settings to the archive because this method is only
// called for forced relation removal.
// I.e. the operator has explicitly eschewed the normal death workflow.
func (st *State) DeleteRelationUnits(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	relationUUID := entityUUID{UUID: rUUID}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.deleteRelationUnitsForRelation(ctx, tx, relationUUID)
	}))
}

// DeleteRelation removes a relation from the database completely.
// Note that if any units are in scope, this will return a
// constraint violation error.
func (st *State) DeleteRelation(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	relationUUID := entityUUID{UUID: rUUID}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.deleteRelation(ctx, tx, relationUUID)
	}))
}

func (st *State) deleteRelation(ctx context.Context, tx *sqlair.TX, relationUUID entityUUID) error {
	settingsStmt, err := st.Prepare(`
DELETE FROM relation_application_setting
WHERE  relation_endpoint_uuid IN (
    SELECT uuid FROM relation_endpoint WHERE relation_uuid = $entityUUID.uuid
)`, relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation app settings deletion: %w", err)
	}

	settingsHashStmt, err := st.Prepare(`
DELETE FROM relation_application_settings_hash
WHERE  relation_endpoint_uuid IN (
    SELECT uuid FROM relation_endpoint WHERE relation_uuid = $entityUUID.uuid
)`, relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation app settings deletion: %w", err)
	}

	endpointStmt, err := st.Prepare("DELETE FROM relation_endpoint WHERE relation_uuid = $entityUUID.uuid", relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation endpoint deletion: %w", err)
	}

	archiveStmt, err := st.Prepare(
		"DELETE FROM relation_unit_setting_archive WHERE relation_uuid = $entityUUID.uuid ", relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation unit settings archive deletion: %w", err)
	}

	statusStmt, err := st.Prepare("DELETE FROM relation_status WHERE relation_uuid = $entityUUID.uuid ", relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation status deletion: %w", err)
	}

	secretPermissionStmt, err := st.Prepare(`
DELETE FROM secret_permission
WHERE  scope_type_id = 3
AND    scope_uuid = $entityUUID.uuid`, relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation secret permission deletion: %w", err)
	}

	relStmt, err := st.Prepare("DELETE FROM relation WHERE uuid = $entityUUID.uuid ", relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation deletion: %w", err)
	}

	err = tx.Query(ctx, settingsStmt, relationUUID).Run()
	if err != nil {
		return errors.Errorf("running relation app settings deletion: %w", err)
	}

	err = tx.Query(ctx, settingsHashStmt, relationUUID).Run()
	if err != nil {
		return errors.Errorf("running relation app settings hash deletion: %w", err)
	}

	err = tx.Query(ctx, endpointStmt, relationUUID).Run()
	if err != nil {
		if database.IsErrConstraintForeignKey(err) {
			err = removalerrors.UnitsStillInScope
		}
		return errors.Errorf("running relation endpoint deletion: %w", err)
	}

	err = tx.Query(ctx, archiveStmt, relationUUID).Run()
	if err != nil {
		return errors.Errorf("running relation unit settings archive deletion: %w", err)
	}

	err = tx.Query(ctx, statusStmt, relationUUID).Run()
	if err != nil {
		return errors.Errorf("running relation status deletion: %w", err)
	}

	err = tx.Query(ctx, secretPermissionStmt, relationUUID).Run()
	if err != nil {
		return errors.Errorf("running relation secret permission deletion: %w", err)
	}

	err = tx.Query(ctx, relStmt, relationUUID).Run()
	if err != nil {
		return errors.Errorf("running relation deletion: %w", err)
	}

	return nil
}

// LeaveScope updates the relation to indicate that the unit represented by
// the input relation unit UUID is not in the implied relation scope.
// It archives the unit's relation settings, then deletes all associated
// relation unit records.
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUnitNotFound] if the relation unit is not found.
func (st *State) LeaveScope(ctx context.Context, relUnitUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	id := entityUUID{UUID: relUnitUUID}

	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   relation_unit
WHERE  uuid = $entityUUID.uuid`, id)
	if err != nil {
		return errors.Errorf("preparing relation unit exists query: %w", err)
	}

	isSyntheticStmt, err := st.Prepare(`
SELECT u.uuid AS &entityUUID.uuid
FROM   relation_unit AS re
JOIN   unit AS u ON re.unit_uuid = u.uuid
JOIN   charm AS c ON u.charm_uuid = c.uuid
JOIN   charm_source AS cs ON c.source_id = cs.id
WHERE  re.uuid = $entityUUID.uuid
AND    cs.name = 'cmr'
	`, entityUUID{})

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, id).Get(&id)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationUnitNotFound
		} else if err != nil {
			return errors.Errorf("running relation unit exists query: %w", err)
		}

		var synthUnitUUID entityUUID
		err = tx.Query(ctx, isSyntheticStmt, id).Get(&synthUnitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
		} else if err != nil {
			return errors.Errorf("running relation unit exists query: %w", err)
		}

		err = st.archiveRelationUnitSettings(ctx, tx, id)
		if err != nil {
			return errors.Capture(err)
		}

		err = st.deleteRelationUnit(ctx, tx, id)
		if err != nil {
			return errors.Capture(err)
		}

		if synthUnitUUID.UUID != "" {
			if err := st.deleteSynthUnit(ctx, tx, synthUnitUUID); err != nil {
				return errors.Capture(err)
			}
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) archiveRelationUnitSettings(ctx context.Context, tx *sqlair.TX, id entityUUID) error {
	delStmt, err := st.Prepare(`
DELETE FROM relation_unit_setting_archive
WHERE (relation_uuid, unit_name) IN (
    SELECT re.relation_uuid, u.name
    FROM   relation_unit ru
    JOIN   relation_endpoint re ON ru.relation_endpoint_uuid = re.uuid
    JOIN   unit u ON ru.unit_uuid = u.uuid
    WHERE  ru.uuid = $entityUUID.uuid
)`, id)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, delStmt, id).Run(); err != nil {
		return errors.Capture(err)
	}

	copyStmt, err := st.Prepare(`
INSERT INTO relation_unit_setting_archive (relation_uuid, unit_name, "key", value)
SELECT re.relation_uuid, u.name, rus."key", rus.value
FROM   relation_unit ru
JOIN   relation_endpoint re ON ru.relation_endpoint_uuid = re.uuid
JOIN   unit u ON ru.unit_uuid = u.uuid
JOIN   relation_unit_setting rus ON ru.uuid = rus.relation_unit_uuid
WHERE  ru.uuid = $entityUUID.uuid`, id)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, copyStmt, id).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) deleteRelationUnit(ctx context.Context, tx *sqlair.TX, id entityUUID) error {
	deleteSettingsStmt, err := st.Prepare(`
DELETE FROM relation_unit_setting
WHERE relation_unit_uuid = $entityUUID.uuid`, id)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, deleteSettingsStmt, id).Run()
	if err != nil {
		return errors.Capture(err)
	}

	deleteSettingsHashStmt, err := st.Prepare(`
DELETE FROM relation_unit_settings_hash
WHERE relation_unit_uuid = $entityUUID.uuid`, id)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, deleteSettingsHashStmt, id).Run()
	if err != nil {
		return errors.Capture(err)
	}

	deleteRelationUnitStmt, err := st.Prepare(`
DELETE FROM relation_unit 
WHERE uuid = $entityUUID.uuid`, id)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, deleteRelationUnitStmt, id).Get(&outcome)
	if err != nil {
		return errors.Capture(err)
	}

	rows, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	} else if rows != 1 {
		return errors.Errorf("deleting relation unit: expected 1 row affected, got %d", rows)
	}

	return nil
}

func (st *State) deleteRelationUnitsForRelation(ctx context.Context, tx *sqlair.TX, relationUUID entityUUID) error {
	settingsStmt, err := st.Prepare(`
WITH rru AS (
    SELECT ru.uuid, re.relation_uuid
    FROM   relation_unit ru 
    JOIN relation_endpoint re ON ru.relation_endpoint_uuid = re.uuid
)
DELETE FROM relation_unit_setting
WHERE  relation_unit_uuid IN (
    SELECT uuid FROM rru WHERE relation_uuid = $entityUUID.uuid
)`, relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation unit settings deletion: %w", err)
	}

	settingsHashStmt, err := st.Prepare(`
WITH rru AS (
    SELECT ru.uuid, re.relation_uuid
    FROM   relation_unit ru 
    JOIN relation_endpoint re ON ru.relation_endpoint_uuid = re.uuid
)
DELETE FROM relation_unit_settings_hash
WHERE  relation_unit_uuid IN (
    SELECT uuid FROM rru WHERE relation_uuid = $entityUUID.uuid
)`, relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation unit settings hash deletion: %w", err)
	}

	ruStmt, err := st.Prepare(`
DELETE FROM relation_unit 
WHERE  relation_endpoint_uuid IN (
    SELECT uuid FROM relation_endpoint WHERE relation_uuid = $entityUUID.uuid
)`, relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation unit deletion: %w", err)
	}

	err = tx.Query(ctx, settingsStmt, relationUUID).Run()
	if err != nil {
		return errors.Errorf("running relation unit settings deletion: %w", err)
	}

	err = tx.Query(ctx, settingsHashStmt, relationUUID).Run()
	if err != nil {
		return errors.Errorf("running relation unit settings hash deletion: %w", err)
	}

	err = tx.Query(ctx, ruStmt, relationUUID).Run()
	if err != nil {
		return errors.Errorf("running relation unit deletion: %w", err)
	}

	return nil
}
