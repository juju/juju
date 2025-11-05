// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// OfferExists returns true if an offer exists with the input UUID.
func (st *State) OfferExists(ctx context.Context, oUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	offerUUID := entityUUID{UUID: oUUID}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   offer
WHERE  uuid = $entityUUID.uuid`, offerUUID)
	if err != nil {
		return false, errors.Errorf("preparing offer exists query: %w", err)
	}

	var offerExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, offerUUID).Get(&offerUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running offer exists query: %w", err)
		}

		offerExists = true
		return nil
	})

	return offerExists, errors.Capture(err)
}

// DeleteOffer removes an offer from the database completely.
func (st *State) DeleteOffer(ctx context.Context, oUUID string, force bool) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	offerUUID := entityUUID{UUID: oUUID}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.deleteOffer(ctx, tx, offerUUID, force)
	})
	return errors.Capture(err)
}

func (st *State) deleteOffer(ctx context.Context, tx *sqlair.TX, offerUUID entityUUID, force bool) error {
	checkConnsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM offer_connection 
WHERE offer_uuid = $entityUUID.uuid
`, count{}, offerUUID)
	if err != nil {
		return errors.Errorf("preparing offer connection count query: %w", err)
	}

	getSynthRelationsStmt, err := st.Prepare(`
SELECT remote_relation_uuid AS &entityUUID.uuid
FROM   offer_connection
WHERE  offer_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing synthetic relations query: %w", err)
	}

	deleteRelationEndpointsStmt, err := st.Prepare(`
DELETE FROM relation_endpoint
WHERE relation_uuid IN ($uuids[:])
`, uuids{})
	if err != nil {
		return errors.Errorf("preparing delete relation endpoint query: %w", err)
	}

	deleteRelationStatusStmt, err := st.Prepare(`
DELETE FROM relation_status
WHERE relation_uuid IN ($uuids[:])
`, uuids{})
	if err != nil {
		return errors.Errorf("preparing delete relation status query: %w", err)
	}

	deleteRelationsStmt, err := st.Prepare(`
DELETE FROM relation
WHERE uuid IN ($uuids[:])
`, uuids{})
	if err != nil {
		return errors.Errorf("preparing delete relations query: %w", err)
	}

	// offer connection uuid is set to match it's the synthetic application uuid
	getSynthAppsStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   offer_connection
WHERE  offer_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing synthetic application query: %w", err)
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
WHERE offer_uuid = $entityUUID.uuid
`, offerUUID)
	if err != nil {
		return errors.Errorf("preparing delete offer connection query: %w", err)
	}

	deleteOfferEndpointsStmt, err := st.Prepare(`
DELETE FROM offer_endpoint
WHERE offer_uuid = $entityUUID.uuid
`, offerUUID)
	if err != nil {
		return errors.Errorf("preparing delete offer endpoints query: %w", err)
	}

	deleteOfferStmt, err := st.Prepare(`
DELETE FROM offer 
WHERE uuid = $entityUUID.uuid
`, offerUUID)
	if err != nil {
		return errors.Errorf("preparing delete offer query: %w", err)
	}

	if !force {
		var count count
		if err := tx.Query(ctx, checkConnsStmt, offerUUID).Get(&count); err != nil {
			return errors.Errorf("checking offer connections: %w", err)
		} else if count.Count > 0 {
			return errors.Errorf("cannot delete offer %q, it has %d connections", offerUUID, count.Count).
				Add(removalerrors.OfferHasRelations).
				Add(removalerrors.ForceRequired)
		}
	}

	// If we aren't force removing, we know we don't have any connections
	// so we don't need to run the queries deleting the connections and
	// remote apps/relations
	if force {
		var synthRelationUUIDs []entityUUID
		err = tx.Query(ctx, getSynthRelationsStmt, offerUUID).GetAll(&synthRelationUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting synthetic relation UUIDs: %w", err)
		}
		relUUIDs := uuids(transform.Slice(synthRelationUUIDs, func(e entityUUID) string { return e.UUID }))

		var synthAppUUIDs []entityUUID
		err = tx.Query(ctx, getSynthAppsStmt, offerUUID).GetAll(&synthAppUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting synthetic application UUIDs: %w", err)
		}

		if err := tx.Query(ctx, deleteRelationEndpointsStmt, relUUIDs).Run(); err != nil {
			return errors.Errorf("deleting relation endpoints: %w", err)
		}

		for _, synthAppUUID := range synthAppUUIDs {
			if err := tx.Query(ctx, deleteRemoteApplicationConsumerStmt, synthAppUUID).Run(); err != nil {
				return errors.Errorf("deleting synthetic application remote consumer: %w", err)
			}

			if err := st.deleteSynthApplication(ctx, tx, synthAppUUID); err != nil {
				return errors.Errorf("deleting synthetic application: %w", err)
			}
		}

		if err := tx.Query(ctx, deleteOfferConnectionStmt, offerUUID).Run(); err != nil {
			return errors.Errorf("deleting offer connections: %w", err)
		}

		if err := tx.Query(ctx, deleteRelationStatusStmt, relUUIDs).Run(); err != nil {
			return errors.Errorf("deleting relation status: %w", err)
		}

		if err := tx.Query(ctx, deleteRelationsStmt, relUUIDs).Run(); err != nil {
			return errors.Errorf("deleting synthetic relations: %w", err)
		}
	}

	if err := tx.Query(ctx, deleteOfferEndpointsStmt, offerUUID).Run(); err != nil {
		return errors.Errorf("deleting offer endpoints: %w", err)
	}

	if err := tx.Query(ctx, deleteOfferStmt, offerUUID).Run(); err != nil {
		return errors.Errorf("deleting offer: %w", err)
	}

	return nil
}
