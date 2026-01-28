// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/transform"

	coreerrors "github.com/juju/juju/core/errors"
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

	SetModelStoragePools(ctx context.Context, pools []domainstorage.RecommendedStoragePoolArg) error
}

// StoragePoolService defines a service for interacting with the underlying state.
type StoragePoolService struct {
	st             StoragePoolState
	registryGetter corestorage.ModelStorageRegistryGetter
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

// ImportStoragePool creates a new storage pool with the given name and
// provider in the model. This is slightly different to [CreateStoragePool] because
// (1) the storage pool name validation uses a legacy regex and (2) we are inserting
// (a) built-in storage pools in which their UUIDs have been hardcoded and (b) user
// defined storage pools in which we have to generate their UUIDs.
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
func (s *StoragePoolService) ImportStoragePool(
	ctx context.Context,
	uuid domainstorage.StoragePoolUUID,
	name string,
	providerType domainstorage.ProviderType,
	originID domainstorage.StoragePoolOrigin,
	attrs map[string]any,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := s.validateStoragePoolCreation(ctx, name, providerType, attrs,
		domainstorage.IsValidStoragePoolNameWithLegacy)
	if err != nil {
		return err
	}

	coercedAttrs := transform.Map(
		attrs,
		func(k string, v any) (string, string) {
			return k, fmt.Sprint(v)
		},
	)

	// A user-defined pool has an empty UUID because we don't carry it over when
	// exporting the model. Therefore, when importing we must generate a UUID
	// before persisting them to the database. Built-in storage pools have their
	// UUIDs hardcoded so there is no need to generate them.
	if uuid == "" {
		uuid, err = domainstorage.NewStoragePoolUUID()
		if err != nil {
			return errors.Errorf(
				"creating new storage pool %q uuid: %w", name, err,
			)
		}
	}

	arg := domainstorageinternal.CreateStoragePool{
		Attrs:        coercedAttrs,
		Name:         name,
		Origin:       originID,
		ProviderType: providerType,
		UUID:         uuid,
	}

	return s.st.CreateStoragePool(ctx, arg)
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
