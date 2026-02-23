// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"slices"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/internal/errors"
)

// checkStoragePoolExists checks to see if a storage pool for the given uuid
// exists in the model.
func (st *State) checkStoragePoolExists(
	ctx context.Context, tx *sqlair.TX, uuid string,
) (bool, error) {
	inputUUID := entityUUID{UUID: uuid}

	checkQ := `
SELECT &entityUUID.*
FROM   storage_pool
WHERE  uuid = $entityUUID.uuid
`

	checkStmt, err := st.Prepare(checkQ, inputUUID)
	if err != nil {
		return false, errors.Errorf(
			"preparing check storage pool %q exists statement: %w",
			uuid, err,
		)
	}

	err = tx.Query(ctx, checkStmt, inputUUID).Get(&inputUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Errorf(
			"checking if storage pool %q exists: %w", uuid, err,
		)
	}

	return true, nil
}

// checkStoragePoolForNameOrUUIDExists checks to see if a storage pool for the
// given name or uuid already exists in the model.
func (st *State) checkStoragePoolForNameOrUUIDExists(
	ctx context.Context, tx *sqlair.TX, name, uuid string,
) (bool, error) {
	var (
		inputName = storagePoolName{Name: name}
		inputUUID = entityUUID{UUID: uuid}
	)
	checkQ := `
SELECT &entityUUID.*
FROM storage_pool
WHERE uuid = $entityUUID.uuid
OR name = $storagePoolName.name
`

	checkStmt, err := st.Prepare(checkQ, inputUUID, inputName)
	if err != nil {
		return false, errors.Errorf(
			"preparing check storage pool name or uuid exists statement: %w",
			err,
		)
	}

	err = tx.Query(ctx, checkStmt, inputUUID, inputName).Get(&inputUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		// Getting a [sqlair.ErrNoRows] indicates that no storage pool exists
		// conflicting with the supplied name and uuid.
		return false, nil
	} else if err != nil {
		return false, errors.Errorf(
			"checking if a storage pool for name %q or uuid %q already exists: %w",
			name, uuid, err,
		)
	}

	// We got a result for the sql query which tells us there is a conflict.
	return true, nil
}

// CreateStoragePool creates a new storage pool in the model with the specified
// args and uuid value.
//
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolAlreadyExists] if a pool with the same name or
// uuid already exist in the model.
func (st *State) CreateStoragePool(
	ctx context.Context, args domainstorageinternal.CreateStoragePool,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertStoragePool := insertStoragePool{
		OriginID: int(args.Origin),
		Name:     args.Name,
		Type:     args.ProviderType.String(),
		UUID:     args.UUID.String(),
	}
	insertStoragePoolAttributes := make(
		[]insertStoragePoolAttribute, 0, len(args.Attrs),
	)

	for k, v := range args.Attrs {
		insertStoragePoolAttributes = append(
			insertStoragePoolAttributes, insertStoragePoolAttribute{
				StoragePoolUUID: args.UUID.String(),
				Key:             k,
				Value:           v,
			},
		)
	}

	insertPoolQ := `
INSERT INTO storage_pool (uuid, name, type, origin_id)
VALUES ($insertStoragePool.*)
`

	insertPoolStmt, err := sqlair.Prepare(insertPoolQ, insertStoragePool)
	if err != nil {
		return errors.Errorf(
			"preparing sql statement for inserting new storage pool: %w",
			err,
		)
	}

	insertPoolAttributeQ := `
INSERT INTO storage_pool_attribute (storage_pool_uuid, key, value)
VALUES ($insertStoragePoolAttribute.*)
	`

	insertPoolAttributeStmt, err := sqlair.Prepare(
		insertPoolAttributeQ, insertStoragePoolAttribute{},
	)
	if err != nil {
		return errors.Errorf(
			"preparing sql statement for inserting new storage pool attribute(s): %w",
			err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkStoragePoolForNameOrUUIDExists(
			ctx, tx, args.Name, args.UUID.String(),
		)
		if err != nil {
			return errors.Errorf(
				"checking if storage pool for name %q or uuid %q already exists in model: %w",
				args.Name, args.UUID, err,
			)
		}

		if exists {
			return errors.Errorf(
				"storage pool with name %q already exists", args.Name,
			).Add(domainstorageerrors.StoragePoolAlreadyExists)
		}

		err = tx.Query(ctx, insertPoolStmt, insertStoragePool).Run()
		if err != nil {
			return errors.Errorf(
				"creating new storage pool for name %q in model database: %w",
				args.Name, err,
			)
		}

		// We don't want to try an insert an empty slice of values as this will
		// fail. If there are no attributes for the storage pool get out early.
		if len(insertStoragePoolAttributes) == 0 {
			return nil
		}

		err = tx.Query(
			ctx, insertPoolAttributeStmt, insertStoragePoolAttributes).Run()
		if err != nil {
			return errors.Errorf(
				"create new storage pool for name %q attributes: %w",
				args.Name, err,
			)
		}

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

type upsertStoragePoolFunc func(ctx context.Context, tx *sqlair.TX, poolUUID string, name string, providerType string) error

func storagePoolUpserter() (upsertStoragePoolFunc, error) {
	insertQuery := `
INSERT INTO storage_pool (uuid, name, type)
VALUES (
    $storagePool.uuid,
    $storagePool.name,
    $storagePool.type
)
ON CONFLICT(uuid) DO UPDATE SET name=excluded.name,
                                type=excluded.type
`

	insertStmt, err := sqlair.Prepare(insertQuery, storagePool{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return func(ctx context.Context, tx *sqlair.TX, poolUUID string, name string, providerType string) error {
		dbPool := storagePool{
			UUID: poolUUID,
			Name: name,
			Type: providerType,
		}

		err = tx.Query(ctx, insertStmt, dbPool).Run()
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	}, nil
}

type updatePoolAttributesFunc func(ctx context.Context, tx *sqlair.TX, storagePoolUUID string, attr domainstorage.Attrs) error

type keysToKeep []string

func poolAttributesUpdater() (updatePoolAttributesFunc, error) {
	// Delete any keys no longer in the attributes map.
	deleteQuery := `
DELETE FROM  storage_pool_attribute
WHERE        storage_pool_uuid = $entityUUID.uuid
AND          key NOT IN ($keysToKeep[:])
`

	deleteStmt, err := sqlair.Prepare(deleteQuery, entityUUID{}, keysToKeep{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertQuery := `
INSERT INTO storage_pool_attribute (storage_pool_uuid, key, value)
VALUES ($insertStoragePoolAttribute.*)
ON CONFLICT(storage_pool_uuid, key) DO UPDATE SET key=excluded.key,
                                                      value=excluded.value
`
	insertStmt, err := sqlair.Prepare(insertQuery, insertStoragePoolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return func(ctx context.Context, tx *sqlair.TX, storagePoolUUID string, attr domainstorage.Attrs) error {
		var keys keysToKeep
		for k := range attr {
			keys = append(keys, k)
		}
		if err := tx.Query(ctx, deleteStmt, entityUUID{UUID: storagePoolUUID}, keys).Run(); err != nil {
			return errors.Capture(err)
		}
		for key, value := range attr {
			// TODO: bulk insert.
			if err := tx.Query(ctx, insertStmt, insertStoragePoolAttribute{
				StoragePoolUUID: storagePoolUUID,
				Key:             key,
				Value:           value,
			}).Run(); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	}, nil
}

// DeleteStoragePool deletes a storage pool with the specified name.
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified name does not exist.
func (st State) DeleteStoragePool(ctx context.Context, name string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	poolAttributeDeleteQ := `
DELETE FROM storage_pool_attribute
WHERE  storage_pool_attribute.storage_pool_uuid = (select uuid FROM storage_pool WHERE name = $M.name)
`

	poolDeleteQ := `
DELETE FROM storage_pool
WHERE  storage_pool.uuid = (select uuid FROM storage_pool WHERE name = $M.name)
`

	poolAttributeDeleteStmt, err := st.Prepare(poolAttributeDeleteQ, sqlair.M{})
	if err != nil {
		return errors.Capture(err)
	}
	poolDeleteStmt, err := st.Prepare(poolDeleteQ, sqlair.M{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		nameMap := sqlair.M{"name": name}
		if err := tx.Query(ctx, poolAttributeDeleteStmt, nameMap).Run(); err != nil {
			return errors.Errorf("deleting storage pool attributes: %w", err)
		}
		var outcome = sqlair.Outcome{}
		err = tx.Query(ctx, poolDeleteStmt, nameMap).Get(&outcome)
		if err != nil {
			return errors.Capture(err)
		}
		rowsAffected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Errorf("deleting storage pool: %w", err)
		}
		if rowsAffected == 0 {
			return errors.Errorf("storage pool %q not found", name).Add(domainstorageerrors.StoragePoolNotFound)
		}
		return nil
	})
	return errors.Capture(err)
}

// ReplaceStoragePool replaces an existing storage pool with the specified configuration.
// The storage pool must already exist, and its UUID must be specified.
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified UUID does not exist.
func (st State) ReplaceStoragePool(ctx context.Context, pool domainstorage.StoragePool) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	storagePoolUpserter, err := storagePoolUpserter()
	if err != nil {
		return errors.Capture(err)
	}
	poolAttributesUpdater, err := poolAttributesUpdater()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkStoragePoolExists(ctx, tx, pool.UUID)
		if err != nil {
			return err
		}
		if !exists {
			return errors.Errorf(
				"storage pool %s not found", pool.UUID,
			).Add(domainstorageerrors.StoragePoolNotFound)
		}

		if err := storagePoolUpserter(ctx, tx, pool.UUID, pool.Name, pool.Provider); err != nil {
			return errors.Errorf("updating storage pool: %w", err)
		}

		if err := poolAttributesUpdater(ctx, tx, pool.UUID, pool.Attrs); err != nil {
			return errors.Errorf("updating storage pool %s attributes: %w", pool.UUID, err)
		}
		return nil
	})

	return errors.Capture(err)
}

// ListStoragePools returns all storage pools in the model.
func (st State) ListStoragePools(ctx context.Context) ([]domainstorage.StoragePool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT   (sp.*) AS (&storagePool.*),
         (sp_attr.*) AS (&storagePoolAttribute.*)
FROM     storage_pool sp
         LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
ORDER BY sp.uuid`,
		storagePool{}, storagePoolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    storagePools
		keyValues []storagePoolAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt).GetAll(&dbRows, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("listing storage pools: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return dbRows.toStoragePools(keyValues)
}

// ListStoragePoolsByNamesAndProviders returns the storage pools matching the specified
// names and providers, including the default storage pools.
// If no names and providers are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByNamesAndProviders(
	ctx context.Context,
	names, providers []string,
) ([]domainstorage.StoragePool, error) {
	if len(names) == 0 || len(providers) == 0 {
		return nil, nil
	}
	spNames := storagePoolNames(names)
	spTypes := storageProviderTypes(providers)

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	stmt, err := st.Prepare(`
SELECT   (sp.*) AS (&storagePool.*),
         (sp_attr.*) AS (&storagePoolAttribute.*)
FROM     storage_pool sp
         LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
-- order matters because the index is on (type, name)
WHERE    sp.type IN ($storageProviderTypes[:]) AND sp.name IN ($storagePoolNames[:])
ORDER BY sp.uuid`,
		spNames, spTypes, storagePool{}, storagePoolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    storagePools
		keyValues []storagePoolAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, spNames, spTypes).GetAll(&dbRows, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("listing storage pools: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return dbRows.toStoragePools(keyValues)
}

// ListStoragePoolsByNames returns the storage pools matching the specified names, including
// the default storage pools.
// If no names are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByNames(
	ctx context.Context,
	names []string,
) ([]domainstorage.StoragePool, error) {
	if len(names) == 0 {
		return nil, nil
	}
	spNames := storagePoolNames(names)

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT   (sp.*) AS (&storagePool.*),
         (sp_attr.*) AS (&storagePoolAttribute.*)
FROM     storage_pool sp
         LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
WHERE    sp.name IN ($storagePoolNames[:])
ORDER BY sp.uuid`, spNames, storagePool{}, storagePoolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    storagePools
		keyValues []storagePoolAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, spNames).GetAll(&dbRows, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("listing storage pools: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return dbRows.toStoragePools(keyValues)
}

// ListStoragePoolsByProviders returns the storage pools matching the specified
// providers, including the default storage pools.
// If no providers are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByProviders(
	ctx context.Context,
	providers []string,
) ([]domainstorage.StoragePool, error) {
	if len(providers) == 0 {
		return nil, nil
	}
	spTypes := storageProviderTypes(providers)

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT   (sp.*) AS (&storagePool.*),
         (sp_attr.*) AS (&storagePoolAttribute.*)
FROM     storage_pool sp
         LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
WHERE    sp.type IN ($storageProviderTypes[:])
ORDER BY sp.uuid`, spTypes, storagePool{}, storagePoolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    storagePools
		keyValues []storagePoolAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, spTypes).GetAll(&dbRows, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("listing storage pools: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return dbRows.toStoragePools(keyValues)
}

// GetStoragePoolUUID returns the UUID of the storage pool for the specified name.
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified name does not exist.
func (st State) GetStoragePoolUUID(
	ctx context.Context,
	name string,
) (domainstorage.StoragePoolUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var poolUUID domainstorage.StoragePoolUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		poolUUID, err = GetStoragePoolUUID(ctx, tx, st, name)
		return err
	})
	if err != nil {
		return "", errors.Errorf("getting storage pool %q UUID: %w", name, err)
	}
	return poolUUID, nil
}

// GetStoragePool returns the storage pool for the specified UUID.
//
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified UUID
// does not exist.
func (st State) GetStoragePool(
	ctx context.Context,
	poolUUID domainstorage.StoragePoolUUID,
) (domainstorage.StoragePool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}

	var (
		inputUUID               = entityUUID{UUID: poolUUID.String()}
		dbStoragePool           storagePool
		dbStoragePoolAttributes []storagePoolAttribute
	)

	storagePoolQ := `
SELECT &storagePool.* FROM storage_pool WHERE uuid = $entityUUID.uuid
`
	storagePoolStmt, err := st.Prepare(storagePoolQ, inputUUID, dbStoragePool)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Errorf(
			"preparing get storage pool %q statement: %w", poolUUID, err,
		)
	}

	storagePoolAttributesQ := `
SELECT &storagePoolAttribute.*
FROM storage_pool_attribute
WHERE storage_pool_uuid = $entityUUID.uuid
`
	storagePoolAttributesStmt, err := st.Prepare(
		storagePoolAttributesQ, inputUUID, storagePoolAttribute{},
	)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Errorf(
			"preparing get storage pool attributes %q statement: %w",
			poolUUID, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, storagePoolStmt, inputUUID).Get(&dbStoragePool)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"storage pool %q does not exist", poolUUID,
			).Add(domainstorageerrors.StoragePoolNotFound)
		} else if err != nil {
			return errors.Errorf(
				"getting storage pool %q: %w", poolUUID, err,
			)
		}

		err = tx.Query(ctx, storagePoolAttributesStmt, inputUUID).GetAll(
			&dbStoragePoolAttributes,
		)
		if errors.Is(err, sqlair.ErrNoRows) {
			// A no rows error tells us the storage pool does not have any
			// attributes set which is perfectly valid.
			return nil
		} else if err != nil {
			return errors.Errorf(
				"getting storage pool %q attributes: %w", poolUUID, err,
			)
		}

		return nil
	})
	if err != nil {
		return domainstorage.StoragePool{}, err
	}

	retVal := domainstorage.StoragePool{
		Attrs:    make(domainstorage.Attrs, len(dbStoragePoolAttributes)),
		Provider: dbStoragePool.Type,
		Name:     dbStoragePool.Name,
		UUID:     dbStoragePool.UUID,
	}
	for _, attr := range dbStoragePoolAttributes {
		retVal.Attrs[attr.Key] = attr.Value
	}

	return retVal, nil
}

// GetStoragePoolProvidersByNames returns a map of storage pool names to their
// provider types for the specified storage pool names.
//
// The following errors may be returned:
// - [domainstorageerrors.StoragePoolNotFound] when any of the specified
// storage pools do not exist.
func (st State) GetStoragePoolProvidersByNames(ctx context.Context, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	storagePoolNames := storagePoolNames(set.NewStrings(names...).Values())
	query := `
SELECT &storagePoolNameAndType.*
FROM storage_pool
WHERE name IN ($storagePoolNames[:])`
	stmt, err := st.Prepare(query, storagePoolNameAndType{}, storagePoolNames)
	if err != nil {
		return nil, errors.Capture(err)
	}

	res := []storagePoolNameAndType{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, storagePoolNames).GetAll(&res)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting storage pool providers by names: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(res) != len(storagePoolNames) {
		// This indicates some of the pool names provided did not hit any results.
		missingName := set.NewStrings(names...).
			Difference(set.NewStrings(transform.Slice(res, func(r storagePoolNameAndType) string { return r.Name })...)).
			Values()
		return nil, errors.Errorf("storage pool(s) with name(s) %s not found", strings.Join(missingName, ", ")).
			Add(domainstorageerrors.StoragePoolNotFound)
	}

	providers := make(map[string]string, len(res))
	for _, r := range res {
		providers[r.Name] = r.Type
	}
	return providers, nil
}

// SetModelStoragePools replaces the model's recommended storage pools with the
// supplied set. All existing model storage pool mappings are removed before the
// new ones are inserted.
//
// If any referenced storage pool UUID does not exist in the model, this
// returns [domainstorageerrors.StoragePoolNotFound]. Supplying an empty slice
// results in a no-op.
func (st State) SetModelStoragePools(ctx context.Context, args []domainstorageinternal.RecommendedStoragePoolArg) error {
	if len(args) == 0 {
		return nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	deleteQuery := "DELETE FROM model_storage_pool"
	deleteStmt, err := st.Prepare(deleteQuery)
	if err != nil {
		return errors.Capture(err)
	}

	insertQuery := `
INSERT INTO model_storage_pool (*) VALUES ($dbModelStoragePool.*)
`
	insertStmt, err := st.Prepare(insertQuery, dbModelStoragePool{})
	if err != nil {
		return errors.Capture(err)
	}

	insertVals := make([]dbModelStoragePool, 0, len(args))
	poolUUIDs := make([]string, 0, len(args))
	for _, a := range args {
		insertVals = append(insertVals, dbModelStoragePool{
			StoragePoolUUID: a.StoragePoolUUID.String(),
			StorageKindID:   int(a.StorageKind),
		})
		poolUUIDs = append(poolUUIDs, a.StoragePoolUUID.String())
	}
	// We must deduplicate poolUUIDs
	slices.Sort(poolUUIDs)
	poolUUIDs = slices.Compact(poolUUIDs)

	err = db.Txn(ctx, func(c context.Context, tx *sqlair.TX) error {
		poolsExist, err := st.checkStoragePoolsExist(ctx, tx, poolUUIDs)
		if err != nil {
			return errors.Errorf(
				"checking storage pool(s) exist in the model: %w", err,
			)
		}
		if !poolsExist {
			return errors.New(
				"one or more storage pools do not exist in the model",
			).Add(domainstorageerrors.StoragePoolNotFound)
		}

		err = tx.Query(ctx, deleteStmt).Run()
		if err != nil {
			return errors.Errorf("deleting existing model storage pools: %w", err)
		}

		err = tx.Query(ctx, insertStmt, insertVals).Run()
		return err
	})

	return errors.Capture(err)
}

// checkStoragePoolsExist checks whether the supplied UUIDs exist in the DB.
// It is expected that the caller of this func MUST de-duplicate the UUIDs.
func (st State) checkStoragePoolsExist(
	ctx context.Context,
	tx *sqlair.TX,
	storagePoolUUIDs []string,
) (bool, error) {
	if len(storagePoolUUIDs) == 0 {
		return true, nil
	}

	type poolUUIDs []string
	var (
		dbVal dbAggregateCount
		input = poolUUIDs(storagePoolUUIDs)
	)

	query := `
SELECT COUNT(*) AS &dbAggregateCount.count
FROM   storage_pool
WHERE  uuid IN ($poolUUIDs[:])
`
	stmt, err := st.Prepare(query, dbVal, input)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, input).Get(&dbVal)
	if err != nil {
		return false, errors.Capture(err)
	}

	return len(storagePoolUUIDs) == dbVal.Count, nil
}
