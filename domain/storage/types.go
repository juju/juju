// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

// Pool configuration attribute names.
const (
	StoragePoolName     = "name"
	StorageProviderType = "type"
)

// Attrs defines storage attributes.
type Attrs map[string]string

// StoragePoolDetails defines the details of a storage pool to save.
// This type is also used when returning query results from state.
type StoragePoolDetails struct {
	Name     string
	Provider string
	Attrs    Attrs
}

// These type aliases are used to specify filter terms.
type (
	Names     []string
	Providers []string
)

// These consts are used to specify nil filter terms.
var (
	NilNames     = Names(nil)
	NilProviders = Providers(nil)
)

// BuiltInStoragePools returns the built in providers common to all.
func BuiltInStoragePools() ([]StoragePoolDetails, error) {
	providerTypes, err := provider.CommonStorageProviders().StorageProviderTypes()
	if err != nil {
		return nil, errors.Errorf("getting built in storage provider types: %w", err)
	}
	result := make([]StoragePoolDetails, len(providerTypes))
	for i, pType := range providerTypes {
		result[i] = StoragePoolDetails{
			Name:     string(pType),
			Provider: string(pType),
		}
	}
	return result, nil
}

// DefaultStoragePools returns the default storage pools to add to a new model
// for a given provider registry.
func DefaultStoragePools(registry storage.ProviderRegistry) ([]*storage.Config, error) {
	var result []*storage.Config
	providerTypes, err := registry.StorageProviderTypes()
	if err != nil {
		return nil, errors.Errorf("getting storage provider types: %w", err)
	}
	for _, providerType := range providerTypes {
		p, err := registry.StorageProvider(providerType)
		if err != nil {
			return nil, errors.Capture(err)
		}
		result = append(result, p.DefaultPools()...)
	}
	return result, nil
}

// ModelDetails describes details about a model.
type ModelDetails struct {
	ModelUUID      string
	ControllerUUID string
}

// FilesystemInfo describes information about a filesystem.
type FilesystemInfo struct {
	storage.FilesystemInfo
	Pool          string
	BackingVolume *storage.VolumeInfo
}
