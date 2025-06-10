// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"slices"

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

// StoragePool represents a storage pool in Juju.
// It contains the name of the pool, the provider type, and any attributes
type StoragePool struct {
	UUID     string
	Name     string
	Provider string
	Attrs    Attrs
}

// These type aliases are used to specify filter terms.
type (
	Names     []string
	Providers []string
)

func deduplicateNamesOrProviders[T ~[]string](namesOrProviders T) T {
	if len(namesOrProviders) == 0 {
		return nil
	}
	// Ensure uniqueness and no empty values.
	result := set.NewStrings()
	for _, v := range namesOrProviders {
		if v != "" {
			result.Add(v)
		}
	}
	if result.IsEmpty() {
		return nil
	}
	return T(result.Values())
}

// Values returns the unique values of the Names.
func (n Names) Values() []string {
	return deduplicateNamesOrProviders(n)
}

// Contains checks if the Names contains a specific name.
// It returns true if the name is found, false otherwise.
// If the Names is empty, it returns false.
func (n Names) Contains(name string) bool {
	return slices.Contains(n, name)
}

// Values returns the unique values of the Providers.
func (p Providers) Values() []string {
	return deduplicateNamesOrProviders(p)
}

// Contains checks if the Providers contains a specific provider.
// It returns true if the provider is found, false otherwise.
// If the Providers is empty, it returns false.
func (p Providers) Contains(provider string) bool {
	return slices.Contains(p, provider)
}

// BuiltInStoragePools returns the built in providers common to all.
func BuiltInStoragePools() ([]StoragePool, error) {
	providerTypes, err := provider.CommonStorageProviders().StorageProviderTypes()
	if err != nil {
		return nil, errors.Errorf("getting built in storage provider types: %w", err)
	}
	result := make([]StoragePool, len(providerTypes))
	for i, pType := range providerTypes {
		result[i] = StoragePool{
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
