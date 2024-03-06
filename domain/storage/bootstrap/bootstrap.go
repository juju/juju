// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/juju/core/database"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/state"
)

// CreateStoragePools adds the initial default and user specified storage pools to the controller model.
func CreateStoragePools(storagePools []domainstorage.StoragePoolDetails) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		return state.CreateStoragePools(ctx, db, storagePools)
	}
}
