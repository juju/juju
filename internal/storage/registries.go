// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"maps"
	"slices"

	"github.com/juju/errors"
)

// StaticProviderRegistry is a storage provider registry with a statically
// defined set of providers.
type StaticProviderRegistry struct {
	// Providers contains the storage providers for this registry.
	Providers map[ProviderType]Provider
}

// RecommendedPoolForKind currently returns a nil pool. This is not implemented.
func (r StaticProviderRegistry) RecommendedPoolForKind(_ StorageKind) *Config {
	return nil
}

// StorageProviderTypes returns all the provider types located within this
// registry. The returns slice is sorted in ascending order.
//
// Implements [Provider.StorageProviderTypes].
func (r StaticProviderRegistry) StorageProviderTypes() ([]ProviderType, error) {
	return slices.Sorted(maps.Keys(r.Providers)), nil
}

// StorageProvider implements ProviderRegistry.
func (r StaticProviderRegistry) StorageProvider(t ProviderType) (Provider, error) {
	p, ok := r.Providers[t]
	if ok {
		return p, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

// NotImplementedProviderRegistry is a storage provider registry that
// returns an error satisfying [errors.NotImplemented] for any method call.
type NotImplementedProviderRegistry struct{}

// RecommendedPoolForKind current returns a nill storage pool and is not
// implemented.
func (NotImplementedProviderRegistry) RecommendedPoolForKind(_ StorageKind) *Config {
	return nil
}

// StorageProviderTypes implements ProviderRegistry.
func (r NotImplementedProviderRegistry) StorageProviderTypes() ([]ProviderType, error) {
	return nil, errors.NotImplementedf(`"StorageProviderTypes"`)
}

// StorageProvider implements ProviderRegistry.
func (r NotImplementedProviderRegistry) StorageProvider(t ProviderType) (Provider, error) {
	return nil, errors.NotImplementedf(`"StorageProvider"`)
}
