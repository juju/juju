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

// StoragePoolProvider defines the interface by where provider based questions
// for storage pools can be asked. This interface acts as the bridge between a
// storage pool and the underlying provider that is used.
//
// The definition lives in the storageprovisioning domain, so that it can be
// shared with the relation domain, which resolves storage directives through
// the same providers when making the storage arguments for subordinate units.
type StoragePoolProvider = domainstorageprovisioning.StoragePoolProvider

// DefaultStoragePoolProvider is the default implementation of
// [StoragePoolProvider] for this domain.
type DefaultStoragePoolProvider struct {
	providerRegistryGetter corestorage.ModelStorageRegistryGetter
	st                     ProviderState
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
// - [storageerrors.StoragePoolNotFound] when no storage pool exists for the
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

	storageKind, err := domainstorageprovisioning.StorageKindFromCharmStorageType(storageType)
	if err != nil {
		return false, err
	}

	return domainstorageprovisioning.CheckStorageProviderSupportsStorageKind(
		provider, storageKind,
	), nil
}

// GetProviderForPool returns the storage provider that is backing a given
// storage pool. This is a utility func for this domain to enable asking
// questions of a provider when you are starting with a storage pool.
//
// The following errors may be expected:
// - [coreerrors.NotValid] if the provided pool uuid is not valid.
// - [storageerrors.StoragePoolNotFound] when no storage pool exists for the
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
