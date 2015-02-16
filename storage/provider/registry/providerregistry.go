// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

//
// A registry of storage providers.
//

// providers maps from provider type to storage.Provider for
// each registered provider type.
var providers = make(map[storage.ProviderType]storage.Provider)

// RegisterProvider registers a new storage provider of the given type.
func RegisterProvider(providerType storage.ProviderType, p storage.Provider) {
	if providers[providerType] != nil {
		panic(errors.Errorf("juju: duplicate storage provider type %q", providerType))
	}
	providers[providerType] = p
}

// StorageProvider returns the previously registered provider with the given type.
func StorageProvider(providerType storage.ProviderType) (storage.Provider, error) {
	p, ok := providers[providerType]
	if !ok {
		return nil, errors.NotFoundf("storage provider %q", providerType)
	}
	return p, nil
}

//
// A registry of storage provider types which are
// valid for a Juju Environ.
//

// supportedEnvironProviders maps from environment type to a slice of
// supported ProviderType(s).
var supportedEnvironProviders = make(map[string][]storage.ProviderType)

// RegisterEnvironStorageProviders records which storage provider types
// are valid for an environment.
// This is to be called from the environ provider's init().
// Also registered will be provider types common to all environments.
func RegisterEnvironStorageProviders(envType string, providers ...storage.ProviderType) {
	existing := supportedEnvironProviders[envType]
	for _, p := range providers {
		if IsProviderSupported(envType, p) {
			continue
		}
		existing = append(existing, p)
	}

	// Add the common providers.
	for p := range provider.CommonProviders() {
		if IsProviderSupported(envType, p) {
			continue
		}
		existing = append(existing, p)
	}
	supportedEnvironProviders[envType] = existing
}

// Returns true is provider is supported for the environment.
func IsProviderSupported(envType string, providerType storage.ProviderType) bool {
	providerTypes, ok := supportedEnvironProviders[envType]
	if !ok {
		return false
	}
	for _, p := range providerTypes {
		if p == providerType {
			return true
		}
	}
	return false
}

type defaultStoragePool map[storage.StorageKind]string

// defaultPools records the default block and filesystem pools to be
// used for an environment, if none is specified by the user when deploying.
var defaultPools map[string]defaultStoragePool = make(map[string]defaultStoragePool)

// RegisterDefaultPool records the default pool for the storage kind and environment.
// NOTE: the pool is not validated as to whether it exists, or if its type is
// supported by the environment. This is expected to be done by the caller.
func RegisterDefaultPool(envType string, kind storage.StorageKind, pool string) {
	if _, ok := defaultPools[envType]; !ok {
		defaultPools[envType] = make(defaultStoragePool)
	}
	defaultPools[envType][kind] = pool
}

// DefaultPool returns the default storage pool for the storage kind and environment.
func DefaultPool(envType string, kind storage.StorageKind) (string, bool) {
	pool, ok := defaultPools[envType][kind]
	return pool, ok && pool != ""
}
