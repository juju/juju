// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/internal/errors"
)

// GetRemoteApplicationOffererUUIDByApplicationUUID returns the remote
// application offerer UUID associated with the input application UUID.
func (st *State) GetRemoteApplicationOffererUUIDByApplicationUUID(ctx context.Context, appUUID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var remoteAppOffererUUID entityUUID
	ident := entityUUID{UUID: appUUID}
	stmt, err := st.Prepare(`
SELECT aro.uuid AS &entityUUID.uuid
FROM   application_remote_offerer AS aro
WHERE  aro.application_uuid = $entityUUID.uuid
`, ident)
	if err != nil {
		return "", errors.Errorf("preparing remote application offerer UUID query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).Get(&remoteAppOffererUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return crossmodelrelationerrors.RemoteApplicationNotFound
		} else if err != nil {
			return errors.Errorf("running remote application offerer UUID query: %w", err)
		}
		return nil
	}); err != nil {
		return "", errors.Capture(err)
	}

	return remoteAppOffererUUID.UUID, nil
}

// GetRemoteApplicationOfferer returns true if a remote application exists
// with the input UUID.
func (st *State) RemoteApplicationOffererExists(ctx context.Context, rUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	remoteAppOffererUUID := entityUUID{UUID: rUUID}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   application_remote_offerer
WHERE  uuid = $entityUUID.uuid`, remoteAppOffererUUID)
	if err != nil {
		return false, errors.Errorf("preparing remote application offerer exists query: %w", err)
	}

	var remoteApplicationOffererExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, remoteAppOffererUUID).Get(&remoteAppOffererUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running remote application offerer exists query: %w", err)
		}

		remoteApplicationOffererExists = true
		return nil
	})

	return remoteApplicationOffererExists, errors.Capture(err)
}

// EnsureRemoteApplicationOffererNotAliveCascade ensures that there is no
// remote application offerer identified by the input UUID, that is still
// alive.
func (st *State) EnsureRemoteApplicationOffererNotAliveCascade(
	ctx context.Context, rUUID string,
) (internal.CascadedRemoteApplicationOffererLives, error) {
	var res internal.CascadedRemoteApplicationOffererLives

	db, err := st.DB(ctx)
	if err != nil {
		return res, errors.Capture(err)
	}

	remoteAppOffererUUID := entityUUID{UUID: rUUID}
	updateRemoteAppOffererStmt, err := st.Prepare(`
UPDATE application_remote_offerer
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, remoteAppOffererUUID)
	if err != nil {
		return res, errors.Errorf("preparing remote application offerer life update: %w", err)
	}

	// Also ensure that any other entities that are associated with the remote
	// application offerer are also set to dying. This has to be done in a
	// single transaction because we want to ensure that the remote application
	// offerer is not alive, and that no units are alive at the same time.
	// Preventing any races.
	selectRelationUUIDsStmt, err := st.Prepare(`
SELECT r.uuid AS &entityUUID.uuid
FROM   v_relation_endpoint AS re
JOIN   relation AS r ON re.relation_uuid = r.uuid
JOIN   application_remote_offerer AS aro ON re.application_uuid = aro.application_uuid
WHERE  r.life_id = 0
	`, entityUUID{})
	if err != nil {
		return res, errors.Errorf("preparing relation uuids query: %w", err)
	}

	updateRelationStmt, err := st.Prepare(`
UPDATE relation
SET    life_id = 1
WHERE  uuid IN ($uuids[:])
AND    life_id = 0`, uuids{})
	if err != nil {
		return res, errors.Errorf("preparing relation life update: %w", err)
	}

	if err := errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, updateRemoteAppOffererStmt, remoteAppOffererUUID).Run(); err != nil {
			return errors.Errorf("advancing remote application offerer life: %w", err)
		}

		var relationUUIDs []entityUUID
		err := tx.Query(ctx, selectRelationUUIDsStmt).GetAll(&relationUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("selecting relation UUIDs: %w", err)
		}
		res.RelationUUIDs = transform.Slice(relationUUIDs, func(e entityUUID) string { return e.UUID })

		if len(res.RelationUUIDs) > 0 {
			if err := tx.Query(ctx, updateRelationStmt, uuids(res.RelationUUIDs)).Run(); err != nil {
				return errors.Errorf("advancing relation life: %w", err)
			}
		}

		return nil
	})); err != nil {
		return res, errors.Capture(err)
	}

	return res, nil
}

// RemoteApplicationOffererScheduleRemoval schedules a removal job for the
// remote application offerer with the input UUID, qualified with the input
// force boolean.
func (st *State) RemoteApplicationOffererScheduleRemoval(
	ctx context.Context, removalUUID, rUUID string, force bool, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.RemoteApplicationOffererJob),
		EntityUUID:    rUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing remote application offerer removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling remote application offerer removal: %w", err)
		}
		return nil
	}))
}

// GetRemoteApplicationOffererLife returns the life of the remote application
// offerer with the input UUID.
func (st *State) GetRemoteApplicationOffererLife(ctx context.Context, rUUID string) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var l life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		l, err = st.getRemoteApplicationOffererLife(ctx, tx, rUUID)
		return err
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return l, nil
}

// DeleteRemoteApplicationOfferer removes a remote application offerer from
// the database completely. This also removes the synthetic application,
// unit and charm associated with the remote application offerer.
func (st *State) DeleteRemoteApplicationOfferer(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	remoteAppOffererUUID := entityUUID{UUID: rUUID}

	relationsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM v_relation_endpoint AS re
JOIN application_remote_offerer AS aro ON re.application_uuid = aro.application_uuid
WHERE aro.uuid = $entityUUID.uuid
`, count{}, remoteAppOffererUUID)
	if err != nil {
		return errors.Capture(err)
	}

	selectSynthAppStmt, err := st.Prepare(`
SELECT application_uuid AS &entityUUID.uuid
FROM   application_remote_offerer
WHERE  uuid = $entityUUID.uuid
`, remoteAppOffererUUID)
	if err != nil {
		return errors.Capture(err)
	}

	deleteRemoteApplicationOffererStatusStmt, err := st.Prepare(`
DELETE FROM application_remote_offerer_status
WHERE  application_remote_offerer_uuid = $entityUUID.uuid`, remoteAppOffererUUID)
	if err != nil {
		return errors.Capture(err)
	}

	deleteRemoteApplicationOffererStmt, err := st.Prepare(`
DELETE FROM application_remote_offerer
WHERE  uuid = $entityUUID.uuid`, remoteAppOffererUUID)
	if err != nil {
		return errors.Capture(err)
	}

	deleteApplicationEndpointBindingsStmt, err := st.Prepare(`
DELETE FROM application_endpoint
WHERE application_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	deleteApplicationStmt, err := st.Prepare(`
DELETE FROM application
WHERE  uuid = $entityUUID.uuid`, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing application delete: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		aLife, err := st.getRemoteApplicationOffererLife(ctx, tx, rUUID)
		if err != nil {
			return errors.Errorf("getting remote application offerer life: %w", err)
		} else if aLife == life.Alive {
			// The remote application offerer is still alive, we cannot delete it.
			return errors.Errorf("cannot delete remote application offerer %q as it is still alive", rUUID).
				Add(removalerrors.EntityStillAlive)
		}

		var numRelations count
		err = tx.Query(ctx, relationsStmt, remoteAppOffererUUID).Get(&numRelations)
		if err != nil {
			return errors.Errorf("getting number of relations for remote application offerer: %w", err)
		} else if numRelations.Count > 0 {
			// It is required that all relations have been completely removed
			// before the remote application offerer can be removed.
			return errors.Errorf("cannot delete remote application offerer as it still has %d relation(s)", numRelations.Count).
				Add(crossmodelrelationerrors.RemoteApplicationHasRelations).
				Add(removalerrors.RemovalJobIncomplete)
		}

		var synthApp entityUUID
		err = tx.Query(ctx, selectSynthAppStmt, remoteAppOffererUUID).Get(&synthApp)
		if err != nil {
			return errors.Errorf("getting application UUID for remote application offerer: %w", err)
		}

		// Get the charm UUID before we delete the application.
		synthCharmUUID, err := st.getCharmUUIDForApplication(ctx, tx, synthApp.UUID)
		if err != nil {
			return errors.Errorf("getting charm UUID for application: %w", err)
		}

		if err := st.deleteRemoteApplicationOffererUnits(ctx, tx, synthApp.UUID); err != nil {
			return errors.Errorf("deleting remote application offerer units: %w", err)
		}

		if err := tx.Query(ctx, deleteRemoteApplicationOffererStatusStmt, remoteAppOffererUUID).Run(); err != nil {
			return errors.Errorf("deleting synthetic application remote relation status: %w", err)
		}

		if err := tx.Query(ctx, deleteRemoteApplicationOffererStmt, remoteAppOffererUUID).Run(); err != nil {
			return errors.Errorf("deleting synthetic application remote relation: %w", err)
		}

		if err := tx.Query(ctx, deleteApplicationEndpointBindingsStmt, synthApp).Run(); err != nil {
			return errors.Errorf("deleting synthetic application endpoint bindings: %w", err)
		}

		if err := tx.Query(ctx, deleteApplicationStmt, synthApp).Run(); err != nil {
			return errors.Errorf("deleting synthetic application: %w", err)
		}

		// Delete the synthetic charm directly, since we know it is not shared
		// but other applications
		if err := st.deleteCharm(ctx, tx, synthCharmUUID); err != nil {
			return errors.Errorf("deleting synthetic charm: %w", err)
		}

		return nil
	}))
}

func (st *State) deleteRemoteApplicationOffererUnits(ctx context.Context, tx *sqlair.TX, appUUID string) error {
	ident := entityUUID{UUID: appUUID}
	selectNetNodesStmt, err := st.Prepare(`
SELECT DISTINCT nn.uuid AS &entityUUID.uuid
FROM   net_node AS nn
JOIN   unit AS u ON nn.uuid = u.net_node_uuid
JOIN   application AS a ON u.application_uuid = a.uuid
WHERE  a.uuid = $entityUUID.uuid
`, ident)
	if err != nil {
		return errors.Capture(err)
	}

	deleteNetNodesStmt, err := st.Prepare(`
DELETE FROM net_node
WHERE uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	deleteUnitsStmt, err := st.Prepare(`
DELETE FROM unit
WHERE application_uuid = $entityUUID.uuid`, ident)
	if err != nil {
		return errors.Capture(err)
	}

	var netNodeEntityUUIDs []entityUUID
	err = tx.Query(ctx, selectNetNodesStmt, ident).GetAll(&netNodeEntityUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Capture(err)
	}

	// no net nodes means no units either, so we can return early.
	if len(netNodeEntityUUIDs) == 0 {
		return nil
	}

	if err := tx.Query(ctx, deleteUnitsStmt, ident).Run(); err != nil {
		return errors.Capture(err)
	}

	netNodeUUIDs := uuids(transform.Slice(netNodeEntityUUIDs, func(e entityUUID) string { return e.UUID }))
	if err := tx.Query(ctx, deleteNetNodesStmt, netNodeUUIDs).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) getRemoteApplicationOffererLife(ctx context.Context, tx *sqlair.TX, rUUID string) (life.Life, error) {
	var remoteApplicationOffererLife entityLife
	remoteAppOffererUUID := entityUUID{UUID: rUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   application_remote_offerer
WHERE  uuid = $entityUUID.uuid;`, remoteApplicationOffererLife, remoteAppOffererUUID)
	if err != nil {
		return -1, errors.Errorf("preparing remote application offerer life query: %w", err)
	}

	err = tx.Query(ctx, stmt, remoteAppOffererUUID).Get(&remoteApplicationOffererLife)
	if errors.Is(err, sqlair.ErrNoRows) {
		return -1, crossmodelrelationerrors.RemoteApplicationNotFound
	} else if err != nil {
		return -1, errors.Errorf("running remote application offerer life query: %w", err)
	}

	return life.Life(remoteApplicationOffererLife.Life), nil
}
