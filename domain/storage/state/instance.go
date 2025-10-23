// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

// GetStorageInstanceUUIDByID retrieves the UUID of a storage instance by
// its ID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.StorageInstanceNotFound] when no storage
// instance exists for the provided ID.
func (s *State) GetStorageInstanceUUIDByID(
	ctx context.Context, storageID string,
) (domainstorage.StorageInstanceUUID, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		input = storageInstanceID{ID: storageID}
		dbVal entityUUID
	)

	stmt, err := s.Prepare(`
SELECT &entityUUID.*
FROM   storage_instance
WHERE  storage_id = $storageInstanceID.storage_id`,
		input, dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"storage instance with ID %q does not exist", storageID,
			).Add(domainstorageerrors.StorageInstanceNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return domainstorage.StorageInstanceUUID(dbVal.UUID), nil
}
