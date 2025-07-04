// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

func (st State) getStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error) {
	poolUUID, err := st.GetStoragePoolUUID(ctx, name)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Errorf(
			"getting storage pool %q UUID: %w", name, err,
		)
	}

	pool, err := st.GetStoragePool(ctx, poolUUID)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}
	return pool, nil
}
