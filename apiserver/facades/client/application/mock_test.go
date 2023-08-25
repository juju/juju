// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	jtesting "github.com/juju/testing"

	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/poolmanager"
)

type mockStoragePoolManager struct {
	jtesting.Stub
	poolmanager.PoolManager
	storageType storage.ProviderType
}

func (m *mockStoragePoolManager) Get(name string) (*storage.Config, error) {
	m.MethodCall(m, "Get", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return storage.NewConfig(name, m.storageType, map[string]interface{}{"foo": "bar"})
}

type mockStorageRegistry struct {
	jtesting.Stub
	storage.ProviderRegistry
}
