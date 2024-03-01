// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"

	jtesting "github.com/juju/testing"

	"github.com/juju/juju/internal/storage"
)

type mockStoragePoolGetter struct {
	jtesting.Stub
	storageType storage.ProviderType
}

func (m *mockStoragePoolGetter) GetStoragePoolByName(_ context.Context, name string) (*storage.Config, error) {
	m.MethodCall(m, "GetStoragePoolByName", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return storage.NewConfig(name, m.storageType, map[string]interface{}{"foo": "bar"})
}

type mockStorageRegistry struct {
	jtesting.Stub
	storage.ProviderRegistry
}
