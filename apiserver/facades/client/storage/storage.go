// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

type storageMetadataFunc func() (poolmanager.PoolManager, storage.ProviderRegistry, error)

// StorageAPI implements the latest version (v6) of the Storage API.
type StorageAPI struct {
	backend         backend
	storageAccess   storageAccess
	storageMetadata storageMetadataFunc
	authorizer      facade.Authorizer
	callContext     context.ProviderCallContext
	modelType       state.ModelType
}

func NewStorageAPI(
	backend backend,
	modelType state.ModelType,
	storageAccess storageAccess,
	storageMetadata storageMetadataFunc,
	authorizer facade.Authorizer,
	callContext context.ProviderCallContext,
) *StorageAPI {
	return &StorageAPI{
		backend:         backend,
		modelType:       modelType,
		storageAccess:   storageAccess,
		storageMetadata: storageMetadata,
		authorizer:      authorizer,
		callContext:     callContext,
	}
}

func (a *StorageAPI) checkCanRead() error {
	err := a.authorizer.HasPermission(permission.SuperuserAccess, a.backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}
	return a.authorizer.HasPermission(permission.ReadAccess, a.backend.ModelTag())
}

func (a *StorageAPI) checkCanWrite() error {
	return a.authorizer.HasPermission(permission.WriteAccess, a.backend.ModelTag())
}

// StorageDetails retrieves and returns detailed information about desired
// storage identified by supplied tags. If specified storage cannot be
// retrieved, individual error is returned instead of storage information.
func (a *StorageAPI) StorageDetails(entities params.Entities) (params.StorageDetailsResults, error) {
	if err := a.checkCanRead(); err != nil {
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
		details, err := storagecommon.StorageDetails(a.storageAccess, a.unitAssignedMachine, storageInstance)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Result = details
	}
	return params.StorageDetailsResults{Results: results}, nil
}

// ListStorageDetails returns storage matching a filter.
func (a *StorageAPI) ListStorageDetails(filters params.StorageFilters) (params.StorageDetailsListResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.StorageDetailsListResults{}, errors.Trace(err)
	}
	results := params.StorageDetailsListResults{
		Results: make([]params.StorageDetailsListResult, len(filters.Filters)),
	}
	for i, filter := range filters.Filters {
		list, err := a.listStorageDetails(filter)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = list
	}
	return results, nil
}

func (a *StorageAPI) listStorageDetails(filter params.StorageFilter) ([]params.StorageDetails, error) {
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
		details, err := storagecommon.StorageDetails(a.storageAccess, a.unitAssignedMachine, stateInstance)
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
	filters params.StoragePoolFilters,
) (params.StoragePoolsResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.StoragePoolsResults{}, errors.Trace(err)
	}

	results := params.StoragePoolsResults{
		Results: make([]params.StoragePoolsResult, len(filters.Filters)),
	}
	for i, filter := range filters.Filters {
		pools, err := a.listPools(a.ensureStoragePoolFilter(filter))
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = pools
	}
	return results, nil
}

func (a *StorageAPI) ensureStoragePoolFilter(filter params.StoragePoolFilter) params.StoragePoolFilter {
	if a.modelType == state.ModelTypeCAAS {
		filter.Providers = append(filter.Providers, k8sconstants.CAASProviderType)
	}
	return filter
}

func (a *StorageAPI) listPools(filter params.StoragePoolFilter) ([]params.StoragePool, error) {
	pm, registry, err := a.storageMetadata()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := a.validatePoolListFilter(registry, filter); err != nil {
		return nil, errors.Trace(err)
	}

	pools, err := pm.List()
	if err != nil {
		return nil, errors.Trace(err)
	}
	providers, err := registry.StorageProviderTypes()
	if err != nil {
		return nil, errors.Trace(err)
	}
	matches := buildFilter(filter)
	results := append(
		filterPools(pools, matches),
		filterProviders(providers, matches)...,
	)
	return results, nil
}

func buildFilter(filter params.StoragePoolFilter) func(n, p string) bool {
	providerSet := set.NewStrings(filter.Providers...)
	nameSet := set.NewStrings(filter.Names...)

	matches := func(n, p string) bool {
		// no filters supplied = pool matches criteria
		if providerSet.IsEmpty() && nameSet.IsEmpty() {
			return true
		}
		// if at least 1 name and type are supplied, use AND to match
		if !providerSet.IsEmpty() && !nameSet.IsEmpty() {
			return nameSet.Contains(n) && providerSet.Contains(p)
		}
		// Otherwise, if only names or types are supplied, use OR to match
		return nameSet.Contains(n) || providerSet.Contains(p)
	}
	return matches
}

func filterProviders(
	providers []storage.ProviderType,
	matches func(n, p string) bool,
) []params.StoragePool {
	if len(providers) == 0 {
		return nil
	}
	all := make([]params.StoragePool, 0, len(providers))
	for _, p := range providers {
		ps := string(p)
		if matches(ps, ps) {
			all = append(all, params.StoragePool{Name: ps, Provider: ps})
		}
	}
	return all
}

func filterPools(
	pools []*storage.Config,
	matches func(n, p string) bool,
) []params.StoragePool {
	if len(pools) == 0 {
		return nil
	}
	all := make([]params.StoragePool, 0, len(pools))
	for _, p := range pools {
		if matches(p.Name(), string(p.Provider())) {
			all = append(all, params.StoragePool{
				Name:     p.Name(),
				Provider: string(p.Provider()),
				Attrs:    p.Attrs(),
			})
		}
	}
	return all
}

func (a *StorageAPI) validatePoolListFilter(registry storage.ProviderRegistry, filter params.StoragePoolFilter) error {
	if err := a.validateProviderCriteria(registry, filter.Providers); err != nil {
		return errors.Trace(err)
	}
	if err := a.validateNameCriteria(filter.Names); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (a *StorageAPI) validateNameCriteria(names []string) error {
	for _, n := range names {
		if !storage.IsValidPoolName(n) {
			return errors.NotValidf("pool name %q", n)
		}
	}
	return nil
}

func (a *StorageAPI) validateProviderCriteria(registry storage.ProviderRegistry, providers []string) error {
	for _, p := range providers {
		_, err := registry.StorageProvider(storage.ProviderType(p))
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// CreatePool creates a new pool with specified parameters.
func (a *StorageAPI) CreatePool(p params.StoragePoolArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(p.Pools)),
	}
	pm, _, err := a.storageMetadata()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, pool := range p.Pools {
		_, err := pm.Create(
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
func (a *StorageAPI) ListVolumes(filters params.VolumeFilters) (params.VolumeDetailsListResults, error) {
	if err := a.checkCanRead(); err != nil {
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
		details, err := a.createVolumeDetailsList(volumes, volumeAttachments)
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
	volumes []state.Volume,
	attachments map[names.VolumeTag][]state.VolumeAttachment,
) ([]params.VolumeDetails, error) {
	if len(volumes) == 0 {
		return nil, nil
	}
	results := make([]params.VolumeDetails, len(volumes))
	for i, v := range volumes {
		details, err := storagecommon.VolumeDetails(a.storageAccess, a.unitAssignedMachine, v, attachments[v.VolumeTag()])
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
func (a *StorageAPI) ListFilesystems(filters params.FilesystemFilters) (params.FilesystemDetailsListResults, error) {
	results := params.FilesystemDetailsListResults{
		Results: make([]params.FilesystemDetailsListResult, len(filters.Filters)),
	}
	if err := a.checkCanRead(); err != nil {
		return results, errors.Trace(err)
	}

	for i, filter := range filters.Filters {
		filesystems, filesystemAttachments, err := filterFilesystems(a.storageAccess, filter)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		details, err := a.createFilesystemDetailsList(filesystems, filesystemAttachments)
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
	filesystems []state.Filesystem,
	attachments map[names.FilesystemTag][]state.FilesystemAttachment,
) ([]params.FilesystemDetails, error) {
	if len(filesystems) == 0 {
		return nil, nil
	}
	results := make([]params.FilesystemDetails, len(filesystems))
	for i, f := range filesystems {
		details, err := storagecommon.FilesystemDetails(a.storageAccess, a.unitAssignedMachine, f, attachments[f.FilesystemTag()])
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
func (a *StorageAPI) AddToUnit(args params.StoragesAddParams) (params.AddStorageResults, error) {
	return a.addToUnit(args)
}

func (a *StorageAPI) addToUnit(args params.StoragesAddParams) (params.AddStorageResults, error) {
	if err := a.checkCanWrite(); err != nil {
		return params.AddStorageResults{}, errors.Trace(err)
	}

	// Check if changes are allowed and the operation may proceed.
	blockChecker := common.NewBlockChecker(a.backend)
	if err := blockChecker.ChangeAllowed(); err != nil {
		return params.AddStorageResults{}, errors.Trace(err)
	}

	paramsToState := func(p params.StorageConstraints) state.StorageConstraints {
		s := state.StorageConstraints{Pool: p.Pool}
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

		storageTags, err := a.storageAccess.AddStorageForUnit(
			u, one.StorageName, paramsToState(one.Constraints),
		)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
		}
		tagStrings := make([]string, len(storageTags))
		for i, tag := range storageTags {
			tagStrings[i] = tag.String()
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
func (a *StorageAPI) Remove(args params.RemoveStorage) (params.ErrorResults, error) {
	return a.remove(args)
}

func (a *StorageAPI) remove(args params.RemoveStorage) (params.ErrorResults, error) {
	if err := a.checkCanWrite(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	blockChecker := common.NewBlockChecker(a.backend)
	if err := blockChecker.RemoveAllowed(); err != nil {
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
func (a *StorageAPI) DetachStorage(args params.StorageDetachmentParams) (params.ErrorResults, error) {
	return a.internalDetach(args.StorageIds, args.Force, args.MaxWait)
}

func (a *StorageAPI) internalDetach(args params.StorageAttachmentIds, force *bool, maxWait *time.Duration) (params.ErrorResults, error) {
	if err := a.checkCanWrite(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	blockChecker := common.NewBlockChecker(a.backend)
	if err := blockChecker.ChangeAllowed(); err != nil {
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
		return a.detachStorage(storageTag, unitTag, force, maxWait)
	}

	result := make([]params.ErrorResult, len(args.Ids))
	for i, arg := range args.Ids {
		result[i].Error = apiservererrors.ServerError(detachOne(arg))
	}
	return params.ErrorResults{result}, nil
}

func (a *StorageAPI) detachStorage(storageTag names.StorageTag, unitTag names.UnitTag, force *bool, maxWait *time.Duration) error {
	forcing := force != nil && *force
	if unitTag != (names.UnitTag{}) {
		// The caller has specified a unit explicitly. Do
		// not filter out "not found" errors in this case.
		return a.storageAccess.DetachStorage(storageTag, unitTag, forcing, common.MaxWait(maxWait))
	}
	attachments, err := a.storageAccess.StorageAttachments(storageTag)
	if err != nil {
		return errors.Trace(err)
	}
	if len(attachments) == 0 {
		// No attachments: check if the storage exists at all.
		if _, err := a.storageAccess.StorageInstance(storageTag); err != nil {
			return errors.Trace(err)
		}
	}
	for _, att := range attachments {
		if att.Life() != state.Alive {
			continue
		}
		err := a.storageAccess.DetachStorage(storageTag, att.Unit(), forcing, common.MaxWait(maxWait))
		if err != nil && !errors.IsNotFound(err) {
			// We only care about NotFound errors if
			// the user specified a unit explicitly.
			return errors.Trace(err)
		}
	}
	return nil
}

// Attach attaches existing storage instances to units.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Attach(args params.StorageAttachmentIds) (params.ErrorResults, error) {
	if err := a.checkCanWrite(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	blockChecker := common.NewBlockChecker(a.backend)
	if err := blockChecker.ChangeAllowed(); err != nil {
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
		return a.storageAccess.AttachStorage(storageTag, unitTag)
	}

	result := make([]params.ErrorResult, len(args.Ids))
	for i, arg := range args.Ids {
		result[i].Error = apiservererrors.ServerError(attachOne(arg))
	}
	return params.ErrorResults{Results: result}, nil
}

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Import(args params.BulkImportStorageParams) (params.ImportStorageResults, error) {
	if err := a.checkCanWrite(); err != nil {
		return params.ImportStorageResults{}, errors.Trace(err)
	}

	blockChecker := common.NewBlockChecker(a.backend)
	if err := blockChecker.ChangeAllowed(); err != nil {
		return params.ImportStorageResults{}, errors.Trace(err)
	}

	results := make([]params.ImportStorageResult, len(args.Storage))
	for i, arg := range args.Storage {
		details, err := a.importStorage(arg)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Result = details
	}
	return params.ImportStorageResults{Results: results}, nil
}

func (a *StorageAPI) importStorage(arg params.ImportStorageParams) (*params.ImportStorageDetails, error) {
	if arg.Kind != params.StorageKindFilesystem {
		// TODO(axw) implement support for volumes.
		return nil, errors.NotSupportedf("storage kind %q", arg.Kind.String())
	}
	if !storage.IsValidPoolName(arg.Pool) {
		return nil, errors.NotValidf("pool name %q", arg.Pool)
	}

	pm, registry, err := a.storageMetadata()
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg, err := pm.Get(arg.Pool)
	if errors.IsNotFound(err) {
		cfg, err = storage.NewConfig(
			arg.Pool,
			storage.ProviderType(arg.Pool),
			map[string]interface{}{},
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	provider, err := registry.StorageProvider(cfg.Provider())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return a.importFilesystem(arg, provider, cfg)
}

func (a *StorageAPI) importFilesystem(
	arg params.ImportStorageParams,
	provider storage.Provider,
	cfg *storage.Config,
) (*params.ImportStorageDetails, error) {
	resourceTags := map[string]string{
		tags.JujuModel:      a.backend.ModelTag().Id(),
		tags.JujuController: a.backend.ControllerTag().Id(),
	}
	var volumeInfo *state.VolumeInfo
	filesystemInfo := state.FilesystemInfo{Pool: arg.Pool}

	// If the storage provider supports filesystems, import the filesystem,
	// otherwise import a volume which will back a filesystem.
	if provider.Supports(storage.StorageKindFilesystem) {
		filesystemSource, err := provider.FilesystemSource(cfg)
		if err != nil {
			return nil, errors.Trace(err)
		}
		filesystemImporter, ok := filesystemSource.(storage.FilesystemImporter)
		if !ok {
			return nil, errors.NotSupportedf(
				"importing filesystem with storage provider %q",
				cfg.Provider(),
			)
		}
		info, err := filesystemImporter.ImportFilesystem(a.callContext, arg.ProviderId, resourceTags)
		if err != nil {
			return nil, errors.Annotate(err, "importing filesystem")
		}
		filesystemInfo.FilesystemId = arg.ProviderId
		filesystemInfo.Size = info.Size
	} else {
		volumeSource, err := provider.VolumeSource(cfg)
		if err != nil {
			return nil, errors.Trace(err)
		}
		volumeImporter, ok := volumeSource.(storage.VolumeImporter)
		if !ok {
			return nil, errors.NotSupportedf(
				"importing volume with storage provider %q",
				cfg.Provider(),
			)
		}
		// Support import-filesystem on k8s only available when featureflag K8SAttachStorage enabled.
		if cfg.Provider() == k8sconstants.StorageProviderType && !featureflag.Enabled(feature.K8SAttachStorage) {
			return nil, errors.NotSupportedf(
				"importing volume with storage provider %q",
				cfg.Provider(),
			)
		}
		info, err := volumeImporter.ImportVolume(a.callContext, arg.ProviderId, resourceTags)
		if err != nil {
			return nil, errors.Annotate(err, "importing volume")
		}
		volumeInfo = &state.VolumeInfo{
			HardwareId: info.HardwareId,
			WWN:        info.WWN,
			Size:       info.Size,
			Pool:       arg.Pool,
			VolumeId:   info.VolumeId,
			Persistent: info.Persistent,
		}
		filesystemInfo.Size = info.Size
	}

	storageTag, err := a.storageAccess.AddExistingFilesystem(filesystemInfo, volumeInfo, arg.StorageName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &params.ImportStorageDetails{
		StorageTag: storageTag.String(),
	}, nil
}

// RemovePool deletes the named pool
func (a *StorageAPI) RemovePool(p params.StoragePoolDeleteArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(p.Pools)),
	}
	if err := a.checkCanWrite(); err != nil {
		return results, errors.Trace(err)
	}
	for i, pool := range p.Pools {
		err := a.storageAccess.RemoveStoragePool(pool.Name)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// UpdatePool deletes the named pool
func (a *StorageAPI) UpdatePool(p params.StoragePoolArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(p.Pools)),
	}
	if err := a.checkCanWrite(); err != nil {
		return results, errors.Trace(err)
	}
	pm, _, err := a.storageMetadata()
	if err != nil {
		return results, errors.Trace(err)
	}

	for i, pool := range p.Pools {
		err := pm.Replace(pool.Name, pool.Provider, pool.Attrs)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// unitAssignedMachine returns the tag of the machine that the unit
// is assigned to, or an error if the unit cannot be obtained or is
// not assigned to a machine.
func (a *StorageAPI) unitAssignedMachine(tag names.UnitTag) (names.MachineTag, error) {
	unit, err := a.backend.Unit(tag.Id())
	if err != nil {
		return names.MachineTag{}, errors.Trace(err)
	}
	mid, err := unit.AssignedMachineId()
	if err != nil {
		return names.MachineTag{}, errors.Trace(err)
	}
	return names.NewMachineTag(mid), nil
}
