// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/logger"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// StoragePoolState defines an interface for interacting with the underlying state.
type StoragePoolState interface {
	// CreateStoragePool creates a storage pool with the specified configuration.
	// The following errors can be expected:
	// - [storageerrors.PoolAlreadyExists] if a pool with the same name already exists.
	CreateStoragePool(ctx context.Context, pool domainstorage.StoragePool) error

	// DeleteStoragePool deletes a storage pool with the specified name.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	DeleteStoragePool(ctx context.Context, name string) error

	// ReplaceStoragePool replaces an existing storage pool with the specified configuration.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	ReplaceStoragePool(ctx context.Context, pool domainstorage.StoragePool) error

	// ListStoragePools returns the storage pools including default storage pools.
	ListStoragePools(ctx context.Context) ([]domainstorage.StoragePool, error)

	// ListStoragePoolsWithoutBuiltins returns the storage pools excluding the built-in storage pools.
	ListStoragePoolsWithoutBuiltins(ctx context.Context) ([]domainstorage.StoragePool, error)

	// ListStoragePoolsByNamesAndProviders returns the storage pools matching the specified
	// names and or providers, including the default storage pools.
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
}

// StoragePoolService defines a service for interacting with the underlying state.
type StoragePoolService struct {
	st             StoragePoolState
	logger         logger.Logger
	registryGetter corestorage.ModelStorageRegistryGetter
}

// PoolAttrs define the attributes of a storage pool.
type PoolAttrs map[string]any

// CreateStoragePool creates a storage pool with the specified configuration.
// The following errors can be expected:
// - [storageerrors.PoolAlreadyExists] if a pool with the same name already exists.
func (s *StoragePoolService) CreateStoragePool(ctx context.Context, name string, providerType storage.ProviderType, attrs PoolAttrs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.validateConfig(ctx, name, providerType, attrs); err != nil {
		return errors.Capture(err)
	}

	attrsToSave := transform.Map(attrs, func(k string, v any) (string, string) { return k, fmt.Sprint(v) })
	sp := domainstorage.StoragePool{
		Name:     name,
		Provider: string(providerType),
		Attrs:    attrsToSave,
	}

	if err := s.st.CreateStoragePool(ctx, sp); err != nil {
		return errors.Errorf("creating storage pool %q: %w", name, err)
	}
	return nil
}

func (s *StoragePoolService) validateConfig(ctx context.Context, name string, providerType storage.ProviderType, attrs map[string]interface{}) error {
	if name == "" {
		return storageerrors.MissingPoolNameError
	}
	if !storage.IsValidPoolName(name) {
		return errors.Errorf("pool name %q not valid", name).Add(storageerrors.InvalidPoolNameError)
	}
	if providerType == "" {
		return storageerrors.MissingPoolTypeError
	}

	cfg, err := storage.NewConfig(name, providerType, attrs)
	if err != nil {
		return errors.Capture(err)
	}

	// GetStorageRegistry result for a given model will be cached after the
	// initial call, so this should be cheap to call.
	registry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	p, err := registry.StorageProvider(providerType)
	if err != nil {
		return errors.Capture(err)
	}
	if err := p.ValidateConfig(cfg); err != nil {
		return errors.Errorf("validating storage provider config: %w", err)
	}
	return nil
}

// DeleteStoragePool deletes a storage pool with the specified name.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
func (s *StoragePoolService) DeleteStoragePool(ctx context.Context, name string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// TODO(storage) - check in use when we have storage in dqlite
	// Below is the code from state that will need to be ported.
	/*
		var inUse bool
		cfg, err := sb.config(context.Background())
		if err != nil {
			return errors.Trace(err)
		}
		operatorStorage, ok := cfg.AllAttrs()[k8sconstants.OperatorStorageKey]
		if sb.modelType == ModelTypeCAAS && ok && operatorStorage == poolName {
			apps, err := sb.allApplications()
			if err != nil {
				return errors.Trace(err)
			}
			inUse = len(apps) > 0
		} else {
			query := bson.D{{"constraints.pool", bson.D{{"$eq", poolName}}}}
			pools, err := storageCollection.Find(query).Count()
			if err != nil {
				return errors.Trace(err)
			}
			inUse = pools > 0
		}
		if inUse {
			return errors.Errorf("storage pool %q in use", poolName)
		}
	*/
	if err := s.st.DeleteStoragePool(ctx, name); err != nil {
		return errors.Errorf("deleting storage pool %q: %w", name, err)
	}
	return nil
}

// ReplaceStoragePool replaces an existing storage pool with the specified configuration.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
func (s *StoragePoolService) ReplaceStoragePool(ctx context.Context, name string, providerType storage.ProviderType, attrs PoolAttrs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Use the existing provider type unless explicitly overwritten.
	if providerType == "" {
		existingConfig, err := s.st.GetStoragePoolByName(ctx, name)
		if err != nil {
			return errors.Capture(err)
		}
		providerType = storage.ProviderType(existingConfig.Provider)
	}

	if err := s.validateConfig(ctx, name, providerType, attrs); err != nil {
		return errors.Capture(err)
	}

	attrsToSave := transform.Map(attrs, func(k string, v any) (string, string) { return k, fmt.Sprint(v) })
	sp := domainstorage.StoragePool{
		Name:     name,
		Provider: string(providerType),
		Attrs:    attrsToSave,
	}

	if err := s.st.ReplaceStoragePool(ctx, sp); err != nil {
		return errors.Errorf("replacing storage pool %q: %w", name, err)
	}
	return nil
}

// ListStoragePoolsWithoutBuiltins returns all storage pools excluding the built-in storage pools.
func (s *StoragePoolService) ListStoragePoolsWithoutBuiltins(ctx context.Context) ([]domainstorage.StoragePool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	pools, err := s.st.ListStoragePoolsWithoutBuiltins(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return pools, nil
}

// ListStoragePools returns the all storage pools including the default storage pools.
func (s *StoragePoolService) ListStoragePools(ctx context.Context) ([]domainstorage.StoragePool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	pools, err := s.st.ListStoragePools(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return pools, nil
}

// ListStoragePoolsByNamesAndProviders returns the storage pools matching the specified
// names and or providers, including the default storage pools.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (s *StoragePoolService) ListStoragePoolsByNamesAndProviders(
	ctx context.Context,
	names domainstorage.Names,
	providers domainstorage.Providers,
) ([]domainstorage.StoragePool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(names) == 0 || len(providers) == 0 {
		return nil, errors.Errorf(
			"at least one name and one provider must be specified, got names: %v, providers: %v",
			names, providers,
		)
	}

	if err := s.validatePoolListFilterTerms(ctx, names, providers); err != nil {
		return nil, errors.Capture(err)
	}

	pools, err := s.st.ListStoragePoolsByNamesAndProviders(ctx, names, providers)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return pools, nil
}

// ListStoragePoolsByNames returns the storage pools matching the specified names, including
// the default storage pools.
// If no names are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (s *StoragePoolService) ListStoragePoolsByNames(
	ctx context.Context,
	names domainstorage.Names,
) ([]domainstorage.StoragePool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.validateNameCriteria(names); err != nil {
		return nil, errors.Capture(err)
	}

	pools, err := s.st.ListStoragePoolsByNames(ctx, names)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return pools, nil
}

// ListStoragePoolsByProviders returns the storage pools matching the specified
// providers, including the default storage pools.
// If no providers are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (s *StoragePoolService) ListStoragePoolsByProviders(
	ctx context.Context,
	providers domainstorage.Providers,
) ([]domainstorage.StoragePool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.validateProviderCriteria(ctx, providers); err != nil {
		return nil, errors.Capture(err)
	}

	pools, err := s.st.ListStoragePoolsByProviders(ctx, providers)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return pools, nil
}

// GetStoragePoolByName returns the storage pool with the specified name.
// The following errors can be expected:
// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
func (s *StoragePoolService) GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !storage.IsValidPoolName(name) {
		return domainstorage.StoragePool{}, errors.Errorf(
			"pool name %q not valid", name,
		).Add(storageerrors.InvalidPoolNameError)
	}

	pool, err := s.st.GetStoragePoolByName(ctx, name)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}
	return pool, nil
}

func (s *StoragePoolService) validatePoolListFilterTerms(ctx context.Context, names domainstorage.Names, providers domainstorage.Providers) error {
	if err := s.validateProviderCriteria(ctx, providers); err != nil {
		return errors.Capture(err)
	}
	if err := s.validateNameCriteria(names); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (s *StoragePoolService) validateNameCriteria(names []string) error {
	if len(names) == 0 {
		// No names specified, so no validation needed.
		return nil
	}

	for _, n := range names {
		if !storage.IsValidPoolName(n) {
			return errors.Errorf("pool name %q not valid", n).Add(storageerrors.InvalidPoolNameError)
		}
	}
	return nil
}

func (s *StoragePoolService) validateProviderCriteria(ctx context.Context, providers []string) error {
	if len(providers) == 0 {
		// No providers specified, so no validation needed.
		return nil
	}

	// GetStorageRegistry result for a given model will be cached after the
	// initial call, so this should be cheap to call.
	registry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	for _, p := range providers {
		_, err := registry.StorageProvider(storage.ProviderType(p))
		if err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}
