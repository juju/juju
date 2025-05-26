// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storageservice "github.com/juju/juju/domain/storage/service"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// StorageService defines apis on the storage service.
type StorageService interface {
	CreateStoragePool(ctx context.Context, name string, providerType storage.ProviderType, attrs storageservice.PoolAttrs) error
	DeleteStoragePool(ctx context.Context, name string) error
	ReplaceStoragePool(ctx context.Context, name string, providerType storage.ProviderType, attrs storageservice.PoolAttrs) error
	ListStoragePools(ctx context.Context, filter domainstorage.Names, providers domainstorage.Providers) ([]*storage.Config, error)
	ImportProviderStorage(ctx context.Context, arg storageservice.ImportStorageParams) (corestorage.ID, error)
}

// ApplicationService is a subset of the [github.com/juju/juju/domain/application.Service] functionality
// related to attaching and detaching storage.
type ApplicationService interface {
	// GetUnitMachineName gets the name of the unit's machine.
	GetUnitMachineName(ctx context.Context, unitName unit.Name) (machine.Name, error)
	// AttachStorageToUnit attaches the specified storage to the specified unit.
	AttachStorageToUnit(ctx context.Context, storageID corestorage.ID, unitName unit.Name) error
	// DetachStorageForUnit detaches the specified storage from the specified unit.
	DetachStorageForUnit(ctx context.Context, storageID corestorage.ID, unitName unit.Name) error
	// DetachStorageFromUnit detaches the specified storage from whatever node it is attached to.
	DetachStorageFromUnit(ctx context.Context, storageID corestorage.ID) error
	// AddStorageForUnit adds storage instances to the given unit.
	AddStorageForUnit(ctx context.Context, storageName corestorage.Name, unitName unit.Name, stor storage.Directive) ([]corestorage.ID, error)
}

// StorageAPI implements the latest version (v6) of the Storage API.
type StorageAPI struct {
	storageAccess       storageAccess
	blockDeviceGetter   blockDeviceGetter
	storageService      StorageService
	applicationService  ApplicationService
	authorizer          facade.Authorizer
	blockCommandService common.BlockCommandService

	controllerUUID string
	modelUUID      coremodel.UUID
	modelType      coremodel.ModelType
}

func NewStorageAPI(
	controllerUUID string,
	modelUUID coremodel.UUID,
	modelType coremodel.ModelType,
	storageAccess storageAccess,
	blockDeviceGetter blockDeviceGetter,
	storageService StorageService,
	applicationService ApplicationService,
	authorizer facade.Authorizer,
	blockCommandService common.BlockCommandService,
) *StorageAPI {
	return &StorageAPI{
		controllerUUID:      controllerUUID,
		modelUUID:           modelUUID,
		modelType:           modelType,
		storageAccess:       storageAccess,
		blockDeviceGetter:   blockDeviceGetter,
		storageService:      storageService,
		applicationService:  applicationService,
		authorizer:          authorizer,
		blockCommandService: blockCommandService,
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
	if err := a.checkCanRead(ctx); err != nil {
		return params.StorageDetailsResults{}, errors.Trace(err)
	}
	results := make([]params.StorageDetailsResult, len(entities.Entities))
	for i, entity := range entities.Entities {
		storageTag, err := names.ParseStorageTag(entity.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		storageInstance, err := a.storageAccess.StorageInstance(storageTag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		details, err := storagecommon.StorageDetails(ctx, a.storageAccess, a.blockDeviceGetter, a.unitAssignedMachine, storageInstance)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Result = details
	}
	return params.StorageDetailsResults{Results: results}, nil
}

// ListStorageDetails returns storage matching a filter.
func (a *StorageAPI) ListStorageDetails(ctx context.Context, filters params.StorageFilters) (params.StorageDetailsListResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.StorageDetailsListResults{}, errors.Trace(err)
	}
	results := params.StorageDetailsListResults{
		Results: make([]params.StorageDetailsListResult, len(filters.Filters)),
	}
	for i, filter := range filters.Filters {
		list, err := a.listStorageDetails(ctx, filter)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = list
	}
	return results, nil
}

func (a *StorageAPI) listStorageDetails(ctx context.Context, filter params.StorageFilter) ([]params.StorageDetails, error) {
	if filter != (params.StorageFilter{}) {
		// StorageFilter has no fields at the time of writing, but
		// check that no fields are set in case we forget to update
		// this code.
		return nil, errors.NotSupportedf("storage filters")
	}
	stateInstances, err := a.storageAccess.AllStorageInstances()
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}
	results := make([]params.StorageDetails, len(stateInstances))
	for i, stateInstance := range stateInstances {
		details, err := storagecommon.StorageDetails(ctx, a.storageAccess, a.blockDeviceGetter, a.unitAssignedMachine, stateInstance)
		if err != nil {
			return nil, errors.Annotatef(
				err, "getting details for %s",
				names.ReadableString(stateInstance.Tag()),
			)
		}
		results[i] = *details
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
		pools, err := a.listPools(ctx, a.ensureStoragePoolFilter(filter))
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = pools
	}
	return results, nil
}

func (a *StorageAPI) ensureStoragePoolFilter(filter params.StoragePoolFilter) params.StoragePoolFilter {
	if a.modelType == coremodel.CAAS {
		filter.Providers = append(filter.Providers, k8sconstants.CAASProviderType)
	}
	return filter
}

func (a *StorageAPI) listPools(ctx context.Context, filter params.StoragePoolFilter) ([]params.StoragePool, error) {
	pools, err := a.storageService.ListStoragePools(ctx, filter.Names, filter.Providers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]params.StoragePool, len(pools))
	for i, p := range pools {
		results[i] = params.StoragePool{
			Name:     p.Name(),
			Provider: string(p.Provider()),
			Attrs:    p.Attrs(),
		}
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
	if err := a.checkCanRead(ctx); err != nil {
		return params.VolumeDetailsListResults{}, errors.Trace(err)
	}
	results := params.VolumeDetailsListResults{
		Results: make([]params.VolumeDetailsListResult, len(filters.Filters)),
	}
	for i, filter := range filters.Filters {
		volumes, volumeAttachments, err := filterVolumes(a.storageAccess, filter)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		details, err := a.createVolumeDetailsList(ctx, volumes, volumeAttachments)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = details
	}
	return results, nil
}

func filterVolumes(
	stVolume storageVolume,
	f params.VolumeFilter,
) ([]state.Volume, map[names.VolumeTag][]state.VolumeAttachment, error) {
	// Exit early if there's no volume support.
	if stVolume == nil {
		return nil, nil, nil
	}
	if f.IsEmpty() {
		// No filter was specified: get all volumes, and all attachments.
		volumes, err := stVolume.AllVolumes()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		volumeAttachments := make(map[names.VolumeTag][]state.VolumeAttachment)
		for _, v := range volumes {
			attachments, err := stVolume.VolumeAttachments(v.VolumeTag())
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			volumeAttachments[v.VolumeTag()] = attachments
		}
		return volumes, volumeAttachments, nil
	}
	volumesByTag := make(map[names.VolumeTag]state.Volume)
	volumeAttachments := make(map[names.VolumeTag][]state.VolumeAttachment)
	for _, machine := range f.Machines {
		machineTag, err := names.ParseMachineTag(machine)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		attachments, err := stVolume.MachineVolumeAttachments(machineTag)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		for _, attachment := range attachments {
			volumeTag := attachment.Volume()
			volumesByTag[volumeTag] = nil
			volumeAttachments[volumeTag] = append(volumeAttachments[volumeTag], attachment)
		}
	}
	for volumeTag := range volumesByTag {
		volume, err := stVolume.Volume(volumeTag)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		volumesByTag[volumeTag] = volume
	}
	volumes := make([]state.Volume, 0, len(volumesByTag))
	for _, volume := range volumesByTag {
		volumes = append(volumes, volume)
	}
	return volumes, volumeAttachments, nil
}

func (a *StorageAPI) createVolumeDetailsList(
	ctx context.Context,
	volumes []state.Volume,
	attachments map[names.VolumeTag][]state.VolumeAttachment,
) ([]params.VolumeDetails, error) {
	if len(volumes) == 0 {
		return nil, nil
	}
	results := make([]params.VolumeDetails, len(volumes))
	for i, v := range volumes {
		details, err := storagecommon.VolumeDetails(ctx, a.storageAccess, a.blockDeviceGetter, a.unitAssignedMachine, v, attachments[v.VolumeTag()])
		if err != nil {
			return nil, errors.Annotatef(
				err, "getting details for %s",
				names.ReadableString(v.VolumeTag()),
			)
		}
		results[i] = *details
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
	if err := a.checkCanRead(ctx); err != nil {
		return results, errors.Trace(err)
	}

	for i, filter := range filters.Filters {
		filesystems, filesystemAttachments, err := filterFilesystems(a.storageAccess, filter)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		details, err := a.createFilesystemDetailsList(ctx, filesystems, filesystemAttachments)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = details
	}
	return results, nil
}

func filterFilesystems(
	stFile storageFile,
	f params.FilesystemFilter,
) ([]state.Filesystem, map[names.FilesystemTag][]state.FilesystemAttachment, error) {
	if f.IsEmpty() {
		// No filter was specified: get all filesystems, and all attachments.
		filesystems, err := stFile.AllFilesystems()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		filesystemAttachments := make(map[names.FilesystemTag][]state.FilesystemAttachment)
		for _, f := range filesystems {
			attachments, err := stFile.FilesystemAttachments(f.FilesystemTag())
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			filesystemAttachments[f.FilesystemTag()] = attachments
		}
		return filesystems, filesystemAttachments, nil
	}
	filesystemsByTag := make(map[names.FilesystemTag]state.Filesystem)
	filesystemAttachments := make(map[names.FilesystemTag][]state.FilesystemAttachment)
	for _, machine := range f.Machines {
		machineTag, err := names.ParseMachineTag(machine)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		attachments, err := stFile.MachineFilesystemAttachments(machineTag)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		for _, attachment := range attachments {
			filesystemTag := attachment.Filesystem()
			filesystemsByTag[filesystemTag] = nil
			filesystemAttachments[filesystemTag] = append(filesystemAttachments[filesystemTag], attachment)
		}
	}
	for filesystemTag := range filesystemsByTag {
		filesystem, err := stFile.Filesystem(filesystemTag)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		filesystemsByTag[filesystemTag] = filesystem
	}
	filesystems := make([]state.Filesystem, 0, len(filesystemsByTag))
	for _, filesystem := range filesystemsByTag {
		filesystems = append(filesystems, filesystem)
	}
	return filesystems, filesystemAttachments, nil
}

func (a *StorageAPI) createFilesystemDetailsList(
	ctx context.Context,
	filesystems []state.Filesystem,
	attachments map[names.FilesystemTag][]state.FilesystemAttachment,
) ([]params.FilesystemDetails, error) {
	if len(filesystems) == 0 {
		return nil, nil
	}
	results := make([]params.FilesystemDetails, len(filesystems))
	for i, f := range filesystems {
		details, err := storagecommon.FilesystemDetails(ctx, a.storageAccess, a.blockDeviceGetter, a.unitAssignedMachine, f, attachments[f.FilesystemTag()])
		if err != nil {
			return nil, errors.Annotatef(
				err, "getting details for %s",
				names.ReadableString(f.FilesystemTag()),
			)
		}
		results[i] = *details
	}
	return results, nil
}

// AddToUnit validates and creates additional storage instances for units.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) AddToUnit(ctx context.Context, args params.StoragesAddParams) (params.AddStorageResults, error) {
	return a.addToUnit(ctx, args)
}

func (a *StorageAPI) addToUnit(ctx context.Context, args params.StoragesAddParams) (params.AddStorageResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.AddStorageResults{}, errors.Trace(err)
	}

	// Check if changes are allowed and the operation may proceed.
	blockChecker := common.NewBlockChecker(a.blockCommandService)
	if err := blockChecker.ChangeAllowed(ctx); err != nil {
		return params.AddStorageResults{}, errors.Trace(err)
	}

	paramsToStorage := func(p params.StorageDirectives) storage.Directive {
		s := storage.Directive{Pool: p.Pool}
		if p.Size != nil {
			s.Size = *p.Size
		}
		if p.Count != nil {
			s.Count = *p.Count
		}
		return s
	}

	result := make([]params.AddStorageResult, len(args.Storages))
	for i, one := range args.Storages {
		u, err := names.ParseUnitTag(one.UnitTag)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		storageIDs, err := a.applicationService.AddStorageForUnit(
			ctx, corestorage.Name(one.StorageName), unit.Name(u.Id()), paramsToStorage(one.Directives),
		)
		switch {
		case errors.Is(err, corestorage.InvalidStorageName):
			err = errors.NotValidf("storage name %q", one.StorageName)
		case errors.Is(err, storageerrors.StorageNotFound):
			err = errors.NotFoundf("storage %s", one.StorageName)
		case errors.Is(err, applicationerrors.UnitNotFound):
			err = errors.NotFoundf("unit %s", u.Id())
		}
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		tagStrings := make([]string, len(storageIDs))
		for i, id := range storageIDs {
			tagStrings[i] = names.NewStorageTag(string(id)).String()
		}
		result[i].Result = &params.AddStorageDetails{
			StorageTags: tagStrings,
		}
	}
	return params.AddStorageResults{Results: result}, nil
}

// Remove sets the specified storage entities to Dying, unless they are
// already Dying or Dead, such that the storage will eventually be removed
// from the model. If the arguments specify that the storage should be
// destroyed, then the associated cloud storage will be destroyed first;
// otherwise it will only be released from Juju's control.
func (a *StorageAPI) Remove(ctx context.Context, args params.RemoveStorage) (params.ErrorResults, error) {
	return a.remove(ctx, args)
}

func (a *StorageAPI) remove(ctx context.Context, args params.RemoveStorage) (params.ErrorResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	blockChecker := common.NewBlockChecker(a.blockCommandService)
	if err := blockChecker.RemoveAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	result := make([]params.ErrorResult, len(args.Storage))
	for i, arg := range args.Storage {
		tag, err := names.ParseStorageTag(arg.Tag)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		remove := a.storageAccess.DestroyStorageInstance
		if !arg.DestroyStorage {
			remove = a.storageAccess.ReleaseStorageInstance
		}
		force := arg.Force != nil && *arg.Force
		result[i].Error = apiservererrors.ServerError(remove(tag, arg.DestroyAttachments, force, common.MaxWait(arg.MaxWait)))
	}
	return params.ErrorResults{result}, nil
}

// DetachStorage sets the specified storage attachments to Dying, unless they are
// already Dying or Dead. Any associated, persistent storage will remain
// alive. This call can be forced.
func (a *StorageAPI) DetachStorage(ctx context.Context, args params.StorageDetachmentParams) (params.ErrorResults, error) {
	return a.internalDetach(ctx, args.StorageIds)
}

func (a *StorageAPI) internalDetach(ctx context.Context, args params.StorageAttachmentIds) (params.ErrorResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	blockChecker := common.NewBlockChecker(a.blockCommandService)
	if err := blockChecker.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	detachOne := func(arg params.StorageAttachmentId) error {
		storageTag, err := names.ParseStorageTag(arg.StorageTag)
		if err != nil {
			return err
		}
		var unitTag names.UnitTag
		if arg.UnitTag != "" {
			var err error
			unitTag, err = names.ParseUnitTag(arg.UnitTag)
			if err != nil {
				return err
			}
		}
		err = a.detachStorage(ctx, storageTag, unitTag)
		return errors.Trace(err)
	}

	result := make([]params.ErrorResult, len(args.Ids))
	for i, arg := range args.Ids {
		result[i].Error = apiservererrors.ServerError(detachOne(arg))
	}
	return params.ErrorResults{result}, nil
}

func (a *StorageAPI) detachStorage(ctx context.Context, storageTag names.StorageTag, unitTag names.UnitTag) error {
	var err error
	if unitTag != (names.UnitTag{}) {
		// The caller has specified a unit explicitly. Do
		// not filter out "not found" errors in this case.
		err = a.applicationService.DetachStorageForUnit(ctx, corestorage.ID(storageTag.Id()), unit.Name(unitTag.Id()))
	} else {
		err = a.applicationService.DetachStorageFromUnit(ctx, corestorage.ID(storageTag.Id()))
	}
	switch {
	case errors.Is(err, storageerrors.StorageNotFound):
		err = errors.NotFoundf("storage %s", storageTag.Id())
	case errors.Is(err, storageerrors.StorageAttachmentNotFound):
		err = errors.NotFoundf("attachment of storage %s to unit %s", storageTag.Id(), unitTag.Id())
	}
	return errors.Trace(err)
}

// Attach attaches existing storage instances to units.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Attach(ctx context.Context, args params.StorageAttachmentIds) (params.ErrorResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	blockChecker := common.NewBlockChecker(a.blockCommandService)
	if err := blockChecker.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	attachOne := func(arg params.StorageAttachmentId) error {
		storageTag, err := names.ParseStorageTag(arg.StorageTag)
		if err != nil {
			return err
		}
		unitTag, err := names.ParseUnitTag(arg.UnitTag)
		if err != nil {
			return err
		}
		err = a.applicationService.AttachStorageToUnit(ctx, corestorage.ID(storageTag.Id()), unit.Name(unitTag.Id()))
		switch {
		case errors.Is(err, storageerrors.StorageNotFound):
			err = errors.NotFoundf("storage %s", storageTag.Id())
		case errors.Is(err, applicationerrors.UnitNotFound):
			err = errors.NotFoundf("unit %s", unitTag.Id())
		}
		return errors.Trace(err)
	}

	result := make([]params.ErrorResult, len(args.Ids))
	for i, arg := range args.Ids {
		result[i].Error = apiservererrors.ServerError(attachOne(arg))
	}
	return params.ErrorResults{Results: result}, nil
}

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Import(ctx context.Context, args params.BulkImportStorageParams) (params.ImportStorageResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.ImportStorageResults{}, errors.Trace(err)
	}

	blockChecker := common.NewBlockChecker(a.blockCommandService)
	if err := blockChecker.ChangeAllowed(ctx); err != nil {
		return params.ImportStorageResults{}, errors.Trace(err)
	}

	results := make([]params.ImportStorageResult, len(args.Storage))
	for i, arg := range args.Storage {
		details, err := a.importStorage(ctx, arg)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Result = details
	}
	return params.ImportStorageResults{Results: results}, nil
}

func (a *StorageAPI) importStorage(ctx context.Context, arg params.ImportStorageParams) (*params.ImportStorageDetails, error) {
	storageName, err := corestorage.ParseName(arg.StorageName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	p := storageservice.ImportStorageParams{
		Kind:        storage.StorageKind(arg.Kind),
		Pool:        arg.Pool,
		ProviderId:  arg.ProviderId,
		StorageName: storageName,
	}
	storageID, err := a.storageService.ImportProviderStorage(ctx, p)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &params.ImportStorageDetails{
		StorageTag: names.NewStorageTag(string(storageID)).String(),
	}, nil
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

// unitAssignedMachine returns the tag of the machine that the unit
// is assigned to, or an error if the unit cannot be obtained or is
// not assigned to a machine.
func (a *StorageAPI) unitAssignedMachine(ctx context.Context, tag names.UnitTag) (names.MachineTag, error) {
	unitName, err := unit.NewName(tag.Id())
	if err != nil {
		return names.MachineTag{}, errors.Trace(err)
	}
	machineName, err := a.applicationService.GetUnitMachineName(ctx, unitName)
	if err != nil {
		return names.MachineTag{}, internalerrors.Errorf("getting machine name for unit %v: %w", unitName, err)
	}
	return names.NewMachineTag(machineName.String()), nil
}
