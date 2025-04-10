// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// RelationExists returns true if a relation exists with the input UUID.
func (st *State) RelationExists(ctx context.Context, rUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	relationUUID := entityUUID{UUID: rUUID}
	existsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   relation
WHERE  uuid = $entityUUID.uuid`, relationUUID)
	if err != nil {
		return false, errors.Errorf("preparing relation exists query: %w", err)
	}

	var relationExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, relationUUID).Get(&relationUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running relation exists query: %w", err)
		}

		relationExists = true
		return nil
	})

	return relationExists, errors.Capture(err)
}

// EnsureRelationNotAlive ensures that there is no relation
// identified by the input UUID, that is still alive.
func (st *State) EnsureRelationNotAlive(ctx context.Context, rUUID string) error {
	db, err := st.DB()
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
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: 0,
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
	db, err := st.DB()
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

	return relationLife.Life, errors.Capture(err)
}

// UnitNamesInScope returns the names of units in
// the scope of the relation with the input UUID.
func (st *State) UnitNamesInScope(ctx context.Context, rUUID string) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	type uName struct {
		Name string `db:"name"`
	}

	relationUUID := entityUUID{UUID: rUUID}

	stmt, err := st.Prepare(`
SELECT &uName.name 
FROM   relation_endpoint re 
       JOIN relation_unit ru ON re.uuid = ru.relation_endpoint_uuid
       JOIN unit u on ru.unit_uuid = u.uuid 
WHERE  re.relation_uuid = $entityUUID.uuid`, uName{}, relationUUID)
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

// DeleteRelationUnits deletes all relation unit records and their
// associated settings for a relation. It effectively departs all
// units from the scope of the input relation immediately.
func (st *State) DeleteRelationUnits(ctx context.Context, rUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	relationUUID := entityUUID{UUID: rUUID}

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

	ruStmt, err := st.Prepare(`
DELETE FROM relation_unit 
WHERE  relation_endpoint_uuid IN (
    SELECT uuid FROM relation_endpoint WHERE relation_uuid = $entityUUID.uuid
)`, relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation unit deletion: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, settingsStmt, relationUUID).Run()
		if err != nil {
			return errors.Errorf("running relation unit settings deletion: %w", err)
		}

		err = tx.Query(ctx, ruStmt, relationUUID).Run()
		if err != nil {
			return errors.Errorf("running relation unit deletion: %w", err)
		}

		return nil
	}))
}

// DeleteRelation removes a relation from the database completely.
// Note that if any units are in scope, this will return a
// constraint violation error.
func (st *State) DeleteRelation(ctx context.Context, rUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	relationUUID := entityUUID{UUID: rUUID}

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

	statusStmt, err := st.Prepare("DELETE FROM relation_status WHERE relation_uuid = $entityUUID.uuid ", relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation status deletion: %w", err)
	}

	relStmt, err := st.Prepare("DELETE FROM relation WHERE uuid = $entityUUID.uuid ", relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation deletion: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
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

		err = tx.Query(ctx, statusStmt, relationUUID).Run()
		if err != nil {
			return errors.Errorf("running relation status deletion: %w", err)
		}

		err = tx.Query(ctx, relStmt, relationUUID).Run()
		if err != nil {
			return errors.Errorf("running relation deletion: %w", err)
		}

		return nil
	}))
}
