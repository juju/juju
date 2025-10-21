// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"time"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/unit"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainremoval "github.com/juju/juju/domain/removal"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storageservice "github.com/juju/juju/domain/storage/service"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

type BlockDeviceService interface {
	GetBlockDevicesForMachine(
		ctx context.Context, machineUUID machine.UUID,
	) ([]blockdevice.BlockDevice, error)
}

// RemovalService defines the interface required for removing storage related
// entities in the model on behalf of an API caller.
type RemovalService interface {
	// RemoveStorageAttachmentsFromAliveUnit is reponsible for removing one or
	// more storage attachments from a unit that is still alive in the model.
	// This operation can be considered a detatch of a storage instance from a
	// unit.
	//
	// The following errors may be returned:
	// - [storageerrors.StorageAttachmentNotFound] if the supplied storage
	// attachment uuid does not exist in the model.
	// - [applicationerrors.UnitNotAlive] if the unit the storage attachment is
	// conencted to is not alive.
	// [applicationerrors.UnitStorageMinViolation] if removing a storage
	// attachment would violate the charm minimums required for the unit.
	RemoveStorageAttachmentFromAliveUnit(
		context.Context,
		domainstorageprovisioning.StorageAttachmentUUID,
		bool,
		time.Duration,
	) (domainremoval.UUID, error)
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

	// GetStorageAttachmentUUIDForStorageInstanceAndUnit returns the
	// [domainstorageprovisioning.StorageAttachmentUUID] associated with the
	// given storage instance id and unit name.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] if the
	// storage instance for the supplied uuid no longer exists.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] if the
	// unit no longer exists for the supplied uuid.
	GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		ctx context.Context,
		uuid domainstorage.StorageInstanceUUID,
		unitUUID coreunit.UUID,
	) (domainstorageprovisioning.StorageAttachmentUUID, error)

	// GetStorageInstanceUUIDForID returns the StorageInstanceUUID for the given
	// storage ID.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] if no
	// storage instance exists for the provided storage id.
	GetStorageInstanceUUIDForID(
		ctx context.Context,
		storageID string,
	) (domainstorage.StorageInstanceUUID, error)

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

	// GetUnitUUID returns the UUID for the named unit.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/unit.InvalidUnitName] if the unit name is
	// invalid.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] if the
	// unit doesn't exist.
	GetUnitUUID(context.Context, coreunit.Name) (coreunit.UUID, error)
}

// StorageAPIv6 provides the Storage API facade for version 6.
type StorageAPIv6 struct {
	*StorageAPI
}

// StorageAPI implements the latest version (v7) of the Storage API.
type StorageAPI struct {
	applicationService  ApplicationService
	blockCommandService common.BlockCommandService
	blockDeviceService  BlockDeviceService
	removalService      RemovalService
	storageService      StorageService

	authorizer     facade.Authorizer
	controllerUUID string
	modelUUID      coremodel.UUID
}

func NewStorageAPI(
	controllerUUID string,
	modelUUID coremodel.UUID,
	authorizer facade.Authorizer,
	applicationService ApplicationService,
	blockCommandService common.BlockCommandService,
	blockDeviceService BlockDeviceService,
	removalService RemovalService,
	storageService StorageService,
) *StorageAPI {
	return &StorageAPI{
		applicationService:  applicationService,
		blockCommandService: blockCommandService,
		blockDeviceService:  blockDeviceService,
		removalService:      removalService,
		storageService:      storageService,

		authorizer:     authorizer,
		controllerUUID: controllerUUID,
		modelUUID:      modelUUID,
	}
}

func (a *StorageAPI) checkCanRead(ctx context.Context) error {
	err := a.authorizer.HasPermission(ctx, permission.SuperuserAccess, names.NewControllerTag(a.controllerUUID))
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Capture(err)
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
		return params.StoragePoolsResults{}, errors.Capture(err)
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
		return nil, errors.Capture(err)
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

// DetachStorage sets the specified storage attachment(s) to Dying, unless they
// are already Dying or Dead. Any associated, persistent storage will remain
// alive. This call can be forced to only remove the attachment. Force will not
// bypass business logic or safety checks.
func (a *StorageAPI) DetachStorage(
	ctx context.Context,
	args params.StorageDetachmentParams,
) (params.ErrorResults, error) {
	var (
		force    bool
		waitTime time.Duration
	)
	if args.MaxWait != nil && *args.MaxWait < 0 {
		err := errors.Errorf(
			"max wait time cannot be a negative number",
		).Add(coreerrors.NotValid)
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	} else if args.MaxWait != nil {
		waitTime = *args.MaxWait
	}

	if args.Force != nil {
		force = *args.Force
	}

	processStorageAttachmentID := func(
		ctx context.Context,
		id params.StorageAttachmentId,
	) error {
		storageTag, err := names.ParseStorageTag(id.StorageTag)
		if err != nil {
			return errors.Errorf(
				"invalid storage tag %q", id.StorageTag,
			).Add(coreerrors.NotValid)
		}

		unitTag, err := names.ParseUnitTag(id.UnitTag)
		if err != nil {
			return errors.Errorf(
				"invalid unit tag %q", id.UnitTag,
			).Add(coreerrors.NotValid)
		}

		return a.detachStorageAttachment(
			ctx,
			storageTag.Id(),
			coreunit.Name(unitTag.Id()),
			force,
			waitTime,
		)
	}

	result := make([]params.ErrorResult, 0, len(args.StorageIds.Ids))
	for _, attachID := range args.StorageIds.Ids {
		err := processStorageAttachmentID(ctx, attachID)
		result = append(result, params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		})
	}

	return params.ErrorResults{Results: result}, nil
}

// detachStorageAttachment detaches exactly one storage attachment for a
// request. It expects that the caller has processed the supplied tags and can
// now provide string values representing the entities.
func (a *StorageAPI) detachStorageAttachment(
	ctx context.Context,
	storageID string,
	unitName coreunit.Name,
	force bool,
	wait time.Duration,
) error {
	unitUUID, err := a.applicationService.GetUnitUUID(ctx, unitName)
	switch {
	case errors.Is(err, coreunit.InvalidUnitName):
		return errors.Errorf("invalid unit name %q", unitName).Add(
			coreerrors.NotValid,
		)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return errors.Errorf("unit %q does not exist", unitName).Add(coreerrors.NotFound)
	case err != nil:
		return errors.Errorf(
			"getting unit uuid for unit name %q: %w", unitName, err,
		)
	}

	storageInstanceUUID, err := a.storageService.GetStorageInstanceUUIDForID(
		ctx, storageID,
	)
	switch {
	case errors.Is(err, storageerrors.StorageInstanceNotFound):
		return errors.Errorf("storage %q does not exist", storageID).Add(
			coreerrors.NotFound,
		)
	case err != nil:
		return errors.Errorf(
			"getting storage instance uuid for storage id %q: %w", storageID, err,
		)
	}

	storageAttachmentUUID, err := a.storageService.
		GetStorageAttachmentUUIDForStorageInstanceAndUnit(
			ctx, storageInstanceUUID, unitUUID,
		)
	// We purposely ignore not valid errors for the uuids supplied. We have
	// recieved these uuids from the domain and not the caller so they can
	// safely be considered valid.
	switch {
	case errors.Is(err, storageerrors.StorageInstanceNotFound):
		return errors.Errorf("storage %q does not exist", storageID).Add(coreerrors.NotFound)
	case errors.Is(err, storageerrors.StorageAttachmentNotFound):
		return errors.Errorf(
			"storage %q is not attached to unit %q", storageID, unitName,
		).Add(coreerrors.NotValid)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return errors.Errorf("unit %q does not exist", unitName).Add(coreerrors.NotFound)
	case err != nil:
		return errors.Errorf(
			"getting storage attachment uuid for storage %q attached to unit %q: %w",
			storageID, unitName, err,
		)
	}

	_, err = a.removalService.RemoveStorageAttachmentFromAliveUnit(
		ctx, storageAttachmentUUID, force, wait,
	)
	// Early exit path. Everything below this point can now be considered error
	// handling.
	if err == nil {
		return nil
	}

	viErr, has := errors.AsType[applicationerrors.UnitStorageMinViolation](err)
	if has {
		return errors.Errorf(
			"removing storage %q from unit %q would violate charm storage %q requirements of having minimum %d storage instances",
			storageID, unitName, viErr.CharmStorageName, viErr.RequiredMinimum,
		)
	}

	switch {
	case errors.Is(err, applicationerrors.UnitNotAlive):
		return errors.Errorf(
			"unit %q must be alive in order to remove storage %q",
			unitName, storageID,
		).Add(coreerrors.NotValid)
	case errors.Is(err, storageerrors.StorageAttachmentNotFound):
		// The storage attachment has already been removed. We had already
		// resolved that it existed above and so we can safely ignore this
		// error.
		return nil
	default:
		return errors.Errorf(
			"removing storage %q attachment to unit %q: %w",
			storageID, unitName, err,
		)
	}
}

// Attach attaches existing storage instances to units.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Attach(ctx context.Context, args params.StorageAttachmentIds) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(args.Ids))
	return params.ErrorResults{Results: result}, nil
}

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Import(ctx context.Context, args params.BulkImportStorageParamsV2) (params.ImportStorageResults, error) {
	results := make([]params.ImportStorageResult, len(args.Storage))
	return params.ImportStorageResults{Results: results}, nil
}

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPIv6) Import(ctx context.Context, args params.BulkImportStorageParams) (params.ImportStorageResults, error) {
	v2Args := params.BulkImportStorageParamsV2{Storage: make([]params.ImportStorageParamsV2, len(args.Storage))}
	for idx, param := range args.Storage {
		v2Args.Storage[idx] = params.ImportStorageParamsV2{
			Kind:        param.Kind,
			Pool:        param.Pool,
			ProviderId:  param.ProviderId,
			StorageName: param.StorageName,
			// Always false since force is not supported in v6.
			Force: false,
		}
	}
	return a.StorageAPI.Import(ctx, v2Args)
}

// RemovePool deletes the named pool
func (a *StorageAPI) RemovePool(ctx context.Context, p params.StoragePoolDeleteArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(p.Pools)),
	}
	if err := a.checkCanWrite(ctx); err != nil {
		return results, errors.Capture(err)
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
		return results, errors.Capture(err)
	}

	for i, pool := range p.Pools {
		err := a.storageService.ReplaceStoragePool(ctx, pool.Name, storage.ProviderType(pool.Provider), pool.Attrs)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}
