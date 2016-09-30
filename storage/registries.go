// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"sort"

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
		if errors.IsNotFound(err) {
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

// StorageProviderTypes implements ProviderRegistry.
func (r StaticProviderRegistry) StorageProviderTypes() ([]ProviderType, error) {
	typeStrings := make([]string, 0, len(r.Providers))
	for t := range r.Providers {
		typeStrings = append(typeStrings, string(t))
	}
	sort.Strings(typeStrings)
	types := make([]ProviderType, len(typeStrings))
	for i, s := range typeStrings {
		types[i] = ProviderType(s)
	}
	return types, nil
}

// StorageProvider implements ProviderRegistry.
func (r StaticProviderRegistry) StorageProvider(t ProviderType) (Provider, error) {
	p, ok := r.Providers[t]
	if ok {
		return p, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}
