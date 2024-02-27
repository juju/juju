// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/uuid"
)

// State represents database interactions dealing with block devices.
type State struct {
	*domain.StateBase
}

// NewState returns a new block device state
// based on the input database factory method.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

type poolAttributes map[string]string

// CreateStoragePool creates a storage pool, returning an error satisfying [errors.AlreadyExists]
// if a pool with the same name already exists.
func (st State) CreateStoragePool(ctx context.Context, pool domainstorage.StoragePoolDetails) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	selectUUIDStmt, err := sqlair.Prepare("SELECT &StoragePool.uuid FROM storage_pool WHERE name = $StoragePool.name", StoragePool{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		dbPool := StoragePool{Name: pool.Name}
		err := tx.Query(ctx, selectUUIDStmt, dbPool).Get(&dbPool)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(domain.CoerceError(err))
		}
		if err == nil {
			return fmt.Errorf("storage pool %q %w", pool.Name, errors.AlreadyExists)
		}
		poolUUID := uuid.MustNewUUID().String()

		if err := upsertStoragePool(ctx, tx, poolUUID, pool.Name, pool.Provider); err != nil {
			return errors.Annotate(domain.CoerceError(err), "creating storage pool")
		}

		if err := updatePoolAttributes(ctx, tx, poolUUID, pool.Attrs); err != nil {
			return errors.Annotatef(domain.CoerceError(err), "creating storage pool %s attributes", poolUUID)
		}
		return nil
	})

	return errors.Trace(err)
}

func upsertStoragePool(ctx context.Context, tx *sqlair.TX, poolUUID string, name string, providerType string) error {
	if name == "" {
		return fmt.Errorf("storage pool name cannot be empty%w", errors.Hide(errors.NotValid))
	}
	if providerType == "" {
		return fmt.Errorf("storage pool type cannot be empty%w", errors.Hide(errors.NotValid))
	}

	dbPool := StoragePool{
		ID:           poolUUID,
		Name:         name,
		ProviderType: providerType,
	}

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
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, dbPool).Run()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func updatePoolAttributes(ctx context.Context, tx *sqlair.TX, storagePoolUUID string, attr domainstorage.Attrs) error {
	// Delete any keys no longer in the attributes map.
	// TODO(wallyworld) - use sqlair NOT IN operation
	deleteQuery := fmt.Sprintf(`
DELETE FROM  storage_pool_attribute
WHERE        storage_pool_uuid = $M.uuid
-- AND          key NOT IN (?)
`)

	deleteStmt, err := sqlair.Prepare(deleteQuery, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	if err := tx.Query(ctx, deleteStmt, sqlair.M{"uuid": storagePoolUUID}).Run(); err != nil {
		return errors.Trace(domain.CoerceError(err))
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
		return errors.Trace(err)
	}

	for key, value := range attr {
		if err := tx.Query(ctx, insertStmt, poolAttribute{
			ID:    storagePoolUUID,
			Key:   key,
			Value: value,
		}).Run(); err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
	}
	return nil
}

// DeleteStoragePool deletes a storage pool, returning an error satisfying
// [errors.NotFound] if it doesn't exist.
func (st State) DeleteStoragePool(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	poolAttributeDeleteQ := `
DELETE FROM storage_pool_attribute
WHERE  storage_pool_attribute.storage_pool_uuid = (select uuid FROM storage_pool WHERE name = $M.name)
`

	poolDeleteQ := `
DELETE FROM storage_pool
WHERE  storage_pool.uuid = (select uuid FROM storage_pool WHERE name = $M.name)
`

	poolAttributeDeleteStmt, err := sqlair.Prepare(poolAttributeDeleteQ, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	poolDeleteStmt, err := sqlair.Prepare(poolDeleteQ, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		nameMap := sqlair.M{"name": name}
		if err := tx.Query(ctx, poolAttributeDeleteStmt, nameMap).Run(); err != nil {
			return errors.Annotate(domain.CoerceError(err), "deleting storage pool attributes")
		}
		var outcome = sqlair.Outcome{}
		err = tx.Query(ctx, poolDeleteStmt, nameMap).Get(&outcome)
		if err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
		rowsAffected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Annotate(domain.CoerceError(err), "deleting storage pool")
		}
		if rowsAffected == 0 {
			return fmt.Errorf("storage pool %q %w", name, errors.NotFound)
		}
		return errors.Annotate(domain.CoerceError(err), "deleting storage pool")
	})
}

// ReplaceStoragePool replaces an existing storage pool, returning an error
// satisfying [errors.NotFound] if a pool with the name does not exist.
func (st State) ReplaceStoragePool(ctx context.Context, pool domainstorage.StoragePoolDetails) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	selectUUIDStmt, err := sqlair.Prepare("SELECT &StoragePool.uuid FROM storage_pool WHERE name = $StoragePool.name", StoragePool{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		dbPool := StoragePool{Name: pool.Name}
		err := tx.Query(ctx, selectUUIDStmt, dbPool).Get(&dbPool)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(domain.CoerceError(err))
		}
		if err != nil {
			return fmt.Errorf("storage pool %q %w", pool.Name, errors.NotFound)
		}
		poolUUID := dbPool.ID
		if err := upsertStoragePool(ctx, tx, poolUUID, pool.Name, pool.Provider); err != nil {
			return errors.Annotate(domain.CoerceError(err), "updating storage pool")
		}

		if err := updatePoolAttributes(ctx, tx, poolUUID, pool.Attrs); err != nil {
			return errors.Annotatef(domain.CoerceError(err), "updating storage pool %s attributes", poolUUID)
		}
		return nil
	})

	return errors.Trace(err)
}

func (st State) loadStoragePools(ctx context.Context, tx *sqlair.TX, filter domainstorage.StoragePoolFilter) ([]domainstorage.StoragePoolDetails, error) {
	query := `
SELECT (sp.uuid, sp.name, sp.type) AS (&StoragePool.*),
       (sp_attr.key, sp_attr.value) AS (&poolAttribute.*)
FROM   storage_pool sp
       LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
`

	types := []any{
		StoragePool{},
		poolAttribute{},
	}

	var queryArgs []any
	condition, args := st.buildFilter(filter)
	if len(args) > 0 {
		query = query + "WHERE " + condition
		types = append(types, args...)
		queryArgs = append([]any{}, args...)
	}

	queryStmt, err := sqlair.Prepare(query, types...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		dbRows    StoragePools
		keyValues []poolAttribute
	)
	err = tx.Query(ctx, queryStmt, queryArgs...).GetAll(&dbRows, &keyValues)
	if err != nil {
		return nil, errors.Annotate(domain.CoerceError(err), "loading storage pool")
	}
	return dbRows.toStoragePools(keyValues)
}

// ListStoragePools returns the storage pools matching the specified filter.
func (st State) ListStoragePools(ctx context.Context, filter domainstorage.StoragePoolFilter) ([]domainstorage.StoragePoolDetails, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []domainstorage.StoragePoolDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		result, err = st.loadStoragePools(ctx, tx, filter)
		return errors.Trace(err)
	})
	return result, errors.Trace(err)
}

func (st State) buildFilter(filter domainstorage.StoragePoolFilter) (string, []any) {
	if len(filter.Names) == 0 && len(filter.Providers) == 0 {
		return "", nil
	}

	if len(filter.Names) > 0 && len(filter.Providers) > 0 {
		condition := "sp.name IN ($StoragePoolNames[:]) AND sp.type IN ($StorageProviderTypes[:])"
		return condition, []any{StoragePoolNames(filter.Names), StorageProviderTypes(filter.Providers)}
	}

	if len(filter.Names) > 0 {
		condition := "sp.name IN ($StoragePoolNames[:])"
		return condition, []any{StoragePoolNames(filter.Names)}
	}

	condition := "sp.type IN ($StorageProviderTypes[:])"
	return condition, []any{StorageProviderTypes(filter.Providers)}
}

// GetStoragePoolByName returns the storage pool with the specified name, returning an error
// satisfying [errors.NotFound] if it doesn't exist.
func (st State) GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error) {
	db, err := st.DB()
	if err != nil {
		return domainstorage.StoragePoolDetails{}, errors.Trace(err)
	}

	var storagePools []domainstorage.StoragePoolDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		storagePools, err = st.loadStoragePools(ctx, tx, domainstorage.StoragePoolFilter{
			Names: []string{name},
		})
		return errors.Trace(err)
	})
	if err != nil {
		return domainstorage.StoragePoolDetails{}, errors.Trace(err)
	}
	if len(storagePools) == 0 {
		return domainstorage.StoragePoolDetails{}, fmt.Errorf("storage pool %q %w", name, errors.NotFound)
	}
	if len(storagePools) > 1 {
		return domainstorage.StoragePoolDetails{}, errors.Errorf("expected 1 storage pool, got %d", len(storagePools))
	}
	return storagePools[0], errors.Trace(err)
}
