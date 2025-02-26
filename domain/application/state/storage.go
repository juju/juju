// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

func (st *State) loadStoragePoolUUIDByName(ctx context.Context, tx *sqlair.TX, poolNames []string) (map[string]string, error) {
	type poolnames []string
	storageQuery, err := st.Prepare(`
SELECT &storagePool.*
FROM   storage_pool
WHERE  name IN ($poolnames[:])
`, storagePool{}, poolnames{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var dbPools []storagePool
	err = tx.Query(ctx, storageQuery, poolnames(poolNames)).GetAll(&dbPools)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Errorf("querying storage pools: %w", err)
	}
	poolsByName := make(map[string]string)
	for _, p := range dbPools {
		poolsByName[p.Name] = p.UUID
	}
	return poolsByName, nil
}

// insertStorage constructs inserts storage directive records for the application.
func (st *State) insertStorage(ctx context.Context, tx *sqlair.TX, appDetails applicationDetails, appStorage []application.AddApplicationStorageArg) error {
	if len(appStorage) == 0 {
		return nil
	}

	// This check is here until we rework all of the AddApplication logic to
	// run in a single transaction. There's a TO-DO in the AddApplication service method.
	queryStmt, err := st.Prepare(`
SELECT &charmStorage.name FROM charm_storage
WHERE  charm_uuid = $applicationDetails.charm_uuid
`, appDetails, charmStorage{})
	if err != nil {
		return errors.Capture(err)
	}

	var storageMetadata []charmStorage
	err = tx.Query(ctx, queryStmt, appDetails).GetAll(&storageMetadata)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.Errorf("querying supported charm storage: %w", err)
	}
	supportedStorage := set.NewStrings()
	for _, stor := range storageMetadata {
		supportedStorage.Add(stor.Name)
	}
	wantStorage := set.NewStrings()
	for _, stor := range appStorage {
		wantStorage.Add(stor.Name)
	}
	unsupportedStorage := wantStorage.Difference(supportedStorage)
	if unsupportedStorage.Size() > 0 {
		return errors.Errorf("storage %q is not supported", unsupportedStorage.SortedValues())
	}

	// Storage is either a storage type or a pool name.
	// Get a mapping of pool name to pool UUID for any
	// pools specified in the app storage directives.
	poolNames := make([]string, len(appStorage))
	for i, stor := range appStorage {
		poolNames[i] = stor.PoolNameOrType
	}
	poolsByName, err := st.loadStoragePoolUUIDByName(ctx, tx, poolNames)
	if err != nil {
		return errors.Errorf("loading storage pool UUIDs: %w", err)
	}

	storage := make([]storageToAdd, len(appStorage))
	for i, stor := range appStorage {
		storage[i] = storageToAdd{
			ApplicationUUID: appDetails.UUID.String(),
			CharmUUID:       appDetails.CharmID,
			StorageName:     stor.Name,
			Size:            uint(stor.Size),
			Count:           uint(stor.Count),
		}
		// PoolNameOrType has already been validated to either be
		// a pool name or a valid storage type for the relevant cloud.
		if uuid, ok := poolsByName[stor.PoolNameOrType]; ok {
			storage[i].StoragePoolUUID = &uuid
		} else {
			storage[i].StorageType = &stor.PoolNameOrType
		}
	}

	insertStmt, err := st.Prepare(`
INSERT INTO application_storage_directive (*)
VALUES ($storageToAdd.*)`, storageToAdd{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, storage).Run()
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// GetStorageUUIDByID returns the UUID for the specified storage, returning an error
// satisfying [storageerrors.StorageNotFound] if the storage doesn't exist.
func (st *State) GetStorageUUIDByID(ctx context.Context, storageID corestorage.ID) (corestorage.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}
	inst := storageInstance{StorageID: storageID}

	query, err := st.Prepare(`
SELECT &storageInstance.*
FROM   storage_instance
WHERE  storage_id = $storageInstance.storage_id
`, inst)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, query, inst).Get(&inst)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("storage %q not found", storageID).Add(storageerrors.StorageNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying storage ID %q: %w", storageID, err)
	}

	return inst.StorageUUID, nil
}

func (st *State) AttachStorage(ctx context.Context, storageUUID corestorage.UUID, unitUUID coreunit.UUID) error {
	//TODO implement me
	return errors.New("not implemented")
}

func (st *State) AddStorageForUnit(ctx context.Context, storageName corestorage.Name, unitUUID coreunit.UUID, stor storage.Directive) ([]corestorage.ID, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (st *State) DetachStorageForUnit(ctx context.Context, storageUUID corestorage.UUID, unitUUID coreunit.UUID) error {
	//TODO implement me
	return errors.New("not implemented")
}

func (st *State) DetachStorage(ctx context.Context, storageUUID corestorage.UUID) error {
	//TODO implement me
	return errors.New("not implemented")
}
