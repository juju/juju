// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"maps"
	"slices"

	"github.com/juju/errors"
)

// ChainedProviderRegistry is storage provider registry that combines
// multiple storage provider registries, chaining their results. Registries
// earlier in the chain take precedence.
type ChainedProviderRegistry []ProviderRegistry

// StorageProviderTypes implements ProviderRegistry.
func (r ChainedProviderRegistry) StorageProviderTypes() ([]ProviderType, error) {
	var result []ProviderType
	for _, r := range r {
		types, err := r.StorageProviderTypes()
		if err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, types...)
	}
	return result, nil
}

// StorageProvider implements ProviderRegistry.
func (r ChainedProviderRegistry) StorageProvider(t ProviderType) (Provider, error) {
	for _, r := range r {
		p, err := r.StorageProvider(t)
		if err == nil {
			return p, nil
		}
		if errors.Is(err, errors.NotFound) {
			continue
		}
		return nil, errors.Annotatef(err, "getting storage provider %q", t)
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

// StaticProviderRegistry is a storage provider registry with a statically
// defined set of providers.
type StaticProviderRegistry struct {
	// Providers contains the storage providers for this registry.
	Providers map[ProviderType]Provider
}

// StorageProviderTypes returns all the provider types located within this
// registy. The returns slice is sorted in ascending order.
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

// StorageProviderTypes implements ProviderRegistry.
func (r NotImplementedProviderRegistry) StorageProviderTypes() ([]ProviderType, error) {
	return nil, errors.NotImplementedf(`"StorageProviderTypes"`)
}

// StorageProvider implements ProviderRegistry.
func (r NotImplementedProviderRegistry) StorageProvider(t ProviderType) (Provider, error) {
	return nil, errors.NotImplementedf(`"StorageProvider"`)
}
