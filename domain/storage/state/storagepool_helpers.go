// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

// GetStoragePoolUUID returns the UUID of the storage pool for the specified name.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
// Exported for use by other domains that need to load storage pools.
func GetStoragePoolUUID(
	ctx context.Context,
	tx *sqlair.TX,
	preparer domain.Preparer,
	name string,
) (domainstorage.StoragePoolUUID, error) {
	inputArg := storagePoolIdentifiers{Name: name}
	stmt, err := preparer.Prepare(`
SELECT &storagePoolIdentifiers.uuid
FROM   storage_pool
WHERE  name = $storagePoolIdentifiers.name`, inputArg)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, inputArg).Get(&inputArg)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("storage pool %q not found", name).Add(storageerrors.PoolNotFoundError)
	}
	if err != nil {
		return "", errors.Errorf("getting storage pool UUID %q: %w", name, err)
	}
	return domainstorage.StoragePoolUUID(inputArg.UUID), nil
}

// GetStoragePool returns the storage pool for the specified UUID.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified UUID does not exist.
// Exported for use by other domains that need to load storage pools.
func GetStoragePool(
	ctx context.Context,
	tx *sqlair.TX,
	preparer domain.Preparer,
	poolUUID domainstorage.StoragePoolUUID,
) (domainstorage.StoragePool, error) {
	inputArg := storagePool{ID: poolUUID.String()}
	stmt, err := preparer.Prepare(`
SELECT   (sp.*) AS (&storagePool.*),
         (sp_attr.*) AS (&poolAttribute.*)
FROM     storage_pool sp
         LEFT JOIN storage_pool_attribute sp_attr ON sp_attr.storage_pool_uuid = sp.uuid
WHERE    sp.uuid = $storagePool.uuid`, inputArg, poolAttribute{})
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}

	var (
		dbRows    storagePools
		keyValues []poolAttribute
	)
	err = tx.Query(ctx, stmt, inputArg).GetAll(&dbRows, &keyValues)
	if errors.Is(err, sqlair.ErrNoRows) {
		return domainstorage.StoragePool{}, errors.Errorf(
			"storage pool %q", poolUUID,
		).Add(storageerrors.PoolNotFoundError)
	}
	if err != nil {
		return domainstorage.StoragePool{}, errors.Errorf("getting storage pools: %w", err)
	}
	storagePools, err := dbRows.toStoragePools(keyValues)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}

	if len(storagePools) == 0 {
		return domainstorage.StoragePool{}, errors.Errorf(
			"storage pool %q", poolUUID,
		).Add(storageerrors.PoolNotFoundError)
	}
	if len(storagePools) > 1 {
		return domainstorage.StoragePool{}, errors.Errorf("expected 1 storage pool, got %d", len(storagePools))
	}
	return storagePools[0], errors.Capture(err)
}
