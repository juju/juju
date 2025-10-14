// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

//go:generate go run go.uber.org/mock/mockgen -typed -package storage -destination storage_mock_test.go github.com/juju/juju/domain/application/service/storage ProviderState,State,StoragePoolProvider
//go:generate go run go.uber.org/mock/mockgen -typed -package storage -mock_names=Provider=MockStorageProvider -destination internal_storage_mock_test.go github.com/juju/juju/internal/storage Provider,ProviderRegistry

func ptr[T any](v T) *T {
	return &v
}
