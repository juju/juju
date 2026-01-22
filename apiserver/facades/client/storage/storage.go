// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"time"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	coreversion "github.com/juju/juju/core/version"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/service/storage"
	domainremoval "github.com/juju/juju/domain/removal"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// BlockChecker defines the block-checking functionality required by
// the application facade. This is implemented by
// apiserver/common.BlockChecker.
type BlockChecker interface {
	ChangeAllowed(context.Context) error
}

// RemovalService defines the interface required for removing storage related
// entities in the model on behalf of an API caller.
type RemovalService interface {
	// RemoveStorageAttachment is responsible for removing a storage attachment
	// from a unit. If the unit is Alive then removing this storage attachment
	// must not violate the storage requirements of the charm.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] if the storage attachment uuid is not valid.
	// - [storageerrors.StorageAttachmentNotFound] if the storage attachment
	// does not exist in the model.
	// - [applicationerrors.UnitStorageMinViolation] if removing a storage
	// attachment would violate the charm minimums required for the unit.
	RemoveStorageAttachment(
		ctx context.Context,
		uuid domainstorage.StorageAttachmentUUID,
		force bool,
		wait time.Duration,
	) (domainremoval.UUID, error)

	// RemoveStorageInstance ensures that the specified storage instance is no
	// longer alive, scheduling removal jobs if needed and if specified, mark the
	// volume and filesystems for obliteration.
	RemoveStorageInstance(
		ctx context.Context,
		uuid domainstorage.StorageInstanceUUID,
		force bool, wait time.Duration,
		obliterate bool,
	) error
}

// StorageService defines apis on the storage service.
type StorageService interface {
	StoragePoolService

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
	) (domainstorage.StorageAttachmentUUID, error)

	// GetStorageInstanceAttachments returns the set of attachments a storage
	// instance has. If the storage instance has no attachments then an empty
	// slice is returned.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] when the supplied uuid did not pass validation.
	// - [github.com/juju/juju/domain/storage/errors.StorageInstanceNotFound] if
	// the storage instance for the supplied uuid does not exist.
	GetStorageInstanceAttachments(
		context.Context, domainstorage.StorageInstanceUUID,
	) ([]domainstorage.StorageAttachmentUUID, error)

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
}

// ApplicationService defines apis on the application service.
type ApplicationService interface {
	// GetUnitUUID returns the UUID for the named unit.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/unit.InvalidUnitName] if the unit name is
	// invalid.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] if the
	// unit doesn't exist.
	GetUnitUUID(context.Context, coreunit.Name) (coreunit.UUID, error)

	// AddStorageForUnit adds storage instances to the given unit.
	// Missing storage directive attributes are populated
	// based on model defaults.
	AddStorageForUnit(
		ctx context.Context, storageName corestorage.Name, unitUUID coreunit.UUID, arg storage.AddUnitStorageArgs,
	) ([]corestorage.ID, error)
}

// StorageAPIv6 provides the Storage API facade for version 6.
type StorageAPIv6 struct {
	*StorageAPI
}

// StorageAPI implements the latest version (v7) of the Storage API.
type StorageAPI struct {
	blockChecker       BlockChecker
	applicationService ApplicationService
	removalService     RemovalService
	storageService     StorageService

	authorizer     facade.Authorizer
	controllerUUID string
	modelUUID      coremodel.UUID
	logger         corelogger.Logger
}

func NewStorageAPI(
	controllerUUID string,
	modelUUID coremodel.UUID,
	authorizer facade.Authorizer,
	logger corelogger.Logger,
	blockChecker BlockChecker,
	applicationService ApplicationService,
	removalService RemovalService,
	storageService StorageService,
) *StorageAPI {
	return &StorageAPI{
		blockChecker:       blockChecker,
		applicationService: applicationService,
		removalService:     removalService,
		storageService:     storageService,

		authorizer:     authorizer,
		controllerUUID: controllerUUID,
		modelUUID:      modelUUID,
		logger:         logger,
	}
}

// checkHasModelPermission checks to see if the authenticated entity has the
// supplied permission on the model returning true or false. Any errors
// encountered performing the check are returned with false.
func (a *StorageAPI) checkHasModelPermission(
	ctx context.Context, perm permission.Access,
) (bool, error) {
	err := a.authorizer.HasPermission(
		ctx, perm, names.NewModelTag(a.modelUUID.String()),
	)
	if errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// checkHasSuperUserAccess checks to see if the authenticated entity has super
// user access on the controller. Any errors encountered performing the check
// are returned with false.
func (a *StorageAPI) checkHasSuperUserAccess(ctx context.Context) (bool, error) {
	err := a.authorizer.HasPermission(
		ctx,
		permission.SuperuserAccess,
		names.NewControllerTag(a.controllerUUID),
	)
	if errors.Is(err, authentication.ErrorEntityMissingPermission) {
		// Subject doesn't have superuser access on the controller
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// checkCanRead checks that the caller of the facade has read permissions on
// the model or super user access on the controller. If the caller does not have
// read permissions then a [params.Error] value is returned with the code set to
// [apiservererrors.CodeUnauthorized].
//
// NOTE: this is a slight change in patterns from how this type of authorisation
// check would be performed in a facade. We explicitly return a params error
// here that has the unauthorized code check so that the facade contracts are
// strict and can be tested within the facade.
//
// By returning any error out of this func and subsequently out of the facade
// func we are relying on a catch all handler at the api root to perform the
// last transformation. Relying on this means the facades contracts can not be
// tested.
//
// Errors encountered checking permissions will be logged at warning level with a
// params error returned and the code set to [apiservererrors.CodeUnauthorized].
func (a *StorageAPI) checkCanRead(ctx context.Context) *params.Error {
	hasSuperUser, err := a.checkHasSuperUserAccess(ctx)
	if err != nil {
		a.logger.Warningf(
			ctx,
			"checking for super user access on entity %q: %s",
			a.authorizer.GetAuthTag().String(),
			err.Error(),
		)
		return apiservererrors.ParamsErrorf(
			params.CodeUnauthorized, "not authorized for request",
		)
	}

	if hasSuperUser {
		// authenticated entity has super user access on the controller.
		return nil
	}

	hasModelRead, err := a.checkHasModelPermission(ctx, permission.ReadAccess)
	if err != nil {
		a.logger.Warningf(
			ctx,
			"checking for read access on entity %q: %s",
			a.authorizer.GetAuthTag().String(),
			err.Error(),
		)
		return apiservererrors.ParamsErrorf(
			params.CodeUnauthorized, "not authorized for request",
		)
	}

	if hasModelRead {
		return nil
	}

	return apiservererrors.ParamsErrorf(
		params.CodeUnauthorized, "not authorized for request",
	)
}

// checkCanWrite checks that the caller of the facade has write permissions on
// the model. If the caller does not have write permissions then a
// [params.Error] value is returned with the code set to
// [apiservererrors.CodeUnauthorized].
//
// NOTE: this is a slight change in patterns from how this type of authorisation
// check would be performed in a facade. We explicitly return a params error
// here that has the unauthorized code check so that the facade contracts are
// strict and can be tested within the facade.
//
// By returning any error out of this func and subsequently out of the facade
// func we are relying on a catch all handler at the api root to perform the
// last transformation. Relying on this means the facades contracts can not be
// tested.
//
// Errors encountered checking permissions will be logged at warning level with a
// params error returned and the code set to [apiservererrors.CodeUnauthorized].
func (a *StorageAPI) checkCanWrite(ctx context.Context) *params.Error {
	hasModelWrite, err := a.checkHasModelPermission(ctx, permission.WriteAccess)
	if err != nil {
		a.logger.Warningf(
			ctx,
			"checking for write access on entity %q: %s",
			a.authorizer.GetAuthTag().String(),
			err.Error(),
		)
		return apiservererrors.ParamsErrorf(
			params.CodeUnauthorized, "not authorized for request",
		)
	}

	if hasModelWrite {
		return nil
	}

	return apiservererrors.ParamsErrorf(
		params.CodeUnauthorized, "not authorized for request",
	)
}

// StorageDetails retrieves and returns detailed information about desired
// storage identified by supplied tags. If specified storage cannot be
// retrieved, individual error is returned instead of storage information.
func (a *StorageAPI) StorageDetails(ctx context.Context, entities params.Entities) (params.StorageDetailsResults, error) {
	return params.StorageDetailsResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}

// ListStorageDetails returns storage matching a filter.
func (a *StorageAPI) ListStorageDetails(ctx context.Context, filters params.StorageFilters) (params.StorageDetailsListResults, error) {
	return params.StorageDetailsListResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}

// ListVolumes lists volumes with the given filters. Each filter produces
// an independent list of volumes, or an error if the filter is invalid
// or the volumes could not be listed.
func (a *StorageAPI) ListVolumes(ctx context.Context, filters params.VolumeFilters) (params.VolumeDetailsListResults, error) {
	return params.VolumeDetailsListResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}

// ListFilesystems returns a list of filesystems in the environment matching
// the provided filter. Each result describes a filesystem in detail, including
// the filesystem's attachments.
func (a *StorageAPI) ListFilesystems(ctx context.Context, filters params.FilesystemFilters) (params.FilesystemDetailsListResults, error) {
	return params.FilesystemDetailsListResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}

// AddToUnit validates and creates additional storage instances for units.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) AddToUnit(ctx context.Context, args params.StoragesAddParams) (params.AddStorageResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.AddStorageResults{}, errors.Capture(err)
	}

	// Check if changes are allowed and the operation may proceed.
	if err := a.blockChecker.ChangeAllowed(ctx); err != nil {
		return params.AddStorageResults{}, errors.Capture(err)
	}

	result := make([]params.AddStorageResult, len(args.Storages))
	for i, one := range args.Storages {
		storageIDs, err := a.addOneStorage(ctx, one)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		tagStrings := make([]string, len(storageIDs))
		for i, id := range storageIDs {
			tagStrings[i] = names.NewStorageTag(id.String()).String()
		}
		result[i].Result = &params.AddStorageDetails{
			StorageTags: tagStrings,
		}
	}
	return params.AddStorageResults{Results: result}, nil
}

func (a *StorageAPI) addOneStorage(ctx context.Context, one params.StorageAddParams) ([]corestorage.ID, error) {
	u, err := names.ParseUnitTag(one.UnitTag)
	if err != nil {
		return nil, errors.Capture(err)
	}

	unitName := coreunit.Name(u.Id())
	unitUUID, err := a.applicationService.GetUnitUUID(ctx, unitName)
	switch {
	case errors.Is(err, coreunit.InvalidUnitName):
		return nil, errors.Errorf("invalid unit name %q", unitName).Add(
			coreerrors.NotValid,
		)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return nil, errors.Errorf("unit %q does not exist", unitName).Add(coreerrors.NotFound)
	case err != nil:
		return nil, errors.Errorf(
			"getting unit uuid for unit name %q: %w", unitName, err,
		)
	}

	result, err := a.applicationService.AddStorageForUnit(
		ctx, corestorage.Name(one.StorageName), unitUUID, storage.AddUnitStorageArgs{
			StorageName: corestorage.Name(one.StorageName),
			SizeMiB:     one.Directives.SizeMiB,
			Count:       one.Directives.Count,
		},
	)
	switch {
	case errors.Is(err, corestorage.InvalidStorageName):
		return nil, errors.Errorf("invalid storage name %q", one.StorageName).Add(
			coreerrors.NotValid,
		)
	case errors.Is(err, applicationerrors.StorageNameNotSupported):
		return nil, errors.Errorf("storage name %q not supported by charm", one.StorageName).Add(
			coreerrors.NotSupported,
		)
	case errors.Is(err, applicationerrors.InvalidStorageCount):
		count := uint64(0)
		if one.Directives.Count != nil {
			count = *one.Directives.Count
		}
		return nil, errors.Errorf("storage count %d not valid for storage %q", count, one.StorageName).Add(
			coreerrors.NotValid,
		)
	case err != nil:
		return nil, errors.Errorf(
			"adding storage %q to unit name %q: %w", one.StorageName, unitName, err,
		)
	}
	return result, nil
}

// Remove sets the specified storage entities to Dying, unless they are
// already Dying or Dead, such that the storage will eventually be removed
// from the model. If the arguments specify that the storage should be
// destroyed, then the associated cloud storage will be destroyed first;
// otherwise it will only be released from Juju's control.
func (a *StorageAPI) Remove(ctx context.Context, args params.RemoveStorage) (params.ErrorResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, errors.Capture(err)
	}

	one := func(arg params.RemoveStorageInstance) error {
		tag, err := names.ParseStorageTag(arg.Tag)
		if err != nil {
			return errors.New("invalid storage tag").Add(coreerrors.NotValid)
		}
		return a.removeStorageInstance(ctx, tag, arg)
	}

	results := make([]params.ErrorResult, 0, len(args.Storage))
	for _, v := range args.Storage {
		var result params.ErrorResult
		err := one(v)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		}
		results = append(results, result)
	}
	return params.ErrorResults{Results: results}, nil
}

// removeStorageInstance performs the operations to remove a storage instance
// for the corresponding Remove facade method.
func (a *StorageAPI) removeStorageInstance(
	ctx context.Context, tag names.StorageTag, arg params.RemoveStorageInstance,
) error {
	force := false
	if arg.Force != nil {
		force = *arg.Force
	}
	wait := time.Duration(0)
	if arg.MaxWait != nil {
		wait = *arg.MaxWait
	}
	if wait < 0 {
		return errors.Errorf(
			"max wait time cannot be a negative number",
		).Add(coreerrors.NotValid)
	}

	uuid, err := a.storageService.GetStorageInstanceUUIDForID(ctx, tag.Id())
	if errors.Is(err, storageerrors.StorageInstanceNotFound) {
		return errors.Errorf("storage %q does not exist", tag.Id()).Add(
			coreerrors.NotFound,
		)
	} else if err != nil {
		return errors.Errorf(
			"getting storage instance uuid for storage id %q: %w",
			tag.Id(), err,
		)
	}

	if arg.DestroyAttachments {
		saUUIDs, err := a.storageService.GetStorageInstanceAttachments(
			ctx, uuid)
		if errors.Is(err, storageerrors.StorageInstanceNotFound) {
			return errors.Errorf("storage %q does not exist", tag.Id()).Add(
				coreerrors.NotFound,
			)
		} else if err != nil {
			return errors.Errorf(
				"getting attachments of storage instance %q: %w",
				tag.Id(), err,
			)
		}
		for _, saUUID := range saUUIDs {
			err := a.detachStorageAttachment(ctx, saUUID, force, wait)
			if err != nil {
				return errors.Errorf(
					"removing storage attachment %q for storage %q:",
					saUUID, tag.Id(),
				)
			}
		}
	}

	obliterate := arg.DestroyStorage
	err = a.removalService.RemoveStorageInstance(
		ctx, uuid, force, wait, obliterate)
	if errors.Is(err, storageerrors.StorageInstanceNotFound) {
		return errors.Errorf("storage %q does not exist", tag.Id()).Add(
			coreerrors.NotFound,
		)
	} else if err != nil {
		return errors.Errorf("removing storage %q: %w", tag.Id(), err)
	}

	return nil
}

// Attach attaches existing storage instances to units.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Attach(ctx context.Context, args params.StorageAttachmentIds) (params.ErrorResults, error) {
	return params.ErrorResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Import(ctx context.Context, args params.BulkImportStorageParamsV2) (params.ImportStorageResults, error) {
	return params.ImportStorageResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPIv6) Import(ctx context.Context, args params.BulkImportStorageParams) (params.ImportStorageResults, error) {
	return params.ImportStorageResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}
