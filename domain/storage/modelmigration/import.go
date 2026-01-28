// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"
	"slices"

	"github.com/juju/description/v11"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/domain/storage/state"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

type ImportStoragePool struct {
	UUID   domainstorage.StoragePoolUUID
	Name   string
	Origin domainstorage.StoragePoolOrigin
	Type   string
	Attrs  map[string]any
}

type ModelStoragePoolArg struct {
	StorageKind     domainstorage.StorageKind
	StoragePoolUUID domainstorage.StoragePoolUUID
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, storageRegistryGetter corestorage.ModelStorageRegistryGetter, logger logger.Logger) {
	coordinator.Add(&importOperation{
		storageRegistryGetter: storageRegistryGetter,
		logger:                logger,
	})
}

// ImportService provides a subset of the storage domain
// service methods needed for storage pool import.
type ImportService interface {
	ImportStoragePool(ctx context.Context, UUID domainstorage.StoragePoolUUID,
		name string, providerType domainstorage.ProviderType,
		originID domainstorage.StoragePoolOrigin, attrs map[string]any) error
	SetRecommendedStoragePools(ctx context.Context, pools []domainstorage.RecommendedStoragePoolParams) error
}

type importOperation struct {
	modelmigration.BaseOperation

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	service               ImportService
	logger                logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import storage"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ModelDB()), i.logger, i.storageRegistryGetter)
	return nil
}

// Execute the import on the storage pools contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	poolsToCreate := make([]ImportStoragePool, 0)
	// We first create the list of pools from the migrated models.
	// This is to ensure that the user-defined pools from the import are chosen
	// should the name conflicts with built-in pools.
	for _, v := range model.StoragePools() {
		poolsToCreate = append(poolsToCreate, ImportStoragePool{
			Name:   v.Name(),
			Origin: domainstorage.StoragePoolOriginUser,
			Type:   v.Provider(),
			Attrs:  v.Attributes(),
		})
	}

	modelStorageRegistry, err := i.storageRegistryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return errors.Errorf(
			"getting storage provider registry for model: %w", err,
		)
	}

	providerTypes, err := modelStorageRegistry.StorageProviderTypes()
	if err != nil {
		return errors.Errorf(
			"getting storage provider types for model storage registry: %w", err,
		)
	}

	for _, providerType := range providerTypes {
		registry, err := modelStorageRegistry.StorageProvider(providerType)
		if err != nil {
			return errors.Errorf(
				"getting storage provider %q from registry: %w",
				providerType, err,
			)
		}

		providerDefaultPools := registry.DefaultPools()
		for _, providerDefaultPool := range providerDefaultPools {
			providerDefault, err := i.poolSafeToCreate(ctx, poolsToCreate, providerDefaultPool)
			if err != nil {
				return err
			}
			if providerDefault != nil {
				poolsToCreate = append(poolsToCreate, *providerDefault)
			}
		}
	}

	defaultPools, recommendedPools, err := i.getRecommendedStoragePools(poolsToCreate, modelStorageRegistry)
	if err != nil {
		return errors.Errorf("getting recommended storage pools: %w", err)
	}
	poolsToCreate = append(poolsToCreate, defaultPools...)

	for _, pool := range poolsToCreate {
		err := i.service.ImportStoragePool(ctx, pool.UUID, pool.Name, domainstorage.ProviderType(pool.Type),
			pool.Origin, pool.Attrs)
		if err != nil {
			return errors.Errorf("creating storage pool %q: %w", pool.Name, err)
		}
	}

	err = i.service.SetRecommendedStoragePools(ctx, recommendedPools)
	if err != nil {
		return errors.Errorf("setting recommended storage")
	}

	return nil
}

func (i *importOperation) poolSafeToCreate(
	ctx context.Context,
	existingPools []ImportStoragePool,
	config *storage.Config) (*ImportStoragePool, error) {
	// A storage pool with a duplicate provider and name already exists.
	// We skip adding it to the slice.
	if slices.ContainsFunc(existingPools, func(pool ImportStoragePool) bool {
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
		i.logger.Warningf(
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

	// The built-in pool doesn't conflict with the user-defined pools, it's safe
	// to return it for creation.
	return &ImportStoragePool{
		UUID:   uuid,
		Name:   name,
		Origin: domainstorage.StoragePoolOriginProviderDefault,
		Type:   provider,
		Attrs:  config.Attrs(),
	}, nil
}

func (i *importOperation) getRecommendedStoragePools(existingPools []ImportStoragePool,
	reg storage.ProviderRegistry) ([]ImportStoragePool, []domainstorage.RecommendedStoragePoolParams, error) {
	poolsToCreate := make([]ImportStoragePool, 0)
	recommendedPools := make([]domainstorage.RecommendedStoragePoolParams, 0)
	appendPool := func(cfg *storage.Config) (domainstorage.StoragePoolUUID, error) {
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
		index := slices.IndexFunc(existingPools, func(e ImportStoragePool) bool {
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
		if slices.ContainsFunc(existingPools, func(pool ImportStoragePool) bool {
			return pool.Name == cfg.Name() && pool.Type == cfg.Provider().String()
		}) {
			return "", nil
		}
		poolsToCreate = append(poolsToCreate, ImportStoragePool{
			UUID:   uuid,
			Name:   cfg.Name(),
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Type:   cfg.Provider().String(),
			Attrs:  cfg.Attrs(),
		})
		return uuid, nil
	}

	// Get filesystem recommendation.
	poolCfg := reg.RecommendedPoolForKind(storage.StorageKindFilesystem)
	if poolCfg != nil {
		uuid, err := appendPool(poolCfg)
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
	poolCfg = reg.RecommendedPoolForKind(storage.StorageKindBlock)
	if poolCfg != nil {
		uuid, err := appendPool(poolCfg)
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
