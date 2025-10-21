// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"gopkg.in/macaroon.v2"

	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/internal/errors"
)

// SaveMacaroonForRelation saves the given macaroon for the specified relation.
func (st *State) SaveMacaroonForRelation(ctx context.Context, relationUUID string, mac []byte) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	type relationMacaroon struct {
		RelationUUID string `db:"relation_uuid"`
		Macaroon     []byte `db:"macaroon"`
	}

	arg := relationMacaroon{
		RelationUUID: relationUUID,
		Macaroon:     mac,
	}

	stmt, err := st.Prepare(`
INSERT INTO application_remote_offerer_relation_macaroon (relation_uuid, macaroon)
VALUES ($relationMacaroon.relation_uuid, $relationMacaroon.macaroon)
ON CONFLICT (relation_uuid) DO UPDATE SET macaroon = EXCLUDED.macaroon
`, arg)
	if err != nil {
		return errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, arg).Run()
		return errors.Capture(err)
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetMacaroonForRelation gets the macaroon for the specified remote relation,
// returning an error satisfying [crossmodelrelationerrors.MacaroonNotFound]
// if the macaroon is not found.
func (st *State) GetMacaroonForRelation(ctx context.Context, relationUUID string) (*macaroon.Macaroon, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type relationMacaroon struct {
		Macaroon []byte `db:"macaroon"`
	}

	relUUID := remoteRelationUUID{
		UUID: relationUUID,
	}

	stmt, err := st.Prepare(`
SELECT &relationMacaroon.macaroon
FROM   application_remote_offerer_relation_macaroon
WHERE  relation_uuid = $remoteRelationUUID.uuid
`, relUUID, relationMacaroon{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result relationMacaroon
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, relUUID).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("macaroon for relation %q not found", relationUUID).
				Add(crossmodelrelationerrors.MacaroonNotFound)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return decodeMacaroon(result.Macaroon)
}
