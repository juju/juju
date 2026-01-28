// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

// GetStoragePoolUUID returns the UUID of the storage pool for the specified name.
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified name does not exist.
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
		return "", errors.Errorf("storage pool %q not found", name).Add(domainstorageerrors.StoragePoolNotFound)
	}
	if err != nil {
		return "", errors.Errorf("getting storage pool UUID %q: %w", name, err)
	}
	return domainstorage.StoragePoolUUID(inputArg.UUID), nil
}
