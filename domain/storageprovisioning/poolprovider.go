// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"context"

	"github.com/juju/juju/core/trace"
	domainapplicationcharm "github.com/juju/juju/domain/application/charm"
	domainstorage "github.com/juju/juju/domain/storage"
	internalstorage "github.com/juju/juju/internal/storage"
)

// StoragePoolProvider defines the interface by where provider based questions
// for storage pools can be asked. This interface acts as the bridge between a
// storage pool and the underlying provider that is used.
type StoragePoolProvider interface {
	// CheckPoolSupportsCharmStorage checks that the provided storage
	// pool uuid can be used for provisioning a certain type of charm storage.
	//
	// The following errors may be expected:
	// - [coreerrors.NotValid] if the provided pool uuid is not valid.
	// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
	// provided pool uuid.
	CheckPoolSupportsCharmStorage(
		context.Context,
		domainstorage.StoragePoolUUID,
		domainapplicationcharm.StorageType,
	) (bool, error)

	// GetProviderForPool returns the storage provider that is backing a given
	// storage pool. This is a utility func for this domain to enable asking
	// questions of a provider when you are starting with a storage pool.
	//
	// The following errors may be expected:
	// - [coreerrors.NotValid] if the provided pool uuid is not valid.
	// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
	// provided pool uuid.
	GetProviderForPool(
		context.Context, domainstorage.StoragePoolUUID,
	) (internalstorage.Provider, error)
}

// CachedStoragePoolProvider is a special implementation of
// [StoragePoolProvider] it exists to provide a temporary read through cache of
// storage providers used by a storage pool.
//
// For example if the provider is asked to provide the provider for a storage
// pool it will cache the provider so that future questions of the same pool can
// return the provider in the cache.
//
// This type exists to be short lived. It should only ever be created for single
// operation that requires fetching a storage pools provider multiple times in
// the operation.
//
// This implementation is NOT thread safe and never will be. Short operations
// with a defined end that ask the same question repeatedly is that this type
// exists to solve.
type CachedStoragePoolProvider struct {
	// StoragePoolProvider is the storage pool provider that is wrapped by this
	// cache.
	StoragePoolProvider

	// Cache is the internal cache used. This value must be initialised by the
	// user.
	Cache map[domainstorage.StoragePoolUUID]internalstorage.Provider
}

// GetProviderForPool returns the storage provider associated with the given
// storage pool. This func will first consult the cache to see if the provider
// is available there and then if not proxy the call through to the underlying
// [StoragePoolProvider].
//
// This func is not thread safe and never will be. Implements the
// [StoragePoolProvider] interface.
//
// The following errors may be expected:
// - [coreerrors.NotValid] if the provided pool uuid is not valid.
// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
// provided pool uuid.
func (c CachedStoragePoolProvider) GetProviderForPool(
	ctx context.Context,
	poolUUID domainstorage.StoragePoolUUID,
) (internalstorage.Provider, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, has := c.Cache[poolUUID]
	if has {
		return provider, nil
	}

	provider, err := c.StoragePoolProvider.GetProviderForPool(ctx, poolUUID)
	if err != nil {
		return nil, err
	}

	c.Cache[poolUUID] = provider
	return provider, nil
}
