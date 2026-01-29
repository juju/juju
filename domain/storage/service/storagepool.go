// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v11"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// StoragePoolState defines an interface for interacting with the underlying
// state of storage pools in the model.
type StoragePoolState interface {
	// CreateStoragePool creates a new storage pool in the model with the
	// specified args and uuid value.
	//
	// The following errors can be expected:
	// - [storageerrors.PoolAlreadyExists] if a pool with the same name or
	// uuid already exist in the model.
	CreateStoragePool(context.Context, domainstorageinternal.CreateStoragePool) error

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

	// ListStoragePoolsByNamesAndProviders returns the storage pools matching the specified
	// names and or providers, including the default storage pools.
	// If no storage pools match the criteria, an empty slice is returned without an error.
	ListStoragePoolsByNamesAndProviders(
		ctx context.Context, names, providers []string,
	) ([]domainstorage.StoragePool, error)

	// ListStoragePoolsByNames returns the storage pools matching the specified names, including
	// the default storage pools.
	// If no names are specified, an empty slice is returned without an error.
	// If no storage pools match the criteria, an empty slice is returned without an error.
	ListStoragePoolsByNames(
		ctx context.Context, names []string,
	) ([]domainstorage.StoragePool, error)

	// ListStoragePoolsByProviders returns the storage pools matching the specified
	// providers, including the default storage pools.
	// If no providers are specified, an empty slice is returned without an error.
	// If no storage pools match the criteria, an empty slice is returned without an error.
	ListStoragePoolsByProviders(
		ctx context.Context, providers []string,
	) ([]domainstorage.StoragePool, error)

	// GetStoragePoolUUID returns the UUID of the storage pool for the specified name.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	GetStoragePoolUUID(ctx context.Context, name string) (domainstorage.StoragePoolUUID, error)

	// GetStoragePool returns the storage pool for the specified UUID.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified UUID does not exist.
	GetStoragePool(ctx context.Context, poolUUID domainstorage.StoragePoolUUID) (domainstorage.StoragePool, error)

	// SetModelStoragePools replaces the model's recommended storage pools with the
	// supplied set. All existing model storage pool mappings are removed before the
	// new ones are inserted.
	//
	// If any referenced storage pool UUID does not exist in the model, this
	// returns [domainstorageerrors.StoragePoolNotFound]. Supplying an empty slice
	// results in a no-op.
	SetModelStoragePools(ctx context.Context, pools []domainstorage.RecommendedStoragePoolArg) error
}

// StoragePoolService defines a service for interacting with the underlying state.
type StoragePoolService struct {
	st             StoragePoolState
	registryGetter corestorage.ModelStorageRegistryGetter
	logger         logger.Logger
}

// CreateStoragePool creates a new storage pool with the given name and
// provider in the model. Returned is the unique uuid for the new storage
// pool.
//
// The following errors may be returned:
// - [domainstorageerrors.StoragePoolNameInvalid] when the supplied storage
// pool name is considered invalid or empty.
// - [domainstorageerrors.ProviderTypeInvalid] when the supplied provider
// type value is invalid for further use.
// - [domainstorageerrors.ProviderTypeNotFound] when the supplied provider
// type is not known to the controller.
// - [domainstorageerrors.StoragePoolAlreadyExists] when a storage pool for the
// supplied name already exists in the model.
// - [domainstorageerrors.StoragePoolAttributeInvalid] when one of the supplied
// storage pool attributes is invalid.
func (s *StoragePoolService) CreateStoragePool(
	ctx context.Context,
	name string,
	providerType domainstorage.ProviderType,
	attrs map[string]any,
) (domainstorage.StoragePoolUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !domainstorage.IsValidStoragePoolName(name) {
		// We don't include the invalid storage pool name on purpose. It's
		// contents are unknown and so we would shouldn't process the occupied
		// memory any further.
		return "", errors.New("new storage pool name is not valid").Add(
			domainstorageerrors.StoragePoolNameInvalid,
		)
	}

	err := s.validateStoragePoolCreation(ctx, name, providerType, attrs,
		domainstorage.IsValidStoragePoolName)
	if err != nil {
		return "", err
	}

	// TODO(tlm): Storage providers have default configurations for storage
	// pools when the user may have not supplied a config key. We want to
	// capture these defaults and persist them so storage pools are
	// reproducible.

	coercedAttrs := transform.Map(
		attrs,
		func(k string, v any) (string, string) {
			return k, fmt.Sprint(v)
		},
	)

	uuid, err := domainstorage.NewStoragePoolUUID()
	if err != nil {
		return "", errors.Errorf(
			"creating new storage pool %q uuid: %w", name, err,
		)
	}
	arg := domainstorageinternal.CreateStoragePool{
		Attrs:        coercedAttrs,
		Name:         name,
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: providerType,
		UUID:         uuid,
	}

	if err := s.st.CreateStoragePool(ctx, arg); err != nil {
		return "", err
	}
	return uuid, nil
}

// ImportStoragePools creates new storage pools with the slice of [domainstorage.ImportStoragePoolParams]
// . This is slightly different to [CreateStoragePools] because:
//  1. the storage pool name validation uses a legacy regex and,
//  2. the storage pools could be user-defined and provider default.
//
// The following errors may be returned:
// - [domainstorageerrors.StoragePoolNameInvalid] when the supplied storage
// pool name is considered invalid or empty.
// - [domainstorageerrors.ProviderTypeInvalid] when the supplied provider
// type value is invalid for further use.
// - [domainstorageerrors.ProviderTypeNotFound] when the supplied provider
// type is not known to the controller.
// - [domainstorageerrors.StoragePoolAlreadyExists] when a storage pool for the
// supplied name already exists in the model.
// - [domainstorageerrors.StoragePoolAttributeInvalid] when one of the supplied
// storage pool attributes is invalid.
func (s *StoragePoolService) ImportStoragePools(
	ctx context.Context,
	pools []domainstorage.ImportStoragePoolParams,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	for _, pool := range pools {
		err := s.validateStoragePoolCreation(ctx, pool.Name, domainstorage.ProviderType(pool.Type),
			pool.Attrs,
			domainstorage.IsValidStoragePoolNameWithLegacy)
		if err != nil {
			return err
		}

		coercedAttrs := transform.Map(
			pool.Attrs,
			func(k string, v any) (string, string) {
				return k, fmt.Sprint(v)
			},
		)

		arg := domainstorageinternal.CreateStoragePool{
			Attrs:        coercedAttrs,
			Name:         pool.Name,
			Origin:       pool.Origin,
			ProviderType: domainstorage.ProviderType(pool.Type),
			UUID:         pool.UUID,
		}
		err = s.st.CreateStoragePool(ctx, arg)
		if err != nil {
			return errors.Errorf("creating storage pool %q: %w", pool.Name, err)
		}
	}

	return nil
}

func (s *StoragePoolService) validateStoragePoolCreation(
	ctx context.Context,
	name string,
	providerType domainstorage.ProviderType,
	attrs map[string]any,
	isValidStoragePoolName func(string) bool,
) error {
	if !isValidStoragePoolName(name) {
		return errors.New("new storage pool name is not valid").Add(
			domainstorageerrors.StoragePoolNameInvalid,
		)
	}
	if !providerType.IsValid() {
		// We don't include the invalid storage provider type on purpose. It's
		// contents are unknown and so we would shouldn't process the occupied
		// memory any further.
		return errors.New("storage provider type is not valid").Add(
			domainstorageerrors.ProviderTypeInvalid,
		)
	}

	providerRegistry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return errors.Errorf("getting storage provider registry: %w", err)
	}

	storageProvider, err := providerRegistry.StorageProvider(
		internalstorage.ProviderType(providerType),
	)
	if errors.Is(err, coreerrors.NotFound) {
		return errors.Errorf(
			"storage provider %q does not exist in the model",
			providerType.String(),
		).Add(domainstorageerrors.ProviderTypeNotFound)
	} else if err != nil {
		return errors.Errorf(
			"getting storage provider %q: %w", providerType.String(), err,
		)
	}

	// NOTE (tlm): We need to create better long term support with storage
	// providers around their error contract for validation. They should return
	// a better typed error describing the attribute key that failed and the
	// reasons for this. Having this will allow this service func to give better
	// context to the caller and in returned the user.
	return validateNewStoragePoolConfig(
		ctx, storageProvider, name, providerType, attrs,
	)
}

// validateNewStoragePoolConfig is responsible for taking a proposed new storage
// pool configuration and validating the configuration with the storage
// provider.
//
// Any errors encountered with the storage provider during validation are
// returned. There is no defined storage error contracts that exist with the
// providers at present.
func validateNewStoragePoolConfig(
	ctx context.Context,
	storageProvider internalstorage.Provider,
	name string,
	providerType domainstorage.ProviderType,
	attrs map[string]any,
) error {
	cfg, err := internalstorage.NewConfig(name, internalstorage.ProviderType(providerType), attrs)
	if err != nil {
		return err
	}

	if err = storageProvider.ValidateConfig(cfg); err != nil {
		return errors.Errorf(
			"validating new storage pool configuration with provider %q: %w",
			providerType, err,
		)
	}

	return nil
}

func (s *StoragePoolService) validateConfig(ctx context.Context, name string, providerType internalstorage.ProviderType, attrs map[string]any) error {
	cfg, err := internalstorage.NewConfig(name, providerType, attrs)
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
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified name does not exist.
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
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified name does not exist.
// - [domainstorageerrors.StoragePoolNameInvalid] if the pool name is not valid.
func (s *StoragePoolService) ReplaceStoragePool(
	ctx context.Context,
	name string,
	providerType internalstorage.ProviderType,
	attrs map[string]any,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !domainstorage.IsValidStoragePoolName(name) {
		return errors.Errorf("pool name %q not valid", name).Add(domainstorageerrors.StoragePoolNameInvalid)
	}

	poolUUID, err := s.st.GetStoragePoolUUID(ctx, name)
	if err != nil {
		return errors.Errorf("getting storage pool %q UUID: %w", name, err)
	}

	// Use the existing provider type unless explicitly overwritten.
	if providerType == "" {
		existingConfig, err := s.st.GetStoragePool(ctx, poolUUID)
		if err != nil {
			return errors.Capture(err)
		}
		providerType = internalstorage.ProviderType(existingConfig.Provider)
	}

	if err := s.validateConfig(ctx, name, providerType, attrs); err != nil {
		return errors.Capture(err)
	}

	attrsToSave := transform.Map(attrs, func(k string, v any) (string, string) { return k, fmt.Sprint(v) })
	sp := domainstorage.StoragePool{
		UUID:     poolUUID.String(),
		Name:     name,
		Provider: string(providerType),
		Attrs:    attrsToSave,
	}

	if err := s.st.ReplaceStoragePool(ctx, sp); err != nil {
		return errors.Errorf("replacing storage pool %q: %w", name, err)
	}
	return nil
}

// ListStoragePools returns all of the storage pools available in the model.
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
// names and providers, including the default storage pools.
// If no names and providers are specified, an empty slice is returned without an error.
// If no storage pools match the criteria, an empty slice is returned without an error.
func (s *StoragePoolService) ListStoragePoolsByNamesAndProviders(
	ctx context.Context,
	names domainstorage.Names,
	providers domainstorage.Providers,
) ([]domainstorage.StoragePool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(names) == 0 || len(providers) == 0 {
		return nil, nil
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

	if len(names) == 0 {
		return nil, nil
	}

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

	if len(providers) == 0 {
		return nil, nil
	}

	if err := s.validateProviderCriteria(ctx, providers); err != nil {
		return nil, errors.Capture(err)
	}

	pools, err := s.st.ListStoragePoolsByProviders(ctx, providers)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return pools, nil
}

// GetStoragePoolUUID returns the UUID of the storage pool for the specified name.
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified name does not exist.
// - [domainstorageerrors.StoragePoolNameInvalid] if the pool name is not valid.
func (s *StoragePoolService) GetStoragePoolUUID(ctx context.Context, name string) (domainstorage.StoragePoolUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !domainstorage.IsValidStoragePoolName(name) {
		return "", errors.Errorf(
			"pool name %q not valid", name,
		).Add(domainstorageerrors.StoragePoolNameInvalid)
	}

	poolUUID, err := s.st.GetStoragePoolUUID(ctx, name)
	if err != nil {
		return "", errors.Capture(err)
	}
	return poolUUID, nil
}

// GetStoragePoolByName returns the storage pool with the specified name.
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified name does not exist.
func (s *StoragePoolService) GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !domainstorage.IsValidStoragePoolName(name) {
		return domainstorage.StoragePool{}, errors.Errorf(
			"pool name %q not valid", name,
		).Add(domainstorageerrors.StoragePoolNameInvalid)
	}
	poolUUID, err := s.st.GetStoragePoolUUID(ctx, name)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Errorf(
			"getting storage pool %q UUID: %w", name, err,
		)
	}

	pool, err := s.st.GetStoragePool(ctx, poolUUID)
	if err != nil {
		return domainstorage.StoragePool{}, errors.Capture(err)
	}
	return pool, nil
}

func (s *StoragePoolService) validatePoolListFilterTerms(ctx context.Context, names domainstorage.Names, providers domainstorage.Providers) error {
	// Validating names MUST happen before provider type validation. This is
	// because provider validation calls off to dependencyies and we should
	// avoid expensive calls until as much of the basic criteria can be
	// validated first.
	if err := s.validateNameCriteria(names); err != nil {
		return errors.Capture(err)
	}

	if err := s.validateProviderCriteria(ctx, providers); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (s *StoragePoolService) validateNameCriteria(names []string) error {
	for _, n := range names {
		if !domainstorage.IsValidStoragePoolName(n) {
			return errors.Errorf("pool name %q not valid", n).Add(domainstorageerrors.StoragePoolNameInvalid)
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
		_, err := registry.StorageProvider(internalstorage.ProviderType(p))
		if err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

func (s *StoragePoolService) SetRecommendedStoragePools(ctx context.Context,
	pools []domainstorage.RecommendedStoragePoolParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	poolArgs := make([]domainstorage.RecommendedStoragePoolArg, len(pools))
	for i, pool := range pools {
		poolArgs[i] = domainstorage.RecommendedStoragePoolArg{
			StoragePoolUUID: pool.StoragePoolUUID,
			StorageKind:     pool.StorageKind,
		}
	}

	return s.st.SetModelStoragePools(ctx, poolArgs)
}

func (s *StoragePoolService) GetStoragePoolsToImport(
	ctx context.Context,
	userPools []description.StoragePool,
) (
	[]domainstorage.ImportStoragePoolParams,
	[]domainstorage.RecommendedStoragePoolParams,
	error,
) {
	poolsToCreate := make([]domainstorage.ImportStoragePoolParams, 0)
	// We first create the list of pools from the migrated models.
	// This is to ensure that the user-defined pools from the import are chosen
	// should the name conflicts with provider default pools.
	for _, v := range userPools {
		poolsToCreate = append(poolsToCreate, domainstorage.ImportStoragePoolParams{
			Name:   v.Name(),
			Origin: domainstorage.StoragePoolOriginUser,
			Type:   v.Provider(),
			Attrs:  v.Attributes(),
		})
	}

	modelStorageRegistry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, nil, errors.Errorf(
			"getting storage provider registry for model: %w", err,
		)
	}

	providerTypes, err := modelStorageRegistry.StorageProviderTypes()
	if err != nil {
		return nil, nil, errors.Errorf(
			"getting storage provider types for model storage registry: %w", err,
		)
	}

	for _, providerType := range providerTypes {
		registry, err := modelStorageRegistry.StorageProvider(providerType)
		if err != nil {
			return nil, nil, errors.Errorf(
				"getting storage provider %q from registry: %w",
				providerType, err,
			)
		}

		providerDefaultPools := registry.DefaultPools()
		for _, providerDefaultPool := range providerDefaultPools {
			providerDefault, err := s.defaultPoolForImport(ctx, poolsToCreate, providerDefaultPool)
			if err != nil {
				return nil, nil, err
			}
			if providerDefault != nil {
				poolsToCreate = append(poolsToCreate, *providerDefault)
			}
		}
	}

	defaultPools, recommendedPools, err := s.getRecommendedStoragePools(poolsToCreate,
		modelStorageRegistry)
	if err != nil {
		return nil, nil, errors.Errorf("getting recommended storage pools: %w", err)
	}
	poolsToCreate = append(poolsToCreate, defaultPools...)
	return poolsToCreate, recommendedPools, nil
}

func (s *StoragePoolService) defaultPoolForImport(
	ctx context.Context,
	existingPools []domainstorage.ImportStoragePoolParams,
	config *internalstorage.Config) (*domainstorage.ImportStoragePoolParams, error) {
	// A storage pool with a duplicate provider and name already exists.
	// We don't want to choose this default pool to avoid conflicting with
	// the existing one.
	if slices.ContainsFunc(existingPools, func(pool domainstorage.ImportStoragePoolParams) bool {
		return pool.Name == config.Name() && pool.Type == config.Provider().String()
	}) {
		return nil, nil
	}
	name := config.Name()
	provider := config.Provider().String()
	uuid, err := domainstorage.GetProviderDefaultStoragePoolUUID(
		name, provider)

	// Logic carried over from [SeedDefaultStoragePools] func
	// in [github.com/juju/juju/domain/model/service.ProviderModelService].
	if errors.Is(err, coreerrors.NotFound) {
		// This happens when the default pool is not supported yet by the
		// storage domain. This shouldn't stop the model from being created.
		// Instead we log the problem.
		s.logger.Warningf(
			ctx,
			"storage provider %q default pool %q is not recognised, adding to model with generated uuid.",
			provider,
			name,
		)
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf(
			"getting storage pool uuid for default provider %q pool %q",
			provider,
			name,
		)
	}

	// The provider default pool doesn't conflict with the user-defined pools, it's safe
	// to return it for creation.
	return &domainstorage.ImportStoragePoolParams{
		UUID:   uuid,
		Name:   name,
		Origin: domainstorage.StoragePoolOriginProviderDefault,
		Type:   provider,
		Attrs:  config.Attrs(),
	}, nil
}

// getRecommendedStoragePools determines the recommended storage pools
// for each supported storage kind and resolves which of them need to be
// created during import.
//
// For each recommended pool provided by the registry, the function:
//   - Resolves the pool's UUID using provider defaults
//   - Checks for duplicates against existingPools using UUID, and then
//     pool name and provider type
//   - Appends a pool to the creation list only if it does not already exist
//     and does not conflict with a user-defined pool
//
// The returned values are:
//  1. A slice of [ImportStoragePoolParams] describing provider default pools that
//     should be created during import
//  2. A slice of [RecommendedStoragePoolParams] mapping storage kinds to the
//     resolved storage pool UUIDs, which may refer to pools of the provider.
func (s *StoragePoolService) getRecommendedStoragePools(
	existingPools []domainstorage.ImportStoragePoolParams,
	reg internalstorage.ProviderRegistry,
) (
	[]domainstorage.ImportStoragePoolParams,
	[]domainstorage.RecommendedStoragePoolParams,
	error,
) {
	poolsToCreate := make([]domainstorage.ImportStoragePoolParams, 0)
	recommendedPools := make([]domainstorage.RecommendedStoragePoolParams, 0)

	// ensureStoragePool ensures that a provider-recommended storage pool is
	// accounted for during import.
	//
	// It checks the given configuration against existingPools to avoid duplicates.
	// If an identical pool already exists (by UUID), its UUID is returned.
	// If a user-defined pool with the same name and provider exists, no pool is
	// added and an empty UUID is returned.
	//
	// Otherwise, the pool is appended to [poolsToCreate] and its generated UUID
	// is returned.
	ensurePool := func(cfg *internalstorage.Config) (domainstorage.StoragePoolUUID, error) {
		// Get the UUID of the given pool so that we can later
		// check for duplication within [existingPools].
		uuid, err := domainstorage.GenerateProviderDefaultStoragePoolUUIDWithDefaults(
			cfg.Name(),
			cfg.Provider().String(),
		)
		if err != nil {
			return "", errors.Capture(err)
		}

		// Duplication checking is performed on uuid and then name and provider
		// type.
		index := slices.IndexFunc(existingPools, func(e domainstorage.ImportStoragePoolParams) bool {
			return e.UUID == uuid
		},
		)
		// The given pool exists in [existingPools]. We don't want to add a duplicate
		// so return early.
		if index != -1 {
			return (existingPools)[index].UUID, nil
		}
		// We don't want to add it for creation if there exists an existing pool
		// with duplicate name and provider. This may have been a user-defined
		// pool from the source controller.
		// We have no way of guaranteeing that a "foo" user-defined pool is the same
		// "foo" provider default pool.
		if slices.ContainsFunc(existingPools, func(pool domainstorage.ImportStoragePoolParams) bool {
			return pool.Name == cfg.Name() &&
				pool.Type == cfg.Provider().String() &&
				pool.Origin == domainstorage.StoragePoolOriginUser
		}) {
			return "", nil
		}
		poolsToCreate = append(poolsToCreate, domainstorage.ImportStoragePoolParams{
			UUID:   uuid,
			Name:   cfg.Name(),
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Type:   cfg.Provider().String(),
			Attrs:  cfg.Attrs(),
		})
		return uuid, nil
	}

	// Get filesystem recommendation.
	poolCfg := reg.RecommendedPoolForKind(internalstorage.StorageKindFilesystem)
	if poolCfg != nil {
		uuid, err := ensurePool(poolCfg)
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
		if uuid != "" {
			recommendedPools = append(recommendedPools, domainstorage.RecommendedStoragePoolParams{
				StorageKind:     domainstorage.StorageKindFilesystem,
				StoragePoolUUID: uuid,
			})
		}
	}

	// Get block device recommendation.
	poolCfg = reg.RecommendedPoolForKind(internalstorage.StorageKindBlock)
	if poolCfg != nil {
		uuid, err := ensurePool(poolCfg)
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
		if uuid != "" {
			recommendedPools = append(recommendedPools, domainstorage.RecommendedStoragePoolParams{
				StorageKind:     domainstorage.StorageKindBlock,
				StoragePoolUUID: uuid,
			})
		}
	}

	return poolsToCreate, recommendedPools, nil
}
