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
	selectUUIDStmt, err := sqlair.Prepare("SELECT &storagePool.uuid FROM storage_pool WHERE name = $storagePool.name", storagePool{})
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
			dbPool := storagePool{Name: pool.Name}
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
		if name == "" {
			return errors.Errorf("storage pool name cannot be empty").Add(storageerrors.MissingPoolNameError)
		}
		if providerType == "" {
			return errors.Errorf("storage pool type cannot be empty").Add(storageerrors.MissingPoolTypeError)
		}

		dbPool := storagePool{
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

	dbPool := storagePool{Name: pool.Name}
	selectUUIDStmt, err := st.Prepare("SELECT &storagePool.uuid FROM storage_pool WHERE name = $storagePool.name", dbPool)
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
	// TODO: we need to exclude the built-in storage pools.
	return nil, nil
}

// ListStoragePools returns the storage pools including default storage pools.
func (st State) ListStoragePools(ctx context.Context) ([]domainstorage.StoragePool, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := sqlair.Prepare(`
SELECT (sp.*) AS (&storagePool.*),
       (sp_attr.*) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid`,
		storagePool{}, poolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    storagePools
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

// ListStoragePoolsByNamesAndProviders returns the storage pools matching the specified
// names and or providers, including the default storage pools.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByNamesAndProviders(
	ctx context.Context,
	names domainstorage.Names,
	providers domainstorage.Providers,
) ([]domainstorage.StoragePool, error) {
	// TODO: we need to include the built-in storage pools.

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
SELECT (sp.*) AS (&storagePool.*),
       (sp_attr.*) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
-- order matters because the index is on (type, name)
WHERE  sp.type IN ($storageProviderTypes[:])
       AND sp.name IN ($storagePoolNames[:])`,
		spNames, spTypes, storagePool{}, poolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    storagePools
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
	return dbRows.toStoragePools(keyValues)
}

// ListStoragePoolsByNames returns the storage pools matching the specified names, including
// the default storage pools.
// If no names are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByNames(
	ctx context.Context,
	names domainstorage.Names,
) ([]domainstorage.StoragePool, error) {
	// TODO: we need to include the built-in storage pools.
	spNames := storagePoolNames(names.Values())

	if len(spNames) == 0 {
		return []domainstorage.StoragePool{}, nil
	}
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := sqlair.Prepare(`
SELECT (sp.*) AS (&storagePool.*),
       (sp_attr.*) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
WHERE  sp.name IN ($storagePoolNames[:])`, spNames, storagePool{}, poolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    storagePools
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
	return dbRows.toStoragePools(keyValues)
}

// ListStoragePoolsByProviders returns the storage pools matching the specified
// providers, including the default storage pools.
// If no providers are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByProviders(
	ctx context.Context,
	providers domainstorage.Providers,
) ([]domainstorage.StoragePool, error) {
	// TODO: we need to include the built-in storage pools.
	spTypes := storageProviderTypes(providers)

	if len(spTypes) == 0 {
		return []domainstorage.StoragePool{}, nil
	}
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := sqlair.Prepare(`
SELECT (sp.*) AS (&storagePool.*),
       (sp_attr.*) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
WHERE  sp.type IN ($storageProviderTypes[:])`, spTypes, storagePool{}, poolAttribute{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows    storagePools
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
	return dbRows.toStoragePools(keyValues)
}

// GetStoragePoolUUID returns the UUID of the storage pool with the specified name.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
func (st State) GetStoragePoolUUID(ctx context.Context, name string) (domainstorage.StoragePoolUUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}
	return getStoragePoolUUID(ctx, db, name)
}

func getStoragePoolUUID(
	ctx context.Context,
	db domain.TxnRunner,
	name string,
) (domainstorage.StoragePoolUUID, error) {
	// TODO: !!!
	return "", nil
}

// GetStoragePool returns the storage pool with the specified name.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
func (st State) GetStoragePool(ctx context.Context, uuid domainstorage.StoragePoolUUID) (domainstorage.StoragePool, error) {
	db, err := st.DB()
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}
	return getStoragePool(ctx, db, uuid)
}

func getStoragePool(
	ctx context.Context,
	db domain.TxnRunner,
	uuid domainstorage.StoragePoolUUID,
) (domainstorage.StoragePool, error) {
	inputArg := storagePool{ID: uuid.String()}
	stmt, err := sqlair.Prepare(`
SELECT (sp.*) AS (&storagePool.*),
       (sp_attr.*) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
WHERE  sp.uuid = $storagePool.uuid`, inputArg, poolAttribute{})
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}

	var (
		dbRows    storagePools
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
		return domainstorage.StoragePool{}, errors.Errorf("storage pool %q", uuid).Add(storageerrors.PoolNotFoundError)
	}
	if len(storagePools) > 1 {
		return domainstorage.StoragePool{}, errors.Errorf("expected 1 storage pool, got %d", len(storagePools))
	}
	return storagePools[0], errors.Capture(err)
}

// GetStoragePoolByName returns the storage pool with the specified name.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
// Exported for use by other domains that need to load storage pools.
func GetStoragePoolByName(ctx context.Context, db domain.TxnRunner, name string) (domainstorage.StoragePool, error) {
	spUUID, err := getStoragePoolUUID(ctx, db, name)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}
	return getStoragePool(ctx, db, spUUID)
}
