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

	selectSPUUIDStmt, err := sqlair.Prepare(`
SELECT &StoragePool.uuid
FROM   storage_pool
WHERE  name = $StoragePool.name`, StoragePool{})
	if err != nil {
		return errors.Capture(err)
	}

	selectOriginIDStmt, err := sqlair.Prepare(`
SELECT &poolOrigin.id
FROM   storage_pool_origin
WHERE  origin = $poolOrigin.origin`, poolOrigin{})
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
		err := tx.Query(ctx, selectSPUUIDStmt, dbPool).Get(&dbPool)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		if err == nil {
			return errors.Errorf("storage pool %q %w", pool.Name, storageerrors.PoolAlreadyExists)
		}
		origin := poolOrigin{Origin: string(pool.Origin)}
		if pool.Origin != "" {
			err = tx.Query(ctx, selectOriginIDStmt, origin).Get(&origin)
			if errors.Is(err, sqlair.ErrNoRows) {
				// This should never happen, as the origin has been validated
				// in the domain service layer.
				return errors.Errorf("storage pool %q with invalid origin %q", pool.Name, pool.Origin)
			}
			if err != nil {
				return errors.Errorf("getting storage pool origin %q: %w", pool.Origin, err)
			}
		}

		poolUUID, err := domainstorage.NewStoragePoolUUID()
		if err != nil {
			return errors.Errorf("generating storage pool UUID: %w", err)
		}
		if err := storagePoolUpserter(ctx, tx, poolUUID.String(), pool.Name, pool.Provider, origin); err != nil {
			return errors.Errorf("creating storage pool %q: %w", pool.Name, err)
		}

		if err := poolAttributesUpdater(ctx, tx, poolUUID.String(), pool.Attrs); err != nil {
			return errors.Errorf("creating storage pool %q attributes: %w", pool.Name, err)
		}
		return nil
	})

	return errors.Capture(err)
}

type upsertStoragePoolFunc func(
	ctx context.Context,
	tx *sqlair.TX,
	poolUUID string,
	name string,
	providerType string,
	origin poolOrigin,
) error

func storagePoolUpserter() (upsertStoragePoolFunc, error) {
	insertStmt, err := sqlair.Prepare(`
INSERT INTO storage_pool (uuid, name, type)
VALUES (
    $StoragePool.uuid,
    $StoragePool.name,
    $StoragePool.type
)
ON CONFLICT(uuid)
  DO UPDATE SET
    name=excluded.name,
    type=excluded.type`, StoragePool{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertWithOriginStmt, err := sqlair.Prepare(`
INSERT INTO storage_pool (uuid, name, type, origin_id)
VALUES (
    $StoragePool.uuid,
    $StoragePool.name,
    $StoragePool.type,
    $poolOrigin.id
)
ON CONFLICT(uuid)
  DO UPDATE SET
    name=excluded.name,
    type=excluded.type`, StoragePool{}, poolOrigin{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return func(
		ctx context.Context,
		tx *sqlair.TX,
		poolUUID string,
		name string,
		providerType string,
		origin poolOrigin,
	) error {
		if name == "" {
			return errors.Errorf("storage pool name cannot be empty").Add(storageerrors.MissingPoolNameError)
		}
		if providerType == "" {
			return errors.Errorf("storage pool type cannot be empty").Add(storageerrors.MissingPoolTypeError)
		}

		storagePool := StoragePool{
			ID:           poolUUID,
			Name:         name,
			ProviderType: providerType,
		}
		if origin.Origin != "" {
			err = tx.Query(ctx, insertWithOriginStmt, storagePool, origin).Run()
			return errors.Capture(err)
		}

		err = tx.Query(ctx, insertStmt, storagePool).Run()
		return errors.Capture(err)
	}, nil
}

type updatePoolAttributesFunc func(ctx context.Context, tx *sqlair.TX, poolUUID string, attr domainstorage.Attrs) error

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

	return func(ctx context.Context, tx *sqlair.TX, poolUUID string, attr domainstorage.Attrs) error {
		var keys keysToKeep
		for k := range attr {
			keys = append(keys, k)
		}
		if err := tx.Query(ctx, deleteStmt, sqlair.M{"uuid": poolUUID}, keys).Run(); err != nil {
			return errors.Capture(err)
		}
		for key, value := range attr {
			// TODO: bulk insert.
			if err := tx.Query(ctx, insertStmt, poolAttribute{
				ID:    poolUUID,
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

	pool := StoragePool{Name: name}

	poolAttributeDeleteStmt, err := st.Prepare(`
DELETE FROM storage_pool_attribute
WHERE  storage_pool_attribute.storage_pool_uuid = (
  select uuid FROM storage_pool WHERE name = $StoragePool.name
)`, pool)
	if err != nil {
		return errors.Capture(err)
	}
	poolDeleteStmt, err := st.Prepare(`
DELETE FROM storage_pool
WHERE  storage_pool.uuid = (
  select uuid FROM storage_pool WHERE name = $StoragePool.name
)`, pool)
	if err != nil {
		return errors.Capture(err)
	}

	originStmt, err := st.Prepare(`
SELECT (spo.*) AS (&poolOrigin.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_origin spo ON spo.id = sp.origin_id
WHERE  sp.name = $StoragePool.name`, pool, poolOrigin{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var origin poolOrigin
		err := tx.Query(ctx, originStmt, pool).Get(&origin)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("storage pool %q not found", name).Add(storageerrors.PoolNotFoundError)
		}
		if err != nil {
			return errors.Errorf("getting storage pool origin: %w", err)
		}
		if origin.Origin != string(domainstorage.StoragePoolOriginUser) {
			return errors.Errorf("storage pool %q is not user-created, cannot delete", name)
		}

		if err := tx.Query(ctx, poolAttributeDeleteStmt, pool).Run(); err != nil {
			return errors.Errorf("deleting storage pool attributes: %w", err)
		}
		var outcome = sqlair.Outcome{}
		err = tx.Query(ctx, poolDeleteStmt, pool).Get(&outcome)
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
	if pool.Origin != "" {
		return errors.Errorf("storage pool origin is immutable")
	}

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
		if err := storagePoolUpserter(ctx, tx, poolUUID, pool.Name, pool.Provider, poolOrigin{}); err != nil {
			return errors.Errorf("updating storage pool: %w", err)
		}

		if err := poolAttributesUpdater(ctx, tx, poolUUID, pool.Attrs); err != nil {
			return errors.Errorf("updating storage pool %s attributes: %w", poolUUID, err)
		}
		return nil
	})

	return errors.Capture(err)
}

type loadStoragePoolsFunc func(ctx context.Context, tx *sqlair.TX) ([]domainstorage.StoragePool, error)

func storagePoolsLoader(wantNames domainstorage.Names, wantProviders domainstorage.Providers) (loadStoragePoolsFunc, error) {
	query := `
SELECT (sp.uuid, sp.name, sp.type, spo.origin) AS (&StoragePool.*),
       (sp_attr.key, sp_attr.value) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
       LEFT JOIN storage_pool_origin spo ON spo.id = sp.origin_id
`

	types := []any{
		StoragePool{},
		poolAttribute{},
	}

	var queryArgs []any
	condition, args := buildStoragePoolsFilter(wantNames, wantProviders)
	if len(args) > 0 {
		query = query + "WHERE " + condition
		types = append(types, args...)
		queryArgs = append([]any{}, args...)
	}

	queryStmt, err := sqlair.Prepare(query, types...)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return func(ctx context.Context, tx *sqlair.TX) ([]domainstorage.StoragePool, error) {
		var (
			dbRows    StoragePools
			keyValues []poolAttribute
		)
		err = tx.Query(ctx, queryStmt, queryArgs...).GetAll(&dbRows, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return nil, errors.Errorf("loading storage pool: %w", err)
		}
		return dbRows.toStoragePools(keyValues)
	}, nil
}

// ListStoragePoolsWithoutBuiltins returns the storage pools excluding the built-in storage pools.
func (st State) ListStoragePoolsWithoutBuiltins(ctx context.Context) ([]domainstorage.StoragePool, error) {
	// TODO: implement this to satisfy the domainstorageservice.StoragePoolState interface.
	return nil, nil
}

// ListStoragePools returns the storage pools including default storage pools.
func (st State) ListStoragePools(ctx context.Context) ([]domainstorage.StoragePool, error) {
	// TODO: implement this to satisfy the domainstorageservice.StoragePoolState interface.
	return nil, nil
}

// ListStoragePoolsByNamesAndProviders returns the storage pools matching the specified
// names and or providers, including the default storage pools.
// If no names and providers are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByNamesAndProviders(
	ctx context.Context,
	names domainstorage.Names,
	providers domainstorage.Providers,
) ([]domainstorage.StoragePool, error) {
	if len(names) == 0 && len(providers) == 0 {
		return []domainstorage.StoragePool{}, nil
	}

	// TODO: implement this to satisfy the domainstorageservice.StoragePoolState interface.
	return nil, nil
}

// ListStoragePoolsByNames returns the storage pools matching the specified names, including
// the default storage pools.
// If no names are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByNames(
	ctx context.Context,
	names domainstorage.Names,
) ([]domainstorage.StoragePool, error) {
	if len(names) == 0 {
		return []domainstorage.StoragePool{}, nil
	}
	// TODO: implement this to satisfy the domainstorageservice.StoragePoolState interface.
	return nil, nil
}

// ListStoragePoolsByProviders returns the storage pools matching the specified
// providers, including the default storage pools.
// If no providers are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (st State) ListStoragePoolsByProviders(
	ctx context.Context,
	providers domainstorage.Providers,
) ([]domainstorage.StoragePool, error) {
	if len(providers) == 0 {
		return []domainstorage.StoragePool{}, nil
	}
	// TODO: implement this to satisfy the domainstorageservice.StoragePoolState interface.
	return nil, nil
}

func buildStoragePoolsFilter(wantNames domainstorage.Names, wantProviders domainstorage.Providers) (string, []any) {
	if len(wantNames) == 0 && len(wantProviders) == 0 {
		return "", nil
	}

	if len(wantNames) > 0 && len(wantProviders) > 0 {
		condition := "sp.name IN ($StoragePoolNames[:]) AND sp.type IN ($StorageProviderTypes[:])"
		return condition, []any{StoragePoolNames(wantNames), StorageProviderTypes(wantProviders)}
	}

	if len(wantNames) > 0 {
		condition := "sp.name IN ($StoragePoolNames[:])"
		return condition, []any{StoragePoolNames(wantNames)}
	}

	condition := "sp.type IN ($StorageProviderTypes[:])"
	return condition, []any{StorageProviderTypes(wantProviders)}
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
	storagePoolsLoader, err := storagePoolsLoader(domainstorage.Names{name}, nil)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}

	var storagePools []domainstorage.StoragePool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		storagePools, err = storagePoolsLoader(ctx, tx)
		return errors.Capture(err)
	})
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}
	if len(storagePools) == 0 {
		return domainstorage.StoragePool{}, errors.Errorf("storage pool %q %w", name, storageerrors.PoolNotFoundError)
	}
	if len(storagePools) > 1 {
		return domainstorage.StoragePool{}, errors.Errorf("expected 1 storage pool, got %d", len(storagePools))
	}
	return storagePools[0], errors.Capture(err)
}
