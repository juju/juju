// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"

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

// GetOfferRelationUUIDs returns the synthetic relation UUIDs for any remote
// consumers connected to the offer.
func (st *State) GetOfferRelationUUIDs(ctx context.Context, oUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	offerUUID := entityUUID{UUID: oUUID}
	var synthRelationUUIDs []entityUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		relationUUIDs, err := st.getOfferRelationUUIDs(ctx, tx, offerUUID)
		if err != nil {
			return errors.Capture(err)
		}
		synthRelationUUIDs = relationUUIDs
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make([]string, len(synthRelationUUIDs))
	for i, relationUUID := range synthRelationUUIDs {
		result[i] = relationUUID.UUID
	}
	return result, nil
}

// HideOffer removes the offer endpoints so the offer can no longer be listed or
// consumed while existing remote relations finish removal.
func (st *State) HideOffer(ctx context.Context, oUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	offerUUID := entityUUID{UUID: oUUID}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.hideOffer(ctx, tx, offerUUID)
	})
	return errors.Capture(err)
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

func (st *State) getOfferRelationUUIDs(ctx context.Context, tx *sqlair.TX, offerUUID entityUUID) ([]entityUUID, error) {
	getSynthRelationsStmt, err := st.Prepare(`
SELECT remote_relation_uuid AS &entityUUID.uuid
FROM   offer_connection
WHERE  offer_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing synthetic relations query: %w", err)
	}

	var synthRelationUUIDs []entityUUID
	err = tx.Query(ctx, getSynthRelationsStmt, offerUUID).GetAll(&synthRelationUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting synthetic relation UUIDs: %w", err)
	}
	return synthRelationUUIDs, nil
}

func (st *State) hideOffer(ctx context.Context, tx *sqlair.TX, offerUUID entityUUID) error {
	deleteOfferEndpointsStmt, err := st.Prepare(`
DELETE FROM offer_endpoint
WHERE offer_uuid = $entityUUID.uuid
`, offerUUID)
	if err != nil {
		return errors.Errorf("preparing delete offer endpoints query: %w", err)
	}

	if err := tx.Query(ctx, deleteOfferEndpointsStmt, offerUUID).Run(); err != nil {
		return errors.Errorf("deleting offer endpoints: %w", err)
	}

	return nil
}

func (st *State) deleteOffer(ctx context.Context, tx *sqlair.TX, offerUUID entityUUID, force bool) error {
	deleteOfferStmt, err := st.Prepare(`
DELETE FROM offer 
WHERE uuid = $entityUUID.uuid
`, offerUUID)
	if err != nil {
		return errors.Errorf("preparing delete offer query: %w", err)
	}

	synthRelationUUIDs, err := st.getOfferRelationUUIDs(ctx, tx, offerUUID)
	if err != nil {
		return errors.Capture(err)
	}

	for _, synthRelationUUID := range synthRelationUUIDs {
		err = st.deleteRelationUnitsForRelation(ctx, tx, synthRelationUUID)
		if err != nil {
			return errors.Errorf("deleting relation units for relation %q: %w", synthRelationUUID, err)
		}

		err = st.deleteRelationWithRemoteConsumer(ctx, tx, synthRelationUUID)
		if err != nil {
			return errors.Errorf("deleting synthetic relations with remote consumers: %w", err)
		}
	}

	if err := st.hideOffer(ctx, tx, offerUUID); err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteOfferStmt, offerUUID).Run(); err != nil {
		return errors.Errorf("deleting offer: %w", err)
	}

	return nil
}
