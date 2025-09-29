// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	domainstorage "github.com/juju/juju/domain/storage"
	storageservice "github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/internal/storage"
)

type BlockDeviceService interface {
	GetBlockDevicesForMachine(
		ctx context.Context, machineUUID machine.UUID,
	) ([]blockdevice.BlockDevice, error)
}

// StorageService defines apis on the storage service.
type StorageService interface {
	// CreateStoragePool creates a storage pool with the specified configuration.
	// The following errors can be expected:
	// - [storageerrors.PoolAlreadyExists] if a pool with the same name already exists.
	CreateStoragePool(
		ctx context.Context, name string, providerType storage.ProviderType, attrs storageservice.PoolAttrs,
	) error

	// DeleteStoragePool deletes a storage pool with the specified name.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	DeleteStoragePool(ctx context.Context, name string) error

	// ReplaceStoragePool replaces an existing storage pool with the specified configuration.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	ReplaceStoragePool(
		ctx context.Context, name string, providerType storage.ProviderType, attrs storageservice.PoolAttrs,
	) error

	// ListStoragePools returns all the storage pools.
	ListStoragePools(ctx context.Context) ([]domainstorage.StoragePool, error)

	// ListStoragePoolsByNamesAndProviders returns the storage pools matching the specified
	// names and providers, including the default storage pools.
	// If no names and providers are specified, an empty slice is returned without an error.
	// If no storage pools match the criteria, an empty slice is returned without an error.
	ListStoragePoolsByNamesAndProviders(
		ctx context.Context, names domainstorage.Names, providers domainstorage.Providers,
	) ([]domainstorage.StoragePool, error)

	// ListStoragePoolsByNames returns the storage pools matching the specified names, including
	// the default storage pools.
	// If no names are specified, an empty slice is returned without an error.
	// If no storage pools match the criteria, an empty slice is returned without an error.
	ListStoragePoolsByNames(
		ctx context.Context, names domainstorage.Names,
	) ([]domainstorage.StoragePool, error)

	// ListStoragePoolsByProviders returns the storage pools matching the specified
	// providers, including the default storage pools.
	// If no providers are specified, an empty slice is returned without an error.
	// If no storage pools match the criteria, an empty slice is returned without an error.
	ListStoragePoolsByProviders(
		ctx context.Context, providers domainstorage.Providers,
	) ([]domainstorage.StoragePool, error)

	// GetStoragePoolByName returns the storage pool with the specified name.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error)

	// ListStorageInstances returns a list of storage instances in the model.
	ListStorageInstances(ctx context.Context) ([]domainstorage.StorageInstanceInfo, error)
}

// ApplicationService defines apis on the application service.
type ApplicationService interface {
	GetUnitMachineName(ctx context.Context, unitName unit.Name) (machine.Name, error)
}
