// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	gc "gopkg.in/check.v1"

	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/internal/storage"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination state_mock_test.go github.com/juju/juju/domain/storage/service State,StoragePoolState
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination storage_mock_test.go github.com/juju/juju/core/storage ModelStorageRegistryGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination internal_storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type modelStorageRegistryGetter func() storage.ProviderRegistry

var _ corestorage.ModelStorageRegistryGetter = modelStorageRegistryGetter(nil)

func (m modelStorageRegistryGetter) GetStorageRegistry(context.Context) (storage.ProviderRegistry, error) {
	return m(), nil
}
