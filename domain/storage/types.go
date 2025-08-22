// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/collections/set"

	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
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

// Values returns the unique values of the Providers.
func (p Providers) Values() []string {
	return deduplicateNamesOrProviders(p)
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
