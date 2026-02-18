// Copyright 2026 Canonical Ltd.
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
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storage/internal"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// StorageImportState defines an interface for interacting with the underlying
// state for storage import operations.
type StorageImportState interface {
	// ImportStorageInstances imports storage instances and storage unit
	// unit owners if the unit name is provided.
	ImportStorageInstances(ctx context.Context, args []internal.ImportStorageInstanceArgs) error

	// ImportFilesystems imports filesystems from the provided parameters.
	ImportFilesystems(ctx context.Context, args []internal.ImportFilesystemArgs) error

	// GetStoragePoolProvidersByNames returns a map of storage pool names to their
	// provider types for the specified storage pool names.
	GetStoragePoolProvidersByNames(ctx context.Context, names []string) (map[string]string, error)

	// GetStorageInstanceUUIDsByIDs retrieves the UUIDs of storage instances by
	// their IDs.
	GetStorageInstanceUUIDsByIDs(ctx context.Context, storageIDs []string) (map[string]string, error)

	// CreateStoragePool creates a new storage pool in the model with the
	// specified args and uuid value.
	//
	// The following errors can be expected:
	// - [storageerrors.PoolAlreadyExists] if a pool with the same name or
	// uuid already exist in the model.
	CreateStoragePool(context.Context, domainstorageinternal.CreateStoragePool) error

	// SetModelStoragePools replaces the model's recommended storage pools with the
	// supplied set. All existing model storage pool mappings are removed before the
	// new ones are inserted.
	//
	// If any referenced storage pool UUID does not exist in the model, this
	// returns [domainstorageerrors.StoragePoolNotFound]. Supplying an empty slice
	// results in a no-op.
	SetModelStoragePools(ctx context.Context, pools []domainstorage.RecommendedStoragePoolArg) error
}

// StorageImportService defines a service for importing storage entities during
// model import.
type StorageImportService struct {
	st             StorageImportState
	registryGetter corestorage.ModelStorageRegistryGetter
	logger         logger.Logger
}

// ImportStorageInstances imports storage instances and storage unit
// owners. Storage unit owners are created if the unit name is provided.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when any of the params did not pass validation.
func (s *StorageImportService) ImportStorageInstances(ctx context.Context, params []domainstorage.ImportStorageInstanceParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(params) == 0 {
		return nil
	}

	for i, param := range params {
		if err := param.Validate(); err != nil {
			return errors.Errorf("validating import storage instance params %d: %w", i, err)
		}
	}

	args, err := transform.SliceOrErr(params, func(in domainstorage.ImportStorageInstanceParams) (internal.ImportStorageInstanceArgs, error) {
		storageUUID, err := domainstorage.NewStorageInstanceUUID()
		if err != nil {
			return internal.ImportStorageInstanceArgs{}, err
		}
		return internal.ImportStorageInstanceArgs{
			UUID: storageUUID.String(),
			// 3.6 does not pass life of a storage instance during
			// import. Assume alive. domainlife.Life has a test which
			// validates the data against the db.
			Life:             int(life.Alive),
			PoolName:         in.PoolName,
			RequestedSizeMiB: in.RequestedSizeMiB,
			StorageID:        in.StorageID,
			StorageName:      in.StorageName,
			StorageKind:      in.StorageKind,
			UnitName:         in.UnitName,
		}, nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.ImportStorageInstances(ctx, args)
}

// ImportFilesystems imports filesystems from the provided parameters.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when any of the params did not pass validation.
// - [domainstorageerrors.StoragePoolNotFound] when any of the specified
// storage pools do not exist.
// - [domainstorageerrors.ProviderTypeNotFound] when the provider type for any
// of the specified storage pools cannot be found in the storage registry.
// - [domainstorageerrors.StorageInstanceNotFound] when any of the
// provided IDs do not have a corresponding storage instance.
func (s *StorageImportService) ImportFilesystems(ctx context.Context, params []domainstorage.ImportFilesystemParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(params) == 0 {
		return nil
	}

	for i, arg := range params {
		if err := arg.Validate(); err != nil {
			return errors.Errorf("validating import filesystem params %d: %w", i, err)
		}
	}

	poolNames := transform.Slice(params, func(arg domainstorage.ImportFilesystemParams) string { return arg.PoolName })
	poolScopes, err := s.retrieveFilesystemProviderScopesForPools(ctx, poolNames)
	if err != nil {
		return errors.Errorf("getting provider scopes of filesystems: %w", err)
	}

	// storage instance ID can be empty, which indicates the filesystem is not
	// associated with any storage instance.
	storageInstanceIDs := make([]string, 0, len(params))
	for _, arg := range params {
		if arg.StorageInstanceID != "" {
			storageInstanceIDs = append(storageInstanceIDs, arg.StorageInstanceID)
		}
	}
	storageInstanceUUIDsByID, err := s.st.GetStorageInstanceUUIDsByIDs(ctx, storageInstanceIDs)
	if err != nil {
		return errors.Errorf("retrieving storage instance UUIDs by IDs: %w", err)
	}

	fullArgs := make([]internal.ImportFilesystemArgs, len(params))
	for i, arg := range params {
		providerScope, ok := poolScopes[arg.PoolName]
		if !ok {
			// This indicates a programming error. We should fail in the state
			// if a pool name is not found.
			return errors.Errorf("storage pool %q not found for filesystem %q", arg.PoolName, arg.ID).
				Add(domainstorageerrors.StoragePoolNotFound)
		}

		var storageInstanceUUID string
		if arg.StorageInstanceID != "" {
			var ok bool
			storageInstanceUUID, ok = storageInstanceUUIDsByID[arg.StorageInstanceID]
			if !ok {
				return errors.Errorf("storage instance with ID %q not found for filesystem %q", arg.StorageInstanceID, arg.ID).
					Add(domainstorageerrors.StorageInstanceNotFound)
			}
		}

		uuid, err := domainstorage.NewFilesystemUUID()
		if err != nil {
			return errors.Errorf("generating UUID for filesystem %q: %w", arg.ID, err)
		}

		fullArgs[i] = internal.ImportFilesystemArgs{
			UUID:                uuid.String(),
			ID:                  arg.ID,
			Life:                life.Alive,
			SizeInMiB:           arg.SizeInMiB,
			ProviderID:          arg.ProviderID,
			StorageInstanceUUID: storageInstanceUUID,
			Scope:               providerScope,
		}
	}

	return s.st.ImportFilesystems(ctx, fullArgs)
}

func (s *StorageImportService) retrieveFilesystemProviderScopesForPools(
	ctx context.Context, poolNames []string,
) (map[string]domainstorageprovisioning.ProvisionScope, error) {
	providerScopes := make(map[string]domainstorageprovisioning.ProvisionScope)

	providerMap, err := s.st.GetStoragePoolProvidersByNames(ctx, poolNames)
	if err != nil {
		return nil, errors.Errorf("getting storage pool providers by names: %w", err)
	}

	registry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, errors.Errorf("getting storage registry: %w", err)
	}

	for poolName, providerType := range providerMap {
		storageProvider, err := registry.StorageProvider(
			internalstorage.ProviderType(providerType))
		if errors.Is(err, coreerrors.NotFound) {
			return nil, errors.Errorf(
				"storage provider type %q not found for pool %q",
				providerType, poolName,
			).Add(domainstorageerrors.ProviderTypeNotFound)
		} else if err != nil {
			return nil, errors.Errorf("getting storage provider %q for storage pool %q: %w",
				providerType, poolName, err)
		}

		ic, err := domainstorageprovisioning.CalculateStorageInstanceComposition(
			domainstorage.StorageKindFilesystem, storageProvider)
		if err != nil {
			return nil, errors.Errorf(
				"calculating storage instance composition for pool %q: %w",
				poolName, err,
			)
		}

		providerScopes[poolName] = ic.FilesystemProvisionScope
	}

	return providerScopes, nil
}

// GetStoragePoolsToImport resolves the full set of storage pools to create during
// model import.
//
// It starts with user-defined storage pools from the description model, ensuring
// they take precedence over provider default pools on name and provider conflicts.
// Provider default pools are then added where safe, followed by resolving any
// recommended storage pools from the registry.
//
// The function returns:
//  1. A slice of storage pools that should be created during import
//  2. A slice of recommended storage pools referencing existing or newly created pools
func (s *StorageImportService) GetStoragePoolsToImport(
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
		uuid, err := domainstorage.NewStoragePoolUUID()
		if err != nil {
			return nil, nil, errors.Errorf("generating uuid for user pool %q: %w", v.Name(), err)
		}
		poolsToCreate = append(poolsToCreate, domainstorage.ImportStoragePoolParams{
			UUID:   uuid,
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

// SetRecommendedStoragePools persists the set of recommended storage pools
// that are to be used for a model.
func (s *StorageImportService) SetRecommendedStoragePools(ctx context.Context,
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

func (s *StorageImportService) defaultPoolForImport(
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
		return nil, errors.Errorf(
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
func (s *StorageImportService) getRecommendedStoragePools(
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

		// Check if the UUID matches an existing pool that is NOT a user-defined pool.
		// This means that we can recommend a provider default pool for the model.
		index := slices.IndexFunc(existingPools, func(e domainstorage.ImportStoragePoolParams) bool {
			return e.UUID == uuid
		},
		)
		// The given pool exists in [existingPools]. We don't want to add a duplicate
		// so return early.
		if index != -1 &&
			existingPools[index].Origin == domainstorage.StoragePoolOriginProviderDefault {
			return (existingPools)[index].UUID, nil
		} else if index != -1 &&
			existingPools[index].Origin == domainstorage.StoragePoolOriginUser {
			// The chances of a recommended provider default pool UUID matching a user-defined
			// pool UUID is slim to none. But we add it here for defensive programming.
			return "", nil
		}

		// We don't want to add a user-defined pool for recommendation and/or creation.
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
func (s *StorageImportService) ImportStoragePools(
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
		err = pool.UUID.Validate()
		if err != nil {
			return errors.Errorf("storage pool %q UUID is not valid: %w", pool.Name, err)
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
		// TODO(adisazhar123): refactor opportunity. Bulk insert.
		err = s.st.CreateStoragePool(ctx, arg)
		if err != nil {
			return errors.Errorf("creating storage pool %q: %w", pool.Name, err)
		}
	}

	return nil
}

func (s *StorageImportService) validateStoragePoolCreation(
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
