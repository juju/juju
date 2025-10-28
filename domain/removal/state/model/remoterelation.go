// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
)

// RemoteRelationExists returns true if a relation exists with the input
// UUID, and relates a synthetic application
func (st *State) RemoteRelationExists(ctx context.Context, rUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	remoteRelationUUID := entityUUID{UUID: rUUID}
	existsStmt, err := st.Prepare(`
SELECT r.uuid AS &entityUUID.uuid
FROM   relation AS r
JOIN   relation_endpoint AS re ON r.uuid = re.relation_uuid
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
JOIN   charm AS c ON a.charm_uuid = c.uuid
JOIN   charm_source AS cs ON c.source_id = cs.id
WHERE  r.uuid = $entityUUID.uuid
AND    cs.name = 'cmr'`, remoteRelationUUID)
	if err != nil {
		return false, errors.Errorf("preparing remote relation exists query: %w", err)
	}

	var remoteRelationExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, remoteRelationUUID).Get(&remoteRelationUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running remote relation exists query: %w", err)
		}

		remoteRelationExists = true
		return nil
	})

	return remoteRelationExists, errors.Capture(err)
}

// EnsureRemoteRelationNotAliveCascade ensures that the relation identified
// by the input UUID is not alive, and sets the synthetic units in scope
// of this relation to dead
// NOTE: We do not return any artifacts here, as the only entities that are
// cascaded are synthetic units, which do no need a removal job scheduled.
func (st *State) EnsureRemoteRelationNotAliveCascade(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	remoteRelationUUID := entityUUID{UUID: rUUID}
	updateRelationStmt, err := st.Prepare(`
UPDATE relation
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, remoteRelationUUID)
	if err != nil {
		return errors.Errorf("preparing remote relation life update: %w", err)
	}

	getSyntheticAppUUIDStmt, err := st.Prepare(`
SELECT a.uuid AS &entityUUID.uuid
FROM   relation AS r
JOIN   relation_endpoint AS re ON r.uuid = re.relation_uuid
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
JOIN   charm AS c ON a.charm_uuid = c.uuid
JOIN   charm_source AS cs ON c.source_id = cs.id
WHERE  r.uuid = $entityUUID.uuid
AND    cs.name = 'cmr'`, remoteRelationUUID)
	if err != nil {
		return errors.Errorf("preparing remote relation synthetic app UUID query: %w", err)
	}

	updateSyntheticUnitStmt, err := st.Prepare(`
UPDATE unit
SET    life_id = 2
WHERE  life_id = 0
AND    application_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing remote relation synthetic unit update: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, updateRelationStmt, remoteRelationUUID).Run()
		if err != nil {
			return errors.Errorf("advancing remote relation life: %w", err)
		}

		var synthAppUUID entityUUID
		err = tx.Query(ctx, getSyntheticAppUUIDStmt, remoteRelationUUID).Get(&synthAppUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting synthetic app UUID: %w", err)
		}

		err = tx.Query(ctx, updateSyntheticUnitStmt, synthAppUUID).Run()
		if err != nil {
			return errors.Errorf("advancing remote relation synthetic unit life: %w", err)
		}

		return nil
	}))
}

// RemoteRelationScheduleRemoval schedules a removal job for the relation
// with the input UUID, qualified with the input force boolean.
func (st *State) RemoteRelationScheduleRemoval(ctx context.Context, removalUUID, relUUID string, force bool, when time.Time) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.RemoteRelationJob),
		EntityUUID:    relUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing remote relation removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling remote relation removal: %w", err)
		}
		return nil
	}))
}

// DeleteRemoteRelation deletes a remote relation record under and all it's
// and anything dependent upon it. This includes synthetic units.
func (st *State) DeleteRemoteRelation(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	remoteRelationUUID := entityUUID{UUID: rUUID}

	getSyntheticAppUUIDStmt, err := st.Prepare(`
SELECT a.uuid AS &entityUUID.uuid
FROM   relation AS r
JOIN   relation_endpoint AS re ON r.uuid = re.relation_uuid
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
JOIN   charm AS c ON a.charm_uuid = c.uuid
JOIN   charm_source AS cs ON c.source_id = cs.id
WHERE  r.uuid = $entityUUID.uuid
AND    cs.name = 'cmr'`, remoteRelationUUID)
	if err != nil {
		return errors.Errorf("preparing remote relation app UUID query: %w", err)
	}

	countSyntheticAppRelationsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM   relation_endpoint AS re
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
WHERE  ae.application_uuid = $entityUUID.uuid
`, count{}, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing remote relation app relation count query: %w", err)
	}

	deleteRelationNetworkEgressStmt, err := st.Prepare(`
DELETE FROM relation_network_egress
WHERE  relation_uuid = $entityUUID.uuid`, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing relation network egress deletion: %w", err)
	}

	deleteRemoteOffererRelationMacaroonStmt, err := st.Prepare(`
DELETE FROM application_remote_offerer_relation_macaroon
WHERE  relation_uuid = $entityUUID.uuid`, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing remote relation macaroon deletion: %w", err)
	}

	deleteSyntheticUnitsStmt, err := st.Prepare(`
DELETE FROM unit
WHERE  application_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing remote relation unit deletion: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var synthAppUUID entityUUID
		err = tx.Query(ctx, getSyntheticAppUUIDStmt, remoteRelationUUID).Get(&synthAppUUID)
		if err != nil {
			return errors.Errorf("getting app UUID: %w", err)
		}

		err = tx.Query(ctx, deleteRelationNetworkEgressStmt, remoteRelationUUID).Run()
		if err != nil {
			return errors.Errorf("running relation network egress deletion: %w", err)
		}

		err = tx.Query(ctx, deleteRemoteOffererRelationMacaroonStmt, remoteRelationUUID).Run()
		if err != nil {
			return errors.Errorf("running remote relation macaroon deletion: %w", err)
		}

		err := st.deleteRelation(ctx, tx, remoteRelationUUID)
		if err != nil {
			return errors.Capture(err)
		}

		var relationCount count
		err = tx.Query(ctx, countSyntheticAppRelationsStmt, synthAppUUID).Get(&relationCount)
		if err != nil {
			return errors.Errorf("getting relation count: %w", err)
		}

		if relationCount.Count == 0 {
			err = tx.Query(ctx, deleteSyntheticUnitsStmt, synthAppUUID).Run()
			if err != nil {
				return errors.Errorf("running unit deletion: %w", err)
			}
		}

		return nil
	}))
}
