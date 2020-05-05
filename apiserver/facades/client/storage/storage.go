// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package storage provides an API server facade for managing
// storage entities.
package storage

import (
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

// StorageAPI implements the latest version (v6) of the Storage API.
type StorageAPI struct {
	backend       backend
	storageAccess storageAccess
	registry      storage.ProviderRegistry
	poolManager   poolmanager.PoolManager
	authorizer    facade.Authorizer
	callContext   context.ProviderCallContext
	modelType     state.ModelType
}

// APIv5 implements the storage v5 API.
type StorageAPIv5 struct {
	StorageAPI
}

// APIv4 implements the storage v4 API adding AddToUnit, Import and Remove (replacing Destroy)
type StorageAPIv4 struct {
	StorageAPIv5
}

// APIv3 implements the storage v3 API.
type StorageAPIv3 struct {
	StorageAPIv4
}

// NewStorageAPI returns a new storage API facade.
func NewStorageAPI(ctx facade.Context) (*StorageAPI, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	registry, err := stateenvirons.NewStorageProviderRegistryForModel(
		model,
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New))
	if err != nil {
		return nil, errors.Trace(err)
	}
	pm := poolmanager.New(state.NewStateSettings(st), registry)

	storageAccessor, err := getStorageAccessor(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting backend")
	}

	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return newStorageAPI(stateShim{st}, model.Type(), storageAccessor, registry, pm, authorizer, context.CallContext(st)), nil
}

func newStorageAPI(
	backend backend,
	modelType state.ModelType,
	storageAccess storageAccess,
	registry storage.ProviderRegistry,
	pm poolmanager.PoolManager,
	authorizer facade.Authorizer,
	callContext context.ProviderCallContext,
) *StorageAPI {
	return &StorageAPI{
		backend:       backend,
		modelType:     modelType,
		storageAccess: storageAccess,
		registry:      registry,
		poolManager:   pm,
		authorizer:    authorizer,
		callContext:   callContext,
	}
}

// NewStorageAPIV5 returns a new storage v5 API facade.
func NewStorageAPIV5(context facade.Context) (*StorageAPIv5, error) {
	storageAPI, err := NewStorageAPI(context)
	if err != nil {
		return nil, err
	}
	return &StorageAPIv5{
		StorageAPI: *storageAPI,
	}, nil
}

// NewStorageAPIV4 returns a new storage v4 API facade.
func NewStorageAPIV4(context facade.Context) (*StorageAPIv4, error) {
	storageAPI, err := NewStorageAPIV5(context)
	if err != nil {
		return nil, err
	}
	return &StorageAPIv4{
		StorageAPIv5: *storageAPI,
	}, nil
}

// NewStorageAPIV3 returns a new storage v3 API facade.
func NewStorageAPIV3(context facade.Context) (*StorageAPIv3, error) {
	storageAPI, err := NewStorageAPIV4(context)
	if err != nil {
		return nil, err
	}
	return &StorageAPIv3{
		StorageAPIv4: *storageAPI,
	}, nil
}

func (a *StorageAPI) checkCanRead() error {
	canRead, err := a.authorizer.HasPermission(permission.ReadAccess, a.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}

	// Controller admins should have implicit read access to all models
	// in the controller, even the ones they do not own.
	if !canRead {
		canRead, _ = a.authorizer.HasPermission(permission.SuperuserAccess, a.backend.ControllerTag())
	}

	if !canRead {
		return common.ErrPerm
	}
	return nil
}

func (a *StorageAPI) checkCanWrite() error {
	canWrite, err := a.authorizer.HasPermission(permission.WriteAccess, a.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canWrite {
		return common.ErrPerm
	}
	return nil
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
			results[i].Error = common.ServerError(err)
			continue
		}
		storageInstance, err := a.storageAccess.StorageInstance(storageTag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		details, err := createStorageDetails(a.backend, a.storageAccess, storageInstance)
		if err != nil {
			results[i].Error = common.ServerError(err)
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
			results.Results[i].Error = common.ServerError(err)
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
		return nil, common.ServerError(err)
	}
	results := make([]params.StorageDetails, len(stateInstances))
	for i, stateInstance := range stateInstances {
		details, err := createStorageDetails(a.backend, a.storageAccess, stateInstance)
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

func createStorageDetails(
	backend backend,
	st storageAccess,
	si state.StorageInstance,
) (*params.StorageDetails, error) {
	// Get information from underlying volume or filesystem.
	var persistent bool
	var statusEntity status.StatusGetter
	if si.Kind() == state.StorageKindFilesystem {
		stFile := st.FilesystemAccess()
		if stFile == nil {
			return nil, errors.NotImplementedf("FilesystemStorage instance")
		}
		// TODO(axw) when we support persistent filesystems,
		// e.g. CephFS, we'll need to do set "persistent"
		// here too.
		filesystem, err := stFile.StorageInstanceFilesystem(si.StorageTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		statusEntity = filesystem
	} else {
		stVolume := st.VolumeAccess()
		if stVolume == nil {
			return nil, errors.NotImplementedf("BlockStorage instance")
		}
		volume, err := stVolume.StorageInstanceVolume(si.StorageTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if info, err := volume.Info(); err == nil {
			persistent = info.Persistent
		}
		statusEntity = volume
	}
	aStatus, err := statusEntity.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Get unit storage attachments.
	var storageAttachmentDetails map[string]params.StorageAttachmentDetails
	storageAttachments, err := st.StorageAttachments(si.StorageTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(storageAttachments) > 0 {
		storageAttachmentDetails = make(map[string]params.StorageAttachmentDetails)
		for _, a := range storageAttachments {
			// TODO(caas) - handle attachments to units
			machineTag, location, err := storageAttachmentInfo(backend, st, a)
			if err != nil {
				return nil, errors.Trace(err)
			}
			details := params.StorageAttachmentDetails{
				StorageTag: a.StorageInstance().String(),
				UnitTag:    a.Unit().String(),
				Location:   location,
				Life:       life.Value(a.Life().String()),
			}
			if machineTag.Id() != "" {
				details.MachineTag = machineTag.String()
			}
			storageAttachmentDetails[a.Unit().String()] = details
		}
	}

	var ownerTag string
	if owner, ok := si.Owner(); ok {
		ownerTag = owner.String()
	}

	return &params.StorageDetails{
		StorageTag:  si.Tag().String(),
		OwnerTag:    ownerTag,
		Kind:        params.StorageKind(si.Kind()),
		Life:        life.Value(si.Life().String()),
		Status:      common.EntityStatusFromState(aStatus),
		Persistent:  persistent,
		Attachments: storageAttachmentDetails,
	}, nil
}

func storageAttachmentInfo(
	backend backend,
	st storageAccess,
	a state.StorageAttachment,
) (_ names.MachineTag, location string, _ error) {
	machineTag, err := unitAssignedMachine(backend, a.Unit())
	if errors.IsNotAssigned(err) {
		return names.MachineTag{}, "", nil
	} else if err != nil {
		return names.MachineTag{}, "", errors.Trace(err)
	}
	info, err := storagecommon.StorageAttachmentInfo(st, st.VolumeAccess(), st.FilesystemAccess(), a, machineTag)
	if errors.IsNotProvisioned(err) {
		return machineTag, "", nil
	} else if err != nil {
		return names.MachineTag{}, "", errors.Trace(err)
	}
	return machineTag, info.Location, nil
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = pools
	}
	return results, nil
}

func (a *StorageAPI) ensureStoragePoolFilter(filter params.StoragePoolFilter) params.StoragePoolFilter {
	if a.modelType == state.ModelTypeCAAS {
		filter.Providers = append(filter.Providers, k8sprovider.CAASProviderType)
	}
	return filter
}

func (a *StorageAPI) listPools(filter params.StoragePoolFilter) ([]params.StoragePool, error) {
	if err := a.validatePoolListFilter(filter); err != nil {
		return nil, errors.Trace(err)
	}
	pools, err := a.poolManager.List()
	if err != nil {
		return nil, errors.Trace(err)
	}
	providers, err := a.registry.StorageProviderTypes()
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

func (a *StorageAPI) validatePoolListFilter(filter params.StoragePoolFilter) error {
	if err := a.validateProviderCriteria(filter.Providers); err != nil {
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

func (a *StorageAPI) validateProviderCriteria(providers []string) error {
	for _, p := range providers {
		_, err := a.registry.StorageProvider(storage.ProviderType(p))
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// CreatePool creates a new pool with specified parameters.
func (a *StorageAPIv4) CreatePool(p params.StoragePool) error {
	_, err := a.poolManager.Create(
		p.Name,
		storage.ProviderType(p.Provider),
		p.Attrs)
	return err
}

// CreatePool creates a new pool with specified parameters.
func (a *StorageAPI) CreatePool(p params.StoragePoolArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(p.Pools)),
	}
	for i, pool := range p.Pools {
		_, err := a.poolManager.Create(
			pool.Name,
			storage.ProviderType(pool.Provider),
			pool.Attrs)
		results.Results[i].Error = common.ServerError(err)
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
	stVolumeAccess := a.storageAccess.VolumeAccess()
	for i, filter := range filters.Filters {
		volumes, volumeAttachments, err := filterVolumes(stVolumeAccess, filter)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		details, err := createVolumeDetailsList(
			a.backend, a.storageAccess, volumes, volumeAttachments,
		)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
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

func createVolumeDetailsList(
	backend backend,
	st storageAccess,
	volumes []state.Volume,
	attachments map[names.VolumeTag][]state.VolumeAttachment,
) ([]params.VolumeDetails, error) {

	if len(volumes) == 0 {
		return nil, nil
	}
	results := make([]params.VolumeDetails, len(volumes))
	for i, v := range volumes {
		details, err := createVolumeDetails(backend, st, v, attachments[v.VolumeTag()])
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

func createVolumeDetails(
	backend backend,
	st storageAccess,
	v state.Volume,
	attachments []state.VolumeAttachment,
) (*params.VolumeDetails, error) {

	details := &params.VolumeDetails{
		VolumeTag: v.VolumeTag().String(),
		Life:      life.Value(v.Life().String()),
	}

	if info, err := v.Info(); err == nil {
		details.Info = storagecommon.VolumeInfoFromState(info)
	}

	if len(attachments) > 0 {
		details.MachineAttachments = make(map[string]params.VolumeAttachmentDetails, len(attachments))
		details.UnitAttachments = make(map[string]params.VolumeAttachmentDetails, len(attachments))
		for _, attachment := range attachments {
			attDetails := params.VolumeAttachmentDetails{
				Life: life.Value(attachment.Life().String()),
			}
			if stateInfo, err := attachment.Info(); err == nil {
				attDetails.VolumeAttachmentInfo = storagecommon.VolumeAttachmentInfoFromState(
					stateInfo,
				)
			}
			if attachment.Host().Kind() == names.MachineTagKind {
				details.MachineAttachments[attachment.Host().String()] = attDetails
			} else {
				details.UnitAttachments[attachment.Host().String()] = attDetails
			}
		}
	}

	aStatus, err := v.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}
	details.Status = common.EntityStatusFromState(aStatus)

	if storageTag, err := v.StorageInstance(); err == nil {
		storageInstance, err := st.StorageInstance(storageTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		storageDetails, err := createStorageDetails(backend, st, storageInstance)
		if err != nil {
			return nil, errors.Trace(err)
		}
		details.Storage = storageDetails
	}

	return details, nil
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

	stFileAccess := a.storageAccess.FilesystemAccess()
	for i, filter := range filters.Filters {
		filesystems, filesystemAttachments, err := filterFilesystems(stFileAccess, filter)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		details, err := createFilesystemDetailsList(
			a.backend, a.storageAccess, filesystems, filesystemAttachments,
		)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
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

func createFilesystemDetailsList(
	backend backend,
	st storageAccess,
	filesystems []state.Filesystem,
	attachments map[names.FilesystemTag][]state.FilesystemAttachment,
) ([]params.FilesystemDetails, error) {

	if len(filesystems) == 0 {
		return nil, nil
	}
	results := make([]params.FilesystemDetails, len(filesystems))
	for i, f := range filesystems {
		details, err := createFilesystemDetails(backend, st, f, attachments[f.FilesystemTag()])
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

func createFilesystemDetails(
	backend backend,
	st storageAccess,
	f state.Filesystem,
	attachments []state.FilesystemAttachment,
) (*params.FilesystemDetails, error) {

	details := &params.FilesystemDetails{
		FilesystemTag: f.FilesystemTag().String(),
		Life:          life.Value(f.Life().String()),
	}

	if volumeTag, err := f.Volume(); err == nil {
		details.VolumeTag = volumeTag.String()
	}

	if info, err := f.Info(); err == nil {
		details.Info = storagecommon.FilesystemInfoFromState(info)
	}

	if len(attachments) > 0 {
		details.MachineAttachments = make(map[string]params.FilesystemAttachmentDetails, len(attachments))
		details.UnitAttachments = make(map[string]params.FilesystemAttachmentDetails, len(attachments))
		for _, attachment := range attachments {
			attDetails := params.FilesystemAttachmentDetails{
				Life: life.Value(attachment.Life().String()),
			}
			if stateInfo, err := attachment.Info(); err == nil {
				attDetails.FilesystemAttachmentInfo = storagecommon.FilesystemAttachmentInfoFromState(
					stateInfo,
				)
			}
			if attachment.Host().Kind() == names.MachineTagKind {
				details.MachineAttachments[attachment.Host().String()] = attDetails
			} else {
				details.UnitAttachments[attachment.Host().String()] = attDetails
			}
		}
	}

	aStatus, err := f.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}
	details.Status = common.EntityStatusFromState(aStatus)

	if storageTag, err := f.Storage(); err == nil {
		storageInstance, err := st.StorageInstance(storageTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		storageDetails, err := createStorageDetails(backend, st, storageInstance)
		if err != nil {
			return nil, errors.Trace(err)
		}
		details.Storage = storageDetails
	}

	return details, nil
}

// AddToUnit validates and creates additional storage instances for units.
// A "CHANGE" block can block this operation.
func (a *StorageAPIv3) AddToUnit(args params.StoragesAddParams) (params.ErrorResults, error) {
	v4results, err := a.addToUnit(args)
	if err != nil {
		return params.ErrorResults{}, err
	}
	v3results := make([]params.ErrorResult, len(v4results.Results))
	for i, result := range v4results.Results {
		v3results[i].Error = result.Error
	}
	return params.ErrorResults{v3results}, nil
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
			result[i].Error = common.ServerError(err)
			continue
		}

		storageTags, err := a.storageAccess.AddStorageForUnit(
			u, one.StorageName, paramsToState(one.Constraints),
		)
		if err != nil {
			result[i].Error = common.ServerError(err)
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
			result[i].Error = common.ServerError(err)
			continue
		}
		remove := a.storageAccess.DestroyStorageInstance
		if !arg.DestroyStorage {
			remove = a.storageAccess.ReleaseStorageInstance
		}
		force := arg.Force != nil && *arg.Force
		result[i].Error = common.ServerError(remove(tag, arg.DestroyAttachments, force, common.MaxWait(arg.MaxWait)))
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
		result[i].Error = common.ServerError(detachOne(arg))
	}
	return params.ErrorResults{result}, nil
}

// Detach sets the specified storage attachments to Dying, unless they are
// already Dying or Dead. Any associated, persistent storage will remain
// alive.
func (a *StorageAPIv5) Detach(args params.StorageAttachmentIds) (params.ErrorResults, error) {
	return a.internalDetach(args, nil, nil)
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
		return a.attachStorage(storageTag, unitTag)
	}

	result := make([]params.ErrorResult, len(args.Ids))
	for i, arg := range args.Ids {
		result[i].Error = common.ServerError(attachOne(arg))
	}
	return params.ErrorResults{Results: result}, nil
}

func (a *StorageAPI) attachStorage(storageTag names.StorageTag, unitTag names.UnitTag) error {
	return a.storageAccess.AttachStorage(storageTag, unitTag)
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
			results[i].Error = common.ServerError(err)
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

	cfg, err := a.poolManager.Get(arg.Pool)
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
	provider, err := a.registry.StorageProvider(cfg.Provider())
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

	storageTag, err := a.storageAccess.FilesystemAccess().AddExistingFilesystem(filesystemInfo, volumeInfo, arg.StorageName)
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
			results.Results[i].Error = common.ServerError(err)
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
	for i, pool := range p.Pools {
		err := a.poolManager.Replace(pool.Name, pool.Provider, pool.Attrs)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}

// Mask out old methods from the new API versions. The API reflection
// code in rpc/rpcreflect/type.go:newMethod skips 2-argument methods,
// so this removes the method as far as the RPC machinery is concerned.

// Added in v6 api version
func (*StorageAPIv5) DetachStorage(_, _ struct{}) {}

// Added in v5 api version
func (*StorageAPIv4) RemovePool(_, _ struct{}) {}
func (*StorageAPIv4) UpdatePool(_, _ struct{}) {}

// Added in v4
// Destroy was dropped in V4, replaced with Remove.
func (*StorageAPIv3) Remove(_, _ struct{})           {}
func (*StorageAPIv3) Import(_, _ struct{})           {}
func (*StorageAPIv3) importStorage(_, _ struct{})    {}
func (*StorageAPIv3) importFilesystem(_, _ struct{}) {}

// Destroy sets the specified storage entities to Dying, unless they are
// already Dying or Dead.
func (a *StorageAPIv3) Destroy(args params.Entities) (params.ErrorResults, error) {
	v4Args := params.RemoveStorage{
		Storage: make([]params.RemoveStorageInstance, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		v4Args.Storage[i] = params.RemoveStorageInstance{
			Tag: arg.Tag,
			// The v3 behaviour was to detach the storage
			// at the same time as marking the storage Dying.
			DestroyAttachments: true,
			DestroyStorage:     true,
		}
	}
	return a.remove(v4Args)
}
