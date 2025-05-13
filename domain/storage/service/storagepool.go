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
	CreateStoragePool(ctx context.Context, pool domainstorage.StoragePoolDetails) error
	DeleteStoragePool(ctx context.Context, name string) error
	ReplaceStoragePool(ctx context.Context, pool domainstorage.StoragePoolDetails) error
	ListStoragePools(ctx context.Context, filter domainstorage.Names, providers domainstorage.Providers) ([]domainstorage.StoragePoolDetails, error)
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error)
}

// StoragePoolService defines a service for interacting with the underlying state.
type StoragePoolService struct {
	st             StoragePoolState
	logger         logger.Logger
	registryGetter corestorage.ModelStorageRegistryGetter
}

// PoolAttrs define the attributes of a storage pool.
type PoolAttrs map[string]any

// CreateStoragePool creates a storage pool, returning an error satisfying [errors.AlreadyExists]
// if a pool with the same name already exists.
func (s *StoragePoolService) CreateStoragePool(ctx context.Context, name string, providerType storage.ProviderType, attrs PoolAttrs) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := s.validateConfig(ctx, name, providerType, attrs); err != nil {
		return errors.Capture(err)
	}

	attrsToSave := transform.Map(attrs, func(k string, v any) (string, string) { return k, fmt.Sprint(v) })
	sp := domainstorage.StoragePoolDetails{
		Name:     name,
		Provider: string(providerType),
		Attrs:    attrsToSave,
	}
	err = s.st.CreateStoragePool(ctx, sp)
	if err != nil {
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

// DeleteStoragePool deletes a storage pool, returning an error satisfying
// [errors.NotFound] if it doesn't exist.
func (s *StoragePoolService) DeleteStoragePool(ctx context.Context, name string) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

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

// ReplaceStoragePool replaces an existing storage pool, returning an error
// satisfying [storageerrors.PoolNotFoundError] if a pool with the name does not exist.
func (s *StoragePoolService) ReplaceStoragePool(ctx context.Context, name string, providerType storage.ProviderType, attrs PoolAttrs) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

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
	sp := domainstorage.StoragePoolDetails{
		Name:     name,
		Provider: string(providerType),
		Attrs:    attrsToSave,
	}
	err = s.st.ReplaceStoragePool(ctx, sp)
	if err != nil {
		return errors.Errorf("replacing storage pool %q: %w", name, err)
	}
	return nil
}

// AllStoragePools returns the all storage pools.
func (s *StoragePoolService) AllStoragePools(ctx context.Context) (_ []*storage.Config, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	// ListStoragePools returns the storage pools matching the specified filter.
	return s.ListStoragePools(ctx, domainstorage.NilNames, domainstorage.NilProviders)
}

func (s *StoragePoolService) ListStoragePools(ctx context.Context, names domainstorage.Names, providers domainstorage.Providers) (_ []*storage.Config, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := s.validatePoolListFilterTerms(ctx, names, providers); err != nil {
		return nil, errors.Capture(err)
	}

	pools, err := domainstorage.BuiltInStoragePools()
	if err != nil {
		return nil, errors.Capture(err)
	}

	sp, err := s.st.ListStoragePools(ctx, names, providers)
	if err != nil {
		return nil, errors.Capture(err)
	}
	pools = append(pools, sp...)

	results := make([]*storage.Config, len(pools))
	for i, p := range pools {
		results[i], err = s.storageConfig(ctx, p)
		if err != nil {
			return nil, errors.Capture(err)
		}
	}
	return results, nil
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
	for _, n := range names {
		if !storage.IsValidPoolName(n) {
			return errors.Errorf("pool name %q not valid", n).Add(storageerrors.InvalidPoolNameError)
		}
	}
	return nil
}

func (s *StoragePoolService) validateProviderCriteria(ctx context.Context, providers []string) error {
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

// GetStoragePoolByName returns the storage pool with the specified name, returning an error
// satisfying [storageerrors.PoolNotFoundError] if it doesn't exist.
func (s *StoragePoolService) GetStoragePoolByName(ctx context.Context, name string) (_ *storage.Config, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if !storage.IsValidPoolName(name) {
		return nil, errors.Errorf("pool name %q not valid", name).Add(storageerrors.InvalidPoolNameError)
	}

	builtIn, err := domainstorage.BuiltInStoragePools()
	if err != nil {
		return nil, errors.Capture(err)
	}
	for _, p := range builtIn {
		if p.Name == name {
			return s.storageConfig(ctx, p)
		}
	}

	sp, err := s.st.GetStoragePoolByName(ctx, name)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return s.storageConfig(ctx, sp)
}

func (s *StoragePoolService) storageConfig(ctx context.Context, sp domainstorage.StoragePoolDetails) (*storage.Config, error) {
	// GetStorageRegistry result for a given model will be cached after the
	// initial call, so this should be cheap to call.
	registry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var attr map[string]any
	if len(sp.Attrs) > 0 {
		attr = transform.Map(sp.Attrs, func(k, v string) (string, any) { return k, v })
	}
	cfg, err := storage.NewConfig(sp.Name, storage.ProviderType(sp.Provider), attr)
	if err != nil {
		return nil, errors.Capture(err)
	}
	p, err := registry.StorageProvider(cfg.Provider())
	if err != nil {
		return nil, errors.Capture(err)
	}
	if err := p.ValidateConfig(cfg); err != nil {
		return nil, errors.Capture(err)
	}
	return cfg, nil
}
