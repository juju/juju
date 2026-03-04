// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

// GetStorageInstanceUUIDByID retrieves the UUID of a storage instance by
// its ID.
//
// The following errors may be returned:
// - [domainstorageerrors.StorageInstanceNotFound] when no storage
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

// GetStorageInstanceUUIDsByIDs retrieves the UUIDs of storage instances by
// their IDs.
func (s *State) GetStorageInstanceUUIDsByIDs(
	ctx context.Context, storageIDs []string,
) (map[string]string, error) {
	if len(storageIDs) == 0 {
		return map[string]string{}, nil
	}

	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	storageInstanceIDs := storageInstanceIDs(set.NewStrings(storageIDs...).Values())

	stmt, err := s.Prepare(`
SELECT &storageInstanceUUIDAndID.*
FROM   storage_instance
WHERE  storage_id IN ($storageInstanceIDs[:])`,
		storageInstanceUUIDAndID{}, storageInstanceIDs,
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVals []storageInstanceUUIDAndID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, storageInstanceIDs).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string]string, len(dbVals))
	for _, val := range dbVals {
		result[val.ID] = val.UUID
	}

	return result, nil
}
