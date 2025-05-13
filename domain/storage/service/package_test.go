// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"

	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/internal/storage"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination state_mock_test.go github.com/juju/juju/domain/storage/service State
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination storage_mock_test.go github.com/juju/juju/core/storage ModelStorageRegistryGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination internal_storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry,Provider,VolumeSource,VolumeImporter,FilesystemSource,FilesystemImporter

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type modelStorageRegistryGetter func() storage.ProviderRegistry

var _ corestorage.ModelStorageRegistryGetter = modelStorageRegistryGetter(nil)

func (m modelStorageRegistryGetter) GetStorageRegistry(context.Context) (storage.ProviderRegistry, error) {
	return m(), nil
}
