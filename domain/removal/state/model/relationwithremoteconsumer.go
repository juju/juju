// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/internal/errors"
)

// RelationWithRemoteConsumerExists returns true if a relation exists with the input
// UUID, and relates a synthetic application
func (st *State) RelationWithRemoteConsumerExists(ctx context.Context, rUUID string) (bool, error) {
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

// EnsureRelationWithRemoteConsumerNotAliveCascade ensures that the relation identified
// by the input UUID is not alive, and sets the synthetic units in scope
// of this relation to dead. We do this because synthetic units do not have a
// uniter, so we need to handle their life ourselves.
func (st *State) EnsureRelationWithRemoteConsumerNotAliveCascade(ctx context.Context, rUUID string) (internal.CascadedRelationWithRemoteConsumerLives, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return internal.CascadedRelationWithRemoteConsumerLives{}, errors.Capture(err)
	}

	remoteRelationUUID := entityUUID{UUID: rUUID}
	updateRelationStmt, err := st.Prepare(`
UPDATE relation
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, remoteRelationUUID)
	if err != nil {
		return internal.CascadedRelationWithRemoteConsumerLives{}, errors.Errorf("preparing remote relation life update: %w", err)
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
		return internal.CascadedRelationWithRemoteConsumerLives{},
			errors.Errorf("preparing remote relation synthetic application UUID query: %w", err)
	}

	getSyntheticRelationUnitUUIDsStmt, err := st.Prepare(`
SELECT ru.uuid AS &entityUUID.uuid
FROM   relation_unit AS ru
JOIN   unit AS u ON ru.unit_uuid = u.uuid
WHERE  u.application_uuid = $entityUUID.uuid
	`, entityUUID{})
	if err != nil {
		return internal.CascadedRelationWithRemoteConsumerLives{}, errors.Errorf("preparing remote relation synthetic unit UUID query: %w", err)
	}

	updateSyntheticUnitStmt, err := st.Prepare(`
UPDATE unit
SET    life_id = 2
WHERE  life_id = 0
AND    application_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return internal.CascadedRelationWithRemoteConsumerLives{}, errors.Errorf("preparing remote relation synthetic unit update: %w", err)
	}

	var synthRelationUnitUUIDs []entityUUID
	err = errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, updateRelationStmt, remoteRelationUUID).Run()
		if err != nil {
			return errors.Errorf("advancing remote relation life: %w", err)
		}

		var synthAppUUID entityUUID
		err = tx.Query(ctx, getSyntheticAppUUIDStmt, remoteRelationUUID).Get(&synthAppUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting synthetic application UUID: %w", err)
		}

		err = tx.Query(ctx, getSyntheticRelationUnitUUIDsStmt, synthAppUUID).GetAll(&synthRelationUnitUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting synthetic relation unit UUIDs: %w", err)
		}

		err = tx.Query(ctx, updateSyntheticUnitStmt, synthAppUUID).Run()
		if err != nil {
			return errors.Errorf("advancing remote relation synthetic unit life: %w", err)
		}

		return nil
	}))
	if err != nil {
		return internal.CascadedRelationWithRemoteConsumerLives{}, errors.Capture(err)
	}

	return internal.CascadedRelationWithRemoteConsumerLives{
		SyntheticRelationUnitUUIDs: transform.Slice(synthRelationUnitUUIDs, func(eu entityUUID) string { return eu.UUID }),
	}, nil
}

// RelationWithRemoteConsumerScheduleRemoval schedules a removal job for the relation
// with the input UUID, qualified with the input force boolean.
func (st *State) RelationWithRemoteConsumerScheduleRemoval(ctx context.Context, removalUUID, relUUID string, force bool, when time.Time) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.RelationWithRemoteConsumerJob),
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

// DeleteRelationWithRemoteConsumer deletes a remote relation record under and all it's
// and anything dependent upon it. This includes synthetic units.
func (st *State) DeleteRelationWithRemoteConsumer(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	remoteRelationUUID := entityUUID{UUID: rUUID}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.deleteRelationWithRemoteConsumer(ctx, tx, remoteRelationUUID)
	}))
}

func (st *State) deleteRelationWithRemoteConsumer(ctx context.Context, tx *sqlair.TX, remoteRelationUUID entityUUID) error {
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
		return errors.Errorf("preparing remote relation application UUID query: %w", err)
	}

	deleteRelationNetworkIngressStmt, err := st.Prepare(`
DELETE FROM relation_network_ingress
WHERE  relation_uuid = $entityUUID.uuid`, remoteRelationUUID)
	if err != nil {
		return errors.Errorf("preparing relation network egress deletion: %w", err)
	}

	deleteRemoteApplicationConsumerStmt, err := st.Prepare(`
DELETE FROM application_remote_consumer
WHERE offer_connection_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing delete remote application consumer query: %w", err)
	}

	deleteOfferConnectionStmt, err := st.Prepare(`
DELETE FROM offer_connection
WHERE remote_relation_uuid = $entityUUID.uuid
`, remoteRelationUUID)
	if err != nil {
		return errors.Errorf("preparing offer connection deletion: %w", err)
	}

	var synthAppUUID entityUUID
	err = tx.Query(ctx, getSyntheticAppUUIDStmt, remoteRelationUUID).Get(&synthAppUUID)
	if err != nil {
		return errors.Errorf("getting application UUID: %w", err)
	}

	err = tx.Query(ctx, deleteRelationNetworkIngressStmt, remoteRelationUUID).Run()
	if err != nil {
		return errors.Errorf("running relation network egress deletion: %w", err)
	}

	err = tx.Query(ctx, deleteRemoteApplicationConsumerStmt, synthAppUUID).Run()
	if err != nil {
		return errors.Errorf("deleting synthetic application remote consumer: %w", err)
	}

	err = tx.Query(ctx, deleteOfferConnectionStmt, remoteRelationUUID).Run()
	if err != nil {
		return errors.Errorf("running offer connection deletion: %w", err)
	}

	err = st.deleteRelation(ctx, tx, remoteRelationUUID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := st.deleteSynthApplication(ctx, tx, synthAppUUID); err != nil {
		return errors.Errorf("deleting synthetic application: %w", err)
	}

	return nil
}
