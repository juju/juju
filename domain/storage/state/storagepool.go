// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type poolAttributes map[string]string

// CreateStoragePool creates a storage pool with the specified configuration.
// The following errors can be expected:
// - [storageerrors.PoolAlreadyExists] if a pool with the same name already exists.
func (st State) CreateStoragePool(ctx context.Context, pool domainstorage.StoragePool) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return CreateStoragePools(ctx, db, []domainstorage.StoragePool{pool})
}

// CreateStoragePools creates the specified storage pools.
// It is exported for us in the storage/bootstrap package.
func CreateStoragePools(ctx context.Context, db domain.TxnRunner, pools []domainstorage.StoragePool) error {
	selectUUIDStmt, err := sqlair.Prepare("SELECT &StoragePool.uuid FROM storage_pool WHERE name = $StoragePool.name", StoragePool{})
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

	poolsUUIDs := make([]string, len(pools))
	for i := range pools {
		poolsUUIDs[i] = uuid.MustNewUUID().String()
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for i, pool := range pools {
			dbPool := StoragePool{Name: pool.Name}
			err := tx.Query(ctx, selectUUIDStmt, dbPool).Get(&dbPool)
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Capture(err)
			}
			if err == nil {
				return errors.Errorf("storage pool %q %w", pool.Name, storageerrors.PoolAlreadyExists)
			}
			poolUUID := poolsUUIDs[i]

			if err := storagePoolUpserter(ctx, tx, poolUUID, pool.Name, pool.Provider); err != nil {
				return errors.Errorf("creating storage pool %q: %w", pool.Name, err)
			}

			if err := poolAttributesUpdater(ctx, tx, poolUUID, pool.Attrs); err != nil {
				return errors.Errorf("creating storage pool %q attributes: %w", pool.Name, err)
			}
		}
		return nil
	})

	return errors.Capture(err)
}

type upsertStoragePoolFunc func(ctx context.Context, tx *sqlair.TX, poolUUID string, name string, providerType string) error

func storagePoolUpserter() (upsertStoragePoolFunc, error) {
	insertQuery := `
INSERT INTO storage_pool (uuid, name, type)
VALUES (
    $StoragePool.uuid,
    $StoragePool.name,
    $StoragePool.type
)
ON CONFLICT(uuid) DO UPDATE SET name=excluded.name,
                                type=excluded.type
`

	insertStmt, err := sqlair.Prepare(insertQuery, StoragePool{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return func(ctx context.Context, tx *sqlair.TX, poolUUID string, name string, providerType string) error {
		if name == "" {
			return errors.Errorf("storage pool name cannot be empty").Add(storageerrors.MissingPoolNameError)
		}
		if providerType == "" {
			return errors.Errorf("storage pool type cannot be empty").Add(storageerrors.MissingPoolTypeError)
		}

		dbPool := StoragePool{
			ID:           poolUUID,
			Name:         name,
			ProviderType: providerType,
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
	deleteQuery := fmt.Sprintf(`
DELETE FROM  storage_pool_attribute
WHERE        storage_pool_uuid = $M.uuid
AND          key NOT IN ($keysToKeep[:])
`)

	deleteStmt, err := sqlair.Prepare(deleteQuery, sqlair.M{}, keysToKeep{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertQuery := `
INSERT INTO storage_pool_attribute
VALUES (
    $poolAttribute.storage_pool_uuid,
    $poolAttribute.key,
    $poolAttribute.value
)
ON CONFLICT(storage_pool_uuid, key) DO UPDATE SET key=excluded.key,
                                                      value=excluded.value
`
	insertStmt, err := sqlair.Prepare(insertQuery, poolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return func(ctx context.Context, tx *sqlair.TX, storagePoolUUID string, attr domainstorage.Attrs) error {
		var keys keysToKeep
		for k := range attr {
			keys = append(keys, k)
		}
		if err := tx.Query(ctx, deleteStmt, sqlair.M{"uuid": storagePoolUUID}, keys).Run(); err != nil {
			return errors.Capture(err)
		}
		for key, value := range attr {
			// TODO: bulk insert.
			if err := tx.Query(ctx, insertStmt, poolAttribute{
				ID:    storagePoolUUID,
				Key:   key,
				Value: value,
			}).Run(); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	}, nil
}

// DeleteStoragePool deletes a storage pool with the specified name.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
func (st State) DeleteStoragePool(ctx context.Context, name string) error {
	db, err := st.DB()
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
			return errors.Errorf("storage pool %q not found", name).Add(storageerrors.PoolNotFoundError)
		}
		return nil
	})
	return errors.Capture(err)
}

// ReplaceStoragePool replaces an existing storage pool with the specified configuration.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
func (st State) ReplaceStoragePool(ctx context.Context, pool domainstorage.StoragePool) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	selectUUIDStmt, err := st.Prepare("SELECT &StoragePool.uuid FROM storage_pool WHERE name = $StoragePool.name", StoragePool{})
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
		dbPool := StoragePool{Name: pool.Name}
		err := tx.Query(ctx, selectUUIDStmt, dbPool).Get(&dbPool)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		if err != nil {
			return errors.Errorf("storage pool %q not found", pool.Name).Add(storageerrors.PoolNotFoundError)
		}
		poolUUID := dbPool.ID
		if err := storagePoolUpserter(ctx, tx, poolUUID, pool.Name, pool.Provider); err != nil {
			return errors.Errorf("updating storage pool: %w", err)
		}

		if err := poolAttributesUpdater(ctx, tx, poolUUID, pool.Attrs); err != nil {
			return errors.Errorf("updating storage pool %s attributes: %w", poolUUID, err)
		}
		return nil
	})

	return errors.Capture(err)
}

// ListStoragePoolsWithoutBuiltins returns the storage pools excluding the built-in storage pools.
func (st State) ListStoragePoolsWithoutBuiltins(ctx context.Context) ([]domainstorage.StoragePool, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := sqlair.Prepare(`
SELECT (sp.uuid, sp.name, sp.type) AS (&StoragePool.*),
       (sp_attr.key, sp_attr.value) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid`,
		StoragePool{}, poolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    StoragePools
		keyValues []poolAttribute
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

// ListStoragePools returns the storage pools including default storage pools.
func (st State) ListStoragePools(ctx context.Context) ([]domainstorage.StoragePool, error) {
	pools, err := st.ListStoragePoolsWithoutBuiltins(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	builtInPools, err := domainstorage.BuiltInStoragePools()
	if err != nil {
		return nil, errors.Errorf("getting built-in storage pools: %w", err)
	}
	return append(pools, builtInPools...), nil
}

// ListStoragePoolsByNamesAndProviders returns the storage pools matching the specified
// names and or providers, including the default storage pools.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByNamesAndProviders(
	ctx context.Context,
	names domainstorage.Names,
	providers domainstorage.Providers,
) ([]domainstorage.StoragePool, error) {
	spNames := storagePoolNames(names.Values())
	spTypes := storageProviderTypes(providers.Values())

	if len(spNames) == 0 || len(spTypes) == 0 {
		return nil, errors.Errorf(
			"at least one name and one provider must be specified, got names: %v, providers: %v",
			names, providers,
		)
	}

	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	stmt, err := sqlair.Prepare(`
SELECT (sp.uuid, sp.name, sp.type) AS (&StoragePool.*),
       (sp_attr.key, sp_attr.value) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
-- order matters because the index is on (type, name)
WHERE  sp.type IN ($storageProviderTypes[:])
       AND sp.name IN ($storagePoolNames[:])`,
		spNames, spTypes, StoragePool{}, poolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    StoragePools
		keyValues []poolAttribute
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

	pools, err := dbRows.toStoragePools(keyValues)
	if err != nil {
		return nil, errors.Capture(err)
	}
	builtIns, err := domainstorage.BuiltInStoragePools()
	if err != nil {
		return nil, errors.Errorf("getting built-in storage pools: %w", err)
	}
	for _, builtIn := range builtIns {
		// Only append built-in pools that match the specified names and providers.
		if names.Contains(builtIn.Name) && providers.Contains(builtIn.Provider) {
			pools = append(pools, builtIn)
		}
	}
	return pools, nil
}

// ListStoragePoolsByNames returns the storage pools matching the specified names, including
// the default storage pools.
// If no names are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByNames(
	ctx context.Context,
	names domainstorage.Names,
) ([]domainstorage.StoragePool, error) {
	spNames := storagePoolNames(names.Values())

	if len(spNames) == 0 {
		return []domainstorage.StoragePool{}, nil
	}
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := sqlair.Prepare(`
SELECT (sp.uuid, sp.name, sp.type) AS (&StoragePool.*),
       (sp_attr.key, sp_attr.value) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
WHERE  sp.name IN ($storagePoolNames[:])`, spNames, StoragePool{}, poolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    StoragePools
		keyValues []poolAttribute
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
	pools, err := dbRows.toStoragePools(keyValues)
	if err != nil {
		return nil, errors.Capture(err)
	}
	builtIns, err := domainstorage.BuiltInStoragePools()
	if err != nil {
		return nil, errors.Errorf("getting built-in storage pools: %w", err)
	}
	for _, builtIn := range builtIns {
		// Only append built-in pools that match the specified names.
		if names.Contains(builtIn.Name) {
			pools = append(pools, builtIn)
		}
	}
	return pools, nil
}

// ListStoragePoolsByProviders returns the storage pools matching the specified
// providers, including the default storage pools.
// If no providers are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByProviders(
	ctx context.Context,
	providers domainstorage.Providers,
) ([]domainstorage.StoragePool, error) {
	spTypes := storageProviderTypes(providers)

	if len(spTypes) == 0 {
		return []domainstorage.StoragePool{}, nil
	}
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := sqlair.Prepare(`
SELECT (sp.uuid, sp.name, sp.type) AS (&StoragePool.*),
       (sp_attr.key, sp_attr.value) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
WHERE  sp.type IN ($storageProviderTypes[:])`, spTypes, StoragePool{}, poolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    StoragePools
		keyValues []poolAttribute
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
	pools, err := dbRows.toStoragePools(keyValues)
	if err != nil {
		return nil, errors.Capture(err)
	}
	builtIns, err := domainstorage.BuiltInStoragePools()
	if err != nil {
		return nil, errors.Errorf("getting built-in storage pools: %w", err)
	}
	for _, builtIn := range builtIns {
		// Only append built-in pools that match the specified providers.
		if providers.Contains(builtIn.Provider) {
			pools = append(pools, builtIn)
		}
	}
	return pools, nil
}

// GetStoragePoolByName returns the storage pool with the specified name.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
func (st State) GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error) {
	db, err := st.DB()
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}
	return GetStoragePoolByName(ctx, db, name)
}

// GetStoragePoolByName returns the storage pool with the specified name.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
// Exported for use by other domains that need to load storage pools.
func GetStoragePoolByName(ctx context.Context, db domain.TxnRunner, name string) (domainstorage.StoragePool, error) {
	builtIns, err := domainstorage.BuiltInStoragePools()
	if err != nil {
		return domainstorage.StoragePool{}, errors.Errorf("getting built-in storage pools: %w", err)
	}
	for _, pool := range builtIns {
		if pool.Name == name {
			return pool, nil
		}
	}

	inputArg := StoragePool{Name: name}
	stmt, err := sqlair.Prepare(`
SELECT (sp.uuid, sp.name, sp.type) AS (&StoragePool.*),
       (sp_attr.key, sp_attr.value) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
WHERE  sp.name = $StoragePool.name`, inputArg, poolAttribute{})
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}

	var (
		dbRows    StoragePools
		keyValues []poolAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, inputArg).GetAll(&dbRows, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting storage pools: %w", err)
		}
		return nil
	})
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}
	storagePools, err := dbRows.toStoragePools(keyValues)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}

	if len(storagePools) == 0 {
		return domainstorage.StoragePool{}, errors.Errorf("storage pool %q", name).Add(storageerrors.PoolNotFoundError)
	}
	if len(storagePools) > 1 {
		return domainstorage.StoragePool{}, errors.Errorf("expected 1 storage pool, got %d", len(storagePools))
	}
	return storagePools[0], errors.Capture(err)
}
