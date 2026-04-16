// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"math"
	"time"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	domainapplication "github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainremoval "github.com/juju/juju/domain/removal"
	statusservice "github.com/juju/juju/domain/status/service"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// BlockChecker defines the block-checking functionality required by
// the storage facade. This is implemented by
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
	StorageAdoptionService

	// GetStorageAttachmentUUIDForStorageInstanceAndUnit returns the
	// [storage.StorageAttachmentUUID] associated with the given storage
	// instance id and unit name.
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

	// GetStorageInstanceInfo returns the basic information about a
	// StorageInstance in the model.
	GetStorageInstanceInfo(
		context.Context, domainstorage.StorageInstanceUUID,
	) (domainstorage.StorageInstanceInfo, error)

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

	// GetStoragePoolUUID returns the UUID of the storage pool for the specified name.
	GetStoragePoolUUID(context.Context, string) (domainstorage.StoragePoolUUID, error)

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

	// GetVolumesByMachines returns all the volumes attached to or owned by the
	// specified machines.
	GetVolumesByMachines(
		ctx context.Context, uuids []coremachine.UUID,
	) ([]domainstorage.VolumeUUID, error)

	// GetFilesystemsByMachines returns all the filesystems attached to or owned
	// by the specified machines.
	GetFilesystemsByMachines(
		ctx context.Context, uuids []coremachine.UUID,
	) ([]domainstorage.FilesystemUUID, error)
}

// StatusService defines service methods required to perform bulk listing of
// storage entities.
type StatusService interface {
	// GetStorageInstanceStatuses returns the specified storage instance statuses.
	GetStorageInstanceStatuses(
		ctx context.Context, uuids []domainstorage.StorageInstanceUUID,
	) ([]statusservice.StorageInstance, error)

	// GetAllStorageInstanceStatuses returns all the storage instance statuses for
	// the model.
	GetAllStorageInstanceStatuses(ctx context.Context) ([]statusservice.StorageInstance, error)

	// GetFilesystemStatuses returns the specified filesystem statuses.
	GetFilesystemStatuses(
		ctx context.Context, uuids []domainstorage.FilesystemUUID,
	) ([]statusservice.Filesystem, error)

	// GetAllFilesystemStatuses returns all the filesystem statuses for the model.
	GetAllFilesystemStatuses(ctx context.Context) ([]statusservice.Filesystem, error)

	// GetVolumeStatuses returns the specified volume statuses.
	GetVolumeStatuses(
		ctx context.Context, uuids []domainstorage.VolumeUUID,
	) ([]statusservice.Volume, error)

	// GetAllVolumeStatuses returns all the volume statuses for the model.
	GetAllVolumeStatuses(ctx context.Context) ([]statusservice.Volume, error)
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

	// AddStorageForIAASUnit adds storage instances to the given unit.
	AddStorageForIAASUnit(
		context.Context,
		corestorage.Name,
		coreunit.UUID,
		uint32,
		domainapplication.AddUnitStorageOverride,
	) ([]corestorage.ID, error)

	// AddStorageForCAASUnit adds storage instances to the given unit.
	AddStorageForCAASUnit(
		context.Context,
		corestorage.Name,
		coreunit.UUID,
		uint32,
		domainapplication.AddUnitStorageOverride,
	) ([]corestorage.ID, error)

	// AttachStorageToUnit ensures the specified storage instance can be
	// attached to the specified unit and then attaches it.
	//
	// The following errors can be expected:
	// - [storageerrors.StorageInstanceNotFound] when the storage instance does
	// not exist.
	// - [storageerrors.StorageInstanceNotAlive] when the storage instance is
	// not alive.
	// - [applicationerrors.UnitNotFound] when the unit does not exist.
	// - [applicationerrors.UnitNotAlive] when the unit is not alive.
	// - [applicationerrors.StorageNameNotSupported] when the unit's charm does
	// not define the storage name.
	// - [applicationerrors.StorageInstanceCharmNameMismatch] when the storage
	// instance charm name does not match the unit charm.
	// - [applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition]
	// when the storage kind does not match the charm storage definition.
	// - [applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition]
	// when the storage size is below the charm minimum.
	// - [applicationerrors.StorageCountLimitExceeded] when attaching would
	// exceed the charm storage maximum count, including when a concurrent
	// attachment has caused the count to be exceeded since validation.
	// - [applicationerrors.StorageInstanceAlreadyAttachedToUnit] when the
	// storage instance is already attached to the unit.
	// - [applicationerrors.StorageInstanceAttachSharedAccessNotSupported] when
	// the storage instance has existing attachments but the unit's charm
	// storage definition does not support shared access.
	// - [applicationerrors.StorageInstanceUnexpectedAttachments] when the
	// storage instance attachments changed concurrently during the attach
	// operation.
	// - [applicationerrors.StorageInstanceAttachMachineOwnerMismatch] when the
	// storage instance owning machine does not match the unit's machine.
	// - [applicationerrors.UnitCharmChanged] when the unit's charm has changed
	// concurrently during the attach operation.
	// - [applicationerrors.UnitMachineChanged] when the unit's machine has
	// changed concurrently during the attach operation.
	AttachStorageToUnit(
		ctx context.Context,
		storageUUID domainstorage.StorageInstanceUUID,
		unitUUID coreunit.UUID,
	) error
}

// MachineService defines the service methods required by the Storage facade for
// resolving machines.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name coremachine.Name) (coremachine.UUID, error)
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
	machineService     MachineService
	statusService      StatusService

	authorizer     facade.Authorizer
	controllerUUID string
	modelUUID      coremodel.UUID
	modelType      coremodel.ModelType
	logger         corelogger.Logger
}

func NewStorageAPI(
	controllerUUID string,
	modelUUID coremodel.UUID,
	modelType coremodel.ModelType,
	authorizer facade.Authorizer,
	logger corelogger.Logger,
	blockChecker BlockChecker,
	applicationService ApplicationService,
	removalService RemovalService,
	storageService StorageService,
	machineService MachineService,
	statusService StatusService,
) *StorageAPI {
	return &StorageAPI{
		blockChecker:       blockChecker,
		applicationService: applicationService,
		removalService:     removalService,
		storageService:     storageService,
		machineService:     machineService,
		statusService:      statusService,

		authorizer:     authorizer,
		controllerUUID: controllerUUID,
		modelUUID:      modelUUID,
		modelType:      modelType,
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

// ListStorageDetails returns detailed information for every storage instance
// within the model. ListStorageDetails does not currently support any filtering
// of the information returned. At least one filter arg is required to be
// supplied by the caller.
func (a *StorageAPI) ListStorageDetails(
	ctx context.Context,
	filters params.StorageFilters,
) (params.StorageDetailsListResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.StorageDetailsListResults{}, errors.Capture(err)
	}

	// While filters don't filter anything in this facade we do require at least
	// one be supplied by the caller. This is so that we can match the results
	// to the filter index.
	//
	// This check must always occur after permission checks.
	if len(filters.Filters) == 0 {
		return params.StorageDetailsListResults{}, apiservererrors.ParamsErrorf(
			params.CodeNotValid, "at least one filter is required",
		)
	}

	// zeroTime is used to set the status time when a storage instance does not
	// have a status time set.
	zeroTime := time.UnixMicro(0).UTC()

	// processStorageInstance is responsible for taking a single storage
	// instance representation in the model and building a
	// [params.StorageDetails] struct to provide to the caller.
	processStorageInstance := func(
		si statusservice.StorageInstance,
	) params.StorageDetails {
		retVal := params.StorageDetails{}
		storageInstTag := names.NewStorageTag(si.ID)
		retVal.StorageTag = storageInstTag.String()

		var ownerTag string
		if si.Owner != nil {
			ownerTag = names.NewUnitTag(si.Owner.String()).String()
		}
		retVal.OwnerTag = ownerTag

		// Default storage kind when we can't translate to any other value.
		storageKind := params.StorageKindUnknown
		switch si.Kind {
		case domainstorage.StorageKindBlock:
			storageKind = params.StorageKindBlock
		case domainstorage.StorageKindFilesystem:
			storageKind = params.StorageKindFilesystem
		}
		retVal.Kind = storageKind

		retVal.Life = si.Life
		retVal.Status = params.EntityStatus{
			Status: si.Status.Status,
			Info:   si.Status.Message,
			Data:   si.Status.Data,
			Since:  si.Status.Since,
		}
		if si.Status.Since == nil {
			// This prevents a panic in clients due to a storage instance after
			// 4.0 possibly having no filesystem or volume to get a status from.
			// This is poor API design anyway, since a storage instance does not
			// have a status, instead, we've pulled one from the provisioned
			// entities.
			retVal.Status.Since = &zeroTime
		}

		retVal.Attachments = make(
			map[string]params.StorageAttachmentDetails, len(si.Attachments),
		)
		for _, attachment := range si.Attachments {
			unitTag := names.NewUnitTag(attachment.Unit.String())
			var machineTagStr string

			if attachment.Machine != nil {
				machineTagStr = names.NewMachineTag(attachment.Machine.String()).String()
			}

			sad := params.StorageAttachmentDetails{
				Life:       attachment.Life,
				Location:   attachment.Location,
				MachineTag: machineTagStr,
				StorageTag: storageInstTag.String(),
				UnitTag:    unitTag.String(),
			}

			retVal.Attachments[unitTag.String()] = sad
		}
		return retVal
	}

	// We don't support any storage filtering at the moment.
	storageInstances, err := a.statusService.GetAllStorageInstanceStatuses(ctx)
	if err != nil {
		// We log the error instead of returning it to the caller. The contents
		// are unknown and will reflect an internal server error.
		a.logger.Errorf(
			ctx,
			"failed getting storage instance statuses for listing storage details: %s",
			err.Error(),
		)
		return params.StorageDetailsListResults{}, errors.Errorf(
			"failed getting available storage instances",
		)
	}

	results := make([]params.StorageDetails, 0, len(storageInstances))
	for _, storageInstance := range storageInstances {
		results = append(results, processStorageInstance(storageInstance))
	}

	retVal := params.StorageDetailsListResults{
		Results: make([]params.StorageDetailsListResult, 0, len(filters.Filters)),
	}
	for range filters.Filters {
		retVal.Results = append(retVal.Results, params.StorageDetailsListResult{
			Result: results,
		})
	}

	return retVal, nil
}

// ListVolumes lists volumes with the given filters. Each filter produces
// an independent list of volumes, or an error if the filter is invalid
// or the volumes could not be listed.
func (a *StorageAPI) ListVolumes(
	ctx context.Context, filters params.VolumeFilters,
) (params.VolumeDetailsListResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.VolumeDetailsListResults{}, errors.Capture(err)
	}

	one := func(arg params.VolumeFilter) ([]params.VolumeDetails, error) {
		if len(arg.Machines) == 0 {
			return a.listVolumes(ctx)
		}

		var machineTags []names.MachineTag
		for _, m := range arg.Machines {
			t, err := names.ParseMachineTag(m)
			if err != nil {
				return nil, errors.Errorf(
					"invalid machine tag: %w", err,
				).Add(coreerrors.NotValid)
			}
			machineTags = append(machineTags, t)
		}

		return a.listVolumesOnMachines(ctx, machineTags)
	}

	results := params.VolumeDetailsListResults{
		Results: make([]params.VolumeDetailsListResult, 0, len(filters.Filters)),
	}
	for _, arg := range filters.Filters {
		var result params.VolumeDetailsListResult
		var err error
		result.Result, err = one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		}
		results.Results = append(results.Results, result)
	}

	return results, nil
}

// listVolumes is responsible for listing all volumes in the model where no
// filtering is to be applied.
func (a *StorageAPI) listVolumes(
	ctx context.Context,
) ([]params.VolumeDetails, error) {
	storageInstances, err := a.statusService.GetAllStorageInstanceStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	volumes, err := a.statusService.GetAllVolumeStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return a.volumeDetails(ctx, storageInstances, volumes)
}

func (a *StorageAPI) listVolumesOnMachines(
	ctx context.Context, machineTags []names.MachineTag,
) ([]params.VolumeDetails, error) {
	var machineUUIDs []coremachine.UUID
	for _, machineTag := range machineTags {
		name := coremachine.Name(machineTag.Id())
		machineUUID, err := a.machineService.GetMachineUUID(ctx, name)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return nil, errors.Errorf(
				"machine %q not found", name,
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return nil, errors.Errorf(
				"getting machine uuid for machine %q: %w", name, err,
			)
		}
		machineUUIDs = append(machineUUIDs, machineUUID)
	}

	volumeUUIDs, err := a.storageService.GetVolumesByMachines(ctx, machineUUIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if len(volumeUUIDs) == 0 {
		return nil, nil
	}

	volumes, err := a.statusService.GetVolumeStatuses(ctx, volumeUUIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var storageInstanceUUIDs []domainstorage.StorageInstanceUUID
	for _, v := range volumes {
		if v.StorageUUID != nil {
			storageInstanceUUIDs = append(storageInstanceUUIDs, *v.StorageUUID)
		}
	}
	var storageInstances []statusservice.StorageInstance
	if len(storageInstanceUUIDs) > 0 {
		storageInstances, err = a.statusService.GetStorageInstanceStatuses(
			ctx, storageInstanceUUIDs)
		if err != nil {
			return nil, errors.Capture(err)
		}
	}

	return a.volumeDetails(ctx, storageInstances, volumes)
}

func (a *StorageAPI) volumeDetails(
	ctx context.Context,
	storageInstances []statusservice.StorageInstance,
	volumes []statusservice.Volume,
) ([]params.VolumeDetails, error) {
	storageMap := map[string]*params.StorageDetails{}

	// zeroTime is used to set the status time when a storage instance does not
	// have a status time set.
	zeroTime := time.UnixMicro(0).UTC()

	for _, v := range storageInstances {
		details := params.StorageDetails{
			Status: params.EntityStatus{
				Status: v.Status.Status,
				Info:   v.Status.Message,
				Data:   v.Status.Data,
				Since:  v.Status.Since,
			},
			StorageTag: names.NewStorageTag(v.ID).String(),
			Life:       v.Life,
		}
		if v.Status.Since == nil {
			// This prevents a panic in clients due to a storage instance after
			// 4.0 possibly having no filesystem or volume to get a status from.
			// This is poor API design anyway, since a storage instance does not
			// have a status, instead, we've pulled one from the provisioned
			// entities.
			details.Status.Since = &zeroTime
		}
		if v.Owner != nil {
			details.OwnerTag = names.NewUnitTag(v.Owner.String()).String()
		}
		switch v.Kind {
		case domainstorage.StorageKindBlock:
			details.Kind = params.StorageKindBlock
		case domainstorage.StorageKindFilesystem:
			details.Kind = params.StorageKindFilesystem
		default:
			details.Kind = params.StorageKindUnknown
		}
		for unit, sa := range v.Attachments {
			unitTag := names.NewUnitTag(unit.String())
			sad := params.StorageAttachmentDetails{
				StorageTag: details.StorageTag,
				UnitTag:    names.NewUnitTag(sa.Unit.String()).String(),
			}
			if sa.Machine != nil {
				sad.MachineTag = names.NewMachineTag(sa.Machine.String()).String()
			}
			if details.Attachments == nil {
				details.Attachments = map[string]params.StorageAttachmentDetails{}
			}
			details.Attachments[unitTag.String()] = sad
		}
		// Store in a map to get the status and location from either the
		// filesystem or volumes. These are a facade concern, hence why it
		// is done here.
		storageMap[v.ID] = &details
	}

	volumeResult := make([]params.VolumeDetails, 0, len(volumes))
	for _, v := range volumes {
		details := params.VolumeDetails{
			VolumeTag: names.NewVolumeTag(v.ID).String(),
			Life:      v.Life,
			Info: params.VolumeInfo{
				ProviderId: v.ProviderID,
				HardwareId: v.HardwareID,
				WWN:        v.WWN,
				SizeMiB:    v.SizeMiB,
				Persistent: v.Persistent,
			},
			Status: params.EntityStatus{
				Status: v.Status.Status,
				Info:   v.Status.Message,
				Data:   v.Status.Data,
				Since:  v.Status.Since,
			},
		}
		if v.Status.Since == nil {
			// This prevents a panic in clients due to a storage instance after
			// 4.0 possibly having no filesystem or volume to get a status from.
			// This is poor API design anyway, since a storage instance does not
			// have a status, instead, we've pulled one from the provisioned
			// entities.
			details.Status.Since = &zeroTime
		}

		unitAttachmentLocations := map[string]string{}
		for unit, va := range v.UnitAttachments {
			vad := params.VolumeAttachmentDetails{
				Life: va.Life,
				VolumeAttachmentInfo: params.VolumeAttachmentInfo{
					DeviceName: va.DeviceName,
					DeviceLink: va.DeviceLink,
					BusAddress: va.BusAddress,
					ReadOnly:   va.ReadOnly,
				},
			}
			if vap := va.VolumeAttachmentPlan; vap != nil {
				vad.VolumeAttachmentInfo.PlanInfo = &params.VolumeAttachmentPlanInfo{
					DeviceAttributes: vap.DeviceAttributes,
					DeviceType:       vap.DeviceType.String(),
				}
			}
			if details.UnitAttachments == nil {
				details.UnitAttachments = map[string]params.VolumeAttachmentDetails{}
			}
			unitTag := names.NewUnitTag(unit.String()).String()
			details.UnitAttachments[unitTag] = vad

			var deviceLinks []string
			if va.DeviceLink != "" {
				deviceLinks = append(deviceLinks, vad.DeviceLink)
			}
			blockDevicePath, _ := blockdevice.BlockDevicePath(blockdevice.BlockDevice{
				HardwareId:  v.HardwareID,
				WWN:         v.WWN,
				DeviceName:  va.DeviceName,
				DeviceLinks: deviceLinks,
			})
			unitAttachmentLocations[unitTag] = blockDevicePath
		}
		for machine, va := range v.MachineAttachments {
			vad := params.VolumeAttachmentDetails{
				Life: va.Life,
				VolumeAttachmentInfo: params.VolumeAttachmentInfo{
					DeviceName: va.DeviceName,
					DeviceLink: va.DeviceLink,
					BusAddress: va.BusAddress,
					ReadOnly:   va.ReadOnly,
				},
			}
			if vap := va.VolumeAttachmentPlan; vap != nil {
				vad.VolumeAttachmentInfo.PlanInfo = &params.VolumeAttachmentPlanInfo{
					DeviceAttributes: vap.DeviceAttributes,
					DeviceType:       vap.DeviceType.String(),
				}
			}
			if details.MachineAttachments == nil {
				details.MachineAttachments = map[string]params.VolumeAttachmentDetails{}
			}
			machineTag := names.NewMachineTag(machine.String()).String()
			details.MachineAttachments[machineTag] = vad
		}
		if storage, ok := storageMap[v.StorageID]; ok {
			if storage.Kind == params.StorageKindBlock {
				storage.Status = details.Status
				storage.Persistent = details.Info.Persistent
				// give the storage instance attachment the unit's attachment
				// location.
				for k, v := range unitAttachmentLocations {
					ad, ok := storage.Attachments[k]
					if !ok {
						continue
					}
					ad.Location = v
					storage.Attachments[k] = ad
				}
			}
			details.Storage = storage
		}
		volumeResult = append(volumeResult, details)
	}

	return volumeResult, nil
}

// ListFilesystems returns a list of filesystems in the environment matching
// the provided filter. Each result describes a filesystem in detail, including
// the filesystem's attachments.
func (a *StorageAPI) ListFilesystems(
	ctx context.Context,
	filters params.FilesystemFilters,
) (params.FilesystemDetailsListResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.FilesystemDetailsListResults{}, errors.Capture(err)
	}

	one := func(arg params.FilesystemFilter) ([]params.FilesystemDetails, error) {
		if len(arg.Machines) == 0 {
			return a.listFilesystems(ctx)
		}

		var tags []names.MachineTag
		for _, m := range arg.Machines {
			t, err := names.ParseMachineTag(m)
			if err != nil {
				return nil, errors.Errorf(
					"invalid machine tag: %w", err,
				).Add(coreerrors.NotValid)
			}
			tags = append(tags, t)
		}

		return a.listFilesystemsOnMachines(ctx, tags)
	}

	results := params.FilesystemDetailsListResults{
		Results: make([]params.FilesystemDetailsListResult, 0, len(filters.Filters)),
	}
	for _, arg := range filters.Filters {
		var result params.FilesystemDetailsListResult
		var err error
		result.Result, err = one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		}
		results.Results = append(results.Results, result)
	}

	return results, nil
}

func (a *StorageAPI) listFilesystems(
	ctx context.Context,
) ([]params.FilesystemDetails, error) {
	storageInstances, err := a.statusService.GetAllStorageInstanceStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	filesystems, err := a.statusService.GetAllFilesystemStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return a.filesystemDetails(ctx, storageInstances, filesystems)
}

func (a *StorageAPI) listFilesystemsOnMachines(
	ctx context.Context, machineTags []names.MachineTag,
) ([]params.FilesystemDetails, error) {
	var machineUUIDs []coremachine.UUID
	for _, machineTag := range machineTags {
		name := coremachine.Name(machineTag.Id())
		machineUUID, err := a.machineService.GetMachineUUID(ctx, name)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return nil, errors.Errorf(
				"machine %q not found", name,
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return nil, errors.Errorf(
				"getting machine uuid for machine %q: %w", name, err,
			)
		}
		machineUUIDs = append(machineUUIDs, machineUUID)
	}

	filesystemUUIDs, err := a.storageService.GetFilesystemsByMachines(ctx, machineUUIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if len(filesystemUUIDs) == 0 {
		return nil, nil
	}

	filesystems, err := a.statusService.GetFilesystemStatuses(ctx, filesystemUUIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var storageInstanceUUIDs []domainstorage.StorageInstanceUUID
	for _, v := range filesystems {
		if v.StorageUUID != nil {
			storageInstanceUUIDs = append(storageInstanceUUIDs, *v.StorageUUID)
		}
	}
	var storageInstances []statusservice.StorageInstance
	if len(storageInstanceUUIDs) > 0 {
		storageInstances, err = a.statusService.GetStorageInstanceStatuses(
			ctx, storageInstanceUUIDs)
		if err != nil {
			return nil, errors.Capture(err)
		}
	}

	return a.filesystemDetails(ctx, storageInstances, filesystems)
}

func (a *StorageAPI) filesystemDetails(
	ctx context.Context,
	storageInstances []statusservice.StorageInstance,
	filesystems []statusservice.Filesystem,
) ([]params.FilesystemDetails, error) {
	// zeroTime is used to set the status time when a storage instance does not
	// have a status time set.
	zeroTime := time.UnixMicro(0).UTC()

	storageMap := map[string]*params.StorageDetails{}
	for _, v := range storageInstances {
		details := params.StorageDetails{
			StorageTag: names.NewStorageTag(v.ID).String(),
			Life:       v.Life,
		}
		if v.Owner != nil {
			details.OwnerTag = names.NewUnitTag(v.Owner.String()).String()
		}
		switch v.Kind {
		case domainstorage.StorageKindBlock:
			details.Kind = params.StorageKindBlock
		case domainstorage.StorageKindFilesystem:
			details.Kind = params.StorageKindFilesystem
		default:
			details.Kind = params.StorageKindUnknown
		}
		for unit, sa := range v.Attachments {
			unitTag := names.NewUnitTag(unit.String())
			sad := params.StorageAttachmentDetails{
				StorageTag: details.StorageTag,
				UnitTag:    names.NewUnitTag(sa.Unit.String()).String(),
			}
			if sa.Machine != nil {
				sad.MachineTag = names.NewMachineTag(sa.Machine.String()).String()
			}
			if details.Attachments == nil {
				details.Attachments = map[string]params.StorageAttachmentDetails{}
			}
			details.Attachments[unitTag.String()] = sad
		}
		// Store in a map to get the status and location from either the
		// filesystem or volumes. These are a facade concern, hence why it
		// is done here.
		storageMap[v.ID] = &details
	}

	filesystemResult := make([]params.FilesystemDetails, 0, len(filesystems))
	for _, v := range filesystems {
		details := params.FilesystemDetails{
			FilesystemTag: names.NewFilesystemTag(v.ID).String(),
			Life:          v.Life,
			Info: params.FilesystemInfo{
				ProviderId: v.ProviderID,
				SizeMiB:    v.SizeMiB,
			},
			Status: params.EntityStatus{
				Status: v.Status.Status,
				Info:   v.Status.Message,
				Data:   v.Status.Data,
				Since:  v.Status.Since,
			},
		}
		if v.Status.Since == nil {
			// This prevents a panic in clients due to a storage instance after
			// 4.0 possibly having no filesystem or volume to get a status from.
			// This is poor API design anyway, since a storage instance does not
			// have a status, instead, we've pulled one from the provisioned
			// entities.
			details.Status.Since = &zeroTime
		}

		if v.VolumeID != nil {
			details.VolumeTag = names.NewVolumeTag(*v.VolumeID).String()
		}
		unitAttachmentLocations := map[string]string{}
		for unit, fa := range v.UnitAttachments {
			fad := params.FilesystemAttachmentDetails{
				Life: fa.Life,
				FilesystemAttachmentInfo: params.FilesystemAttachmentInfo{
					MountPoint: fa.MountPoint,
					ReadOnly:   fa.ReadOnly,
				},
			}
			if details.UnitAttachments == nil {
				details.UnitAttachments = map[string]params.FilesystemAttachmentDetails{}
			}
			unitTag := names.NewUnitTag(unit.String()).String()
			details.UnitAttachments[unitTag] = fad
			unitAttachmentLocations[unitTag] = fa.MountPoint
		}
		for machine, fa := range v.MachineAttachments {
			fad := params.FilesystemAttachmentDetails{
				Life: fa.Life,
				FilesystemAttachmentInfo: params.FilesystemAttachmentInfo{
					MountPoint: fa.MountPoint,
					ReadOnly:   fa.ReadOnly,
				},
			}
			if details.MachineAttachments == nil {
				details.MachineAttachments = map[string]params.FilesystemAttachmentDetails{}
			}
			machineTag := names.NewMachineTag(machine.String()).String()
			details.MachineAttachments[machineTag] = fad
		}
		if storage, ok := storageMap[v.StorageID]; ok {
			if storage.Kind == params.StorageKindFilesystem {
				storage.Status = details.Status

				// give the storage instance attachment the unit's attachment
				// location.
				for k, v := range unitAttachmentLocations {
					ad, ok := storage.Attachments[k]
					if !ok {
						continue
					}
					ad.Location = v
					storage.Attachments[k] = ad
				}
			}
			details.Storage = storage
		}
		filesystemResult = append(filesystemResult, details)
	}

	return filesystemResult, nil
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
		return nil, apiservererrors.ParamsErrorf(params.CodeNotValid,
			"invalid unit name %q", unitName)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return nil, apiservererrors.ParamsErrorf(params.CodeNotFound,
			"unit %q does not exist", unitName)
	case err != nil:
		return nil, errors.Errorf(
			"getting unit uuid for unit name %q: %w", unitName, err,
		)
	}

	var storagePoolUUID *domainstorage.StoragePoolUUID
	if one.Directives.Pool != "" {
		poolUUID, err := a.storageService.GetStoragePoolUUID(
			ctx,
			one.Directives.Pool,
		)
		switch {
		case errors.Is(err, storageerrors.StoragePoolNameInvalid):
			return nil, apiservererrors.ParamsErrorf(params.CodeNotValid,
				"invalid storage pool name")
		case errors.Is(err, storageerrors.StoragePoolNotFound):
			return nil, apiservererrors.ParamsErrorf(params.CodeNotFound,
				"storage pool %q does not exist", one.Directives.Pool)
		case err != nil:
			return nil, errors.Errorf(
				"getting storage pool uuid for %q: %w", one.Directives.Pool, err,
			)
		}
		storagePoolUUID = &poolUUID
	}

	var storageCount uint32 = 1
	if one.Directives.Count != nil {
		if *one.Directives.Count > math.MaxUint32 {
			return nil, apiservererrors.ParamsErrorf(params.CodeNotValid,
				"storage directive %s count %d too large", one.StorageName, *one.Directives.Count,
			)
		}
		storageCount = uint32(*one.Directives.Count)
	}

	args := domainapplication.AddUnitStorageOverride{
		StoragePoolUUID: storagePoolUUID,
		SizeMiB:         one.Directives.SizeMiB,
	}
	var result []corestorage.ID
	if a.modelType == coremodel.CAAS {
		result, err = a.applicationService.AddStorageForCAASUnit(
			ctx, corestorage.Name(one.StorageName), unitUUID, storageCount, args,
		)
	} else {
		result, err = a.applicationService.AddStorageForIAASUnit(
			ctx, corestorage.Name(one.StorageName), unitUUID, storageCount, args,
		)
	}
	if err == nil {
		return result, nil
	}
	err = handleUnitAddStorageError(err, unitName, one)
	return nil, errors.Capture(err)
}

// handleUnitAddStorageError is a first low pass effort to start handling
// some of the errors that will occur when adding unit storage.
// If a handler does not exist then the original error will be returned.
func handleUnitAddStorageError(err error, unitName coreunit.Name, one params.StorageAddParams) error {
	switch {
	case errors.Is(err, applicationerrors.UnitNotFound):
		return apiservererrors.ParamsErrorf(params.CodeNotFound,
			"unit %q does not exist", unitName)
	case errors.Is(err, storageerrors.StoragePoolNotFound):
		return apiservererrors.ParamsErrorf(params.CodeNotFound,
			"storage pool %q does not exist", one.Directives.Pool)
	case errors.Is(err, corestorage.InvalidStorageName):
		return apiservererrors.ParamsErrorf(params.CodeNotValid,
			"invalid storage name %q", one.StorageName)
	case errors.Is(err, applicationerrors.StorageNameNotSupported):
		return apiservererrors.ParamsErrorf(params.CodeNotSupported,
			"storage name %q not supported by charm", one.StorageName)
		// When the supplied storage directive overrides violates the charm's
		// storage.
	case errors.HasType[applicationerrors.StorageCountLimitExceeded](err):
		limitErr, _ := errors.AsType[applicationerrors.StorageCountLimitExceeded](err)
		if limitErr.Maximum != nil && limitErr.Requested > *limitErr.Maximum {
			return apiservererrors.ParamsErrorf(params.CodeNotValid,
				"storage directive %q request count %d exceeds the charm's maximum count of %d",
				limitErr.StorageName, limitErr.Requested, *limitErr.Maximum,
			)
		}
	}
	return err
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
			return apiservererrors.ParamsErrorf(params.CodeNotValid, "invalid storage tag")
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
		return apiservererrors.ParamsErrorf(params.CodeNotValid,
			"max wait time cannot be a negative number",
		)
	}

	uuid, err := a.storageService.GetStorageInstanceUUIDForID(ctx, tag.Id())
	if errors.Is(err, storageerrors.StorageInstanceNotFound) {
		return apiservererrors.ParamsErrorf(params.CodeNotFound, "storage %q does not exist", tag.Id())
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
			return apiservererrors.ParamsErrorf(params.CodeNotFound, "storage %q does not exist", tag.Id())
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
		return apiservererrors.ParamsErrorf(params.CodeNotFound, "storage %q does not exist", tag.Id())
	} else if err != nil {
		return errors.Errorf("removing storage %q: %w", tag.Id(), err)
	}

	return nil
}

// Attach attaches existing storage instances to units.
// A "CHANGE" block can block this operation.
