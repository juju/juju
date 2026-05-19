// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/internal/storage"
)

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination state_mock_test.go github.com/juju/juju/domain/storage/service State,StoragePoolState,StorageImportState
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination storage_mock_test.go github.com/juju/juju/core/storage ModelStorageRegistryGetter
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination storage_mock_test.go github.com/juju/juju/core/storage ModelStorageRegistryGetter
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination internal_storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry,Provider,VolumeSource,VolumeImporter,FilesystemSource,FilesystemImporter,FilesystemModelMigration

type modelStorageRegistryGetter func() storage.ProviderRegistry

var _ corestorage.ModelStorageRegistryGetter = modelStorageRegistryGetter(nil)

func (m modelStorageRegistryGetter) GetStorageRegistry(context.Context) (storage.ProviderRegistry, error) {
	return m(), nil
}
