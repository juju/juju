// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/collections/set"

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

// StoragePoolConfig defines the config of a storage pool.
type StoragePoolConfig struct {
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

func (n Names) Values() []string {
	if n == nil {
		return nil
	}
	return set.NewStrings(n...).Values()
}

func (p Providers) Values() []string {
	if p == nil {
		return nil
	}
	return set.NewStrings(p...).Values()
}

// BuiltInStoragePools returns the built in providers common to all.
func BuiltInStoragePools() ([]StoragePoolConfig, error) {
	providerTypes, err := provider.CommonStorageProviders().StorageProviderTypes()
	if err != nil {
		return nil, errors.Errorf("getting built in storage provider types: %w", err)
	}
	result := make([]StoragePoolConfig, len(providerTypes))
	for i, pType := range providerTypes {
		result[i] = StoragePoolConfig{
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
