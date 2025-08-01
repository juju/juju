// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/unit"
	domainstorage "github.com/juju/juju/domain/storage"
	storageservice "github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

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
}

// ApplicationService defines apis on the application service.
type ApplicationService interface {
	GetUnitMachineName(ctx context.Context, unitName unit.Name) (machine.Name, error)
}

type storageRegistryGetter func(context.Context) (storage.ProviderRegistry, error)

// StorageAPI implements the latest version (v6) of the Storage API.
type StorageAPI struct {
	blockDeviceGetter     blockDeviceGetter
	storageService        StorageService
	applicationService    ApplicationService
	storageRegistryGetter storageRegistryGetter
	authorizer            facade.Authorizer
	blockCommandService   common.BlockCommandService

	controllerUUID string
	modelUUID      coremodel.UUID
}

func NewStorageAPI(
	controllerUUID string,
	modelUUID coremodel.UUID,
	blockDeviceGetter blockDeviceGetter,
	storageService StorageService,
	applicationService ApplicationService,
	storageRegistryGetter storageRegistryGetter,
	authorizer facade.Authorizer,
	blockCommandService common.BlockCommandService,
) *StorageAPI {
	return &StorageAPI{
		controllerUUID:        controllerUUID,
		modelUUID:             modelUUID,
		blockDeviceGetter:     blockDeviceGetter,
		storageService:        storageService,
		applicationService:    applicationService,
		storageRegistryGetter: storageRegistryGetter,
		authorizer:            authorizer,
		blockCommandService:   blockCommandService,
	}
}

func (a *StorageAPI) checkCanRead(ctx context.Context) error {
	err := a.authorizer.HasPermission(ctx, permission.SuperuserAccess, names.NewControllerTag(a.controllerUUID))
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}
	return a.authorizer.HasPermission(ctx, permission.ReadAccess, names.NewModelTag(a.modelUUID.String()))
}

func (a *StorageAPI) checkCanWrite(ctx context.Context) error {
	return a.authorizer.HasPermission(ctx, permission.WriteAccess, names.NewModelTag(a.modelUUID.String()))
}

// StorageDetails retrieves and returns detailed information about desired
// storage identified by supplied tags. If specified storage cannot be
// retrieved, individual error is returned instead of storage information.
func (a *StorageAPI) StorageDetails(ctx context.Context, entities params.Entities) (params.StorageDetailsResults, error) {
	results := make([]params.StorageDetailsResult, len(entities.Entities))
	return params.StorageDetailsResults{Results: results}, nil
}

// ListStorageDetails returns storage matching a filter.
func (a *StorageAPI) ListStorageDetails(ctx context.Context, filters params.StorageFilters) (params.StorageDetailsListResults, error) {
	results := params.StorageDetailsListResults{
		Results: make([]params.StorageDetailsListResult, len(filters.Filters)),
	}
	return results, nil
}

// ListPools returns a list of pools.
// If filter is provided, returned list only contains pools that match
// the filter.
// Pools can be filtered on names and provider types.
// If both names and types are provided as filter,
// pools that match either are returned.
// This method lists union of pools and environment provider types.
// If no filter is provided, all pools are returned.
func (a *StorageAPI) ListPools(
	ctx context.Context,
	filters params.StoragePoolFilters,
) (params.StoragePoolsResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.StoragePoolsResults{}, errors.Trace(err)
	}

	results := params.StoragePoolsResults{
		Results: make([]params.StoragePoolsResult, len(filters.Filters)),
	}
	for i, filter := range filters.Filters {
		pools, err := a.listPools(ctx, filter)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = pools
	}
	return results, nil
}

func (a *StorageAPI) listPools(ctx context.Context, filter params.StoragePoolFilter) ([]params.StoragePool, error) {
	var (
		pools []domainstorage.StoragePool
		err   error
	)
	if len(filter.Names) == 0 && len(filter.Providers) == 0 {
		pools, err = a.storageService.ListStoragePools(ctx)
	} else if len(filter.Names) != 0 && len(filter.Providers) != 0 {
		pools, err = a.storageService.ListStoragePoolsByNamesAndProviders(ctx, filter.Names, filter.Providers)
	} else if len(filter.Names) != 0 {
		pools, err = a.storageService.ListStoragePoolsByNames(ctx, filter.Names)
	} else {
		pools, err = a.storageService.ListStoragePoolsByProviders(ctx, filter.Providers)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]params.StoragePool, len(pools))
	for i, p := range pools {
		pool := params.StoragePool{
			Name:     p.Name,
			Provider: p.Provider,
		}
		if len(p.Attrs) > 0 {
			pool.Attrs = make(map[string]any, len(p.Attrs))
			for k, v := range p.Attrs {
				pool.Attrs[k] = v
			}
		}
		results[i] = pool

	}
	return results, nil
}

// CreatePool creates a new pool with specified parameters.
func (a *StorageAPI) CreatePool(ctx context.Context, p params.StoragePoolArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(p.Pools)),
	}
	for i, pool := range p.Pools {
		err := a.storageService.CreateStoragePool(
			ctx,
			pool.Name,
			storage.ProviderType(pool.Provider),
			pool.Attrs)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// ListVolumes lists volumes with the given filters. Each filter produces
// an independent list of volumes, or an error if the filter is invalid
// or the volumes could not be listed.
func (a *StorageAPI) ListVolumes(ctx context.Context, filters params.VolumeFilters) (params.VolumeDetailsListResults, error) {
	results := params.VolumeDetailsListResults{
		Results: make([]params.VolumeDetailsListResult, len(filters.Filters)),
	}
	return results, nil
}

// ListFilesystems returns a list of filesystems in the environment matching
// the provided filter. Each result describes a filesystem in detail, including
// the filesystem's attachments.
func (a *StorageAPI) ListFilesystems(ctx context.Context, filters params.FilesystemFilters) (params.FilesystemDetailsListResults, error) {
	results := params.FilesystemDetailsListResults{
		Results: make([]params.FilesystemDetailsListResult, len(filters.Filters)),
	}
	return results, nil
}

// AddToUnit validates and creates additional storage instances for units.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) AddToUnit(ctx context.Context, args params.StoragesAddParams) (params.AddStorageResults, error) {
	result := make([]params.AddStorageResult, len(args.Storages))
	return params.AddStorageResults{Results: result}, nil
}

// Remove sets the specified storage entities to Dying, unless they are
// already Dying or Dead, such that the storage will eventually be removed
// from the model. If the arguments specify that the storage should be
// destroyed, then the associated cloud storage will be destroyed first;
// otherwise it will only be released from Juju's control.
func (a *StorageAPI) Remove(ctx context.Context, args params.RemoveStorage) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(args.Storage))
	return params.ErrorResults{Results: result}, nil
}

// DetachStorage sets the specified storage attachments to Dying, unless they are
// already Dying or Dead. Any associated, persistent storage will remain
// alive. This call can be forced.
func (a *StorageAPI) DetachStorage(ctx context.Context, args params.StorageDetachmentParams) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(args.StorageIds.Ids))
	return params.ErrorResults{Results: result}, nil
}

// Attach attaches existing storage instances to units.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Attach(ctx context.Context, args params.StorageAttachmentIds) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(args.Ids))
	return params.ErrorResults{Results: result}, nil
}

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Import(ctx context.Context, args params.BulkImportStorageParams) (params.ImportStorageResults, error) {
	results := make([]params.ImportStorageResult, len(args.Storage))
	return params.ImportStorageResults{Results: results}, nil
}

// RemovePool deletes the named pool
func (a *StorageAPI) RemovePool(ctx context.Context, p params.StoragePoolDeleteArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(p.Pools)),
	}
	if err := a.checkCanWrite(ctx); err != nil {
		return results, errors.Trace(err)
	}

	for i, pool := range p.Pools {
		err := a.storageService.DeleteStoragePool(ctx, pool.Name)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// UpdatePool deletes the named pool
func (a *StorageAPI) UpdatePool(ctx context.Context, p params.StoragePoolArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(p.Pools)),
	}
	if err := a.checkCanWrite(ctx); err != nil {
		return results, errors.Trace(err)
	}

	for i, pool := range p.Pools {
		err := a.storageService.ReplaceStoragePool(ctx, pool.Name, storage.ProviderType(pool.Provider), pool.Attrs)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}
