// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/application/charm"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// cachedStoragePoolProvider is a special implementation of
// [StoragePoolProvider] it exists to provide a temporary read through cache of
// storage providers used by a storage pool.
//
// For example if the provider is asked to provide the provider for a storage
// pool it will cache the provider so that future questions of the same pool can
// return the provider in the cache.
//
// This type exists to be short lived. It should only ever be created for single
// operation that requires fetching a storage pools provide multiple times in
// the operation.
//
// This implementation is NOT thread safe and never will be. Short operations
// with a defined end that ask the same question repeatedly is that this type
// exists to solve.
type cachedStoragePoolProvider struct {
	// StoragePoolProvider is the storage pool provider that is wrapped by this
	// cache.
	StoragePoolProvider

	// Cache is the internal cache used. This value must be initialised by the
	// user.
	Cache map[domainstorage.StoragePoolUUID]storage.Provider
}

// DefaultStoragePoolProvider is the default implementation of
// [StoragePoolProvider] for this domain.
type DefaultStoragePoolProvider struct {
	providerRegistryGetter corestorage.ModelStorageRegistryGetter
	st                     ProviderState
}

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
		charm.StorageType,
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
	) (storage.Provider, error)
}

// NewStoragePoolProvider returns a new [DefaultStoragePoolProvider]
// that allows getting provider information for a storage pool.
//
// The returned [DefaultStoragePoolProvider] implements the
// [StoragePoolProvider] interface.
func NewStoragePoolProvider(
	providerRegistryGetter corestorage.ModelStorageRegistryGetter,
	st ProviderState,
) *DefaultStoragePoolProvider {
	return &DefaultStoragePoolProvider{
		providerRegistryGetter: providerRegistryGetter,
		st:                     st,
	}
}

// CheckPoolSupportsCharmStorage checks that the provided storage
// pool uuid can be used for provisioning a certain type of charm storage.
//
// The following errors may be expected:
// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
// provided pool uuid.
func (v *DefaultStoragePoolProvider) CheckPoolSupportsCharmStorage(
	ctx context.Context,
	poolUUID domainstorage.StoragePoolUUID,
	storageType charm.StorageType,
) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := v.GetProviderForPool(ctx, poolUUID)
	if err != nil {
		return false, errors.Capture(err)
	}

	storageKind, err := encodeStorageKindFromCharmStorageType(storageType)
	if err != nil {
		return false, err
	}

	return domainstorageprovisioning.CheckStorageProviderSupportsStorageKind(
		provider, storageKind,
	), nil
}

// GetProviderForPool returns the storage provider associated with the given
// storage pool. This func will first consult the cache to see if the provider
// is available there and then if not proxy the call through to the underlying
// [StorageProviderPool].
//
// This func is not thread safe and never will be. Implements the
// [StorageProviderPool] interface.
//
// The following errors may be expected:
// - [coreerrors.NotValid] if the provided pool uuid is not valid.
// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
// provided pool uuid.
func (c cachedStoragePoolProvider) GetProviderForPool(
	ctx context.Context,
	poolUUID domainstorage.StoragePoolUUID,
) (storage.Provider, error) {
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

// GetProviderForPool returns the storage provider that is backing a given
// storage pool. This is a utility func for this domain to enable asking
// questions of a provider when you are starting with a storage pool.
//
// The following errors may be expected:
// - [coreerrors.NotValid] if the provided pool uuid is not valid.
// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
// provided pool uuid.
func (v *DefaultStoragePoolProvider) GetProviderForPool(
	ctx context.Context,
	poolUUID domainstorage.StoragePoolUUID,
) (storage.Provider, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := poolUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"storage pool uuid is not valid: %w", err,
		).Add(coreerrors.NotValid)
	}

	providerTypeStr, err := v.st.GetProviderTypeForPool(ctx, poolUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	providerRegistry, err := v.providerRegistryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting model storage provider registry: %w", err,
		)
	}

	providerType := storage.ProviderType(providerTypeStr)
	provider, err := providerRegistry.StorageProvider(providerType)
	// We check if the error is for the provider type not being found and
	// translate it over to a ProviderTypeNotFound error. This error type is not
	// recorded in the contract as  this should never be possible. But we are
	// being a good citizen and returning meaningful errors.
	if errors.Is(err, coreerrors.NotFound) {
		return nil, errors.Errorf(
			"provider type %q for storage pool %q does not exist",
			providerTypeStr, poolUUID,
		).Add(storageerrors.ProviderTypeNotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"getting storage provider for pool %q: %w", poolUUID, err,
		)
	}

	return provider, nil
}
