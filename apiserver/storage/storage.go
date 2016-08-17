// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package storage provides an API server facade for managing
// storage entities.
package storage

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

// API implements the storage interface and is the concrete
// implementation of the api end point.
type API struct {
	storage     storageAccess
	registry    storage.ProviderRegistry
	poolManager poolmanager.PoolManager
	authorizer  facade.Authorizer
}

// NewAPI returns a new storage API facade.
func NewAPI(
	st storageAccess,
	registry storage.ProviderRegistry,
	pm poolmanager.PoolManager,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		storage:     st,
		registry:    registry,
		poolManager: pm,
		authorizer:  authorizer,
	}, nil
}
func (api *API) checkCanRead() error {
	canRead, err := api.authorizer.HasPermission(description.ReadAccess, api.storage.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return common.ErrPerm
	}
	return nil
}

func (api *API) checkCanWrite() error {
	canWrite, err := api.authorizer.HasPermission(description.WriteAccess, api.storage.ModelTag())
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
func (api *API) StorageDetails(entities params.Entities) (params.StorageDetailsResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.StorageDetailsResults{}, errors.Trace(err)
	}
	results := make([]params.StorageDetailsResult, len(entities.Entities))
	for i, entity := range entities.Entities {
		storageTag, err := names.ParseStorageTag(entity.Tag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		storageInstance, err := api.storage.StorageInstance(storageTag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		details, err := createStorageDetails(api.storage, storageInstance)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Result = details
	}
	return params.StorageDetailsResults{Results: results}, nil
}

// ListStorageDetails returns storage matching a filter.
func (api *API) ListStorageDetails(filters params.StorageFilters) (params.StorageDetailsListResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.StorageDetailsListResults{}, errors.Trace(err)
	}
	results := params.StorageDetailsListResults{
		Results: make([]params.StorageDetailsListResult, len(filters.Filters)),
	}
	for i, filter := range filters.Filters {
		list, err := api.listStorageDetails(filter)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = list
	}
	return results, nil
}

func (api *API) listStorageDetails(filter params.StorageFilter) ([]params.StorageDetails, error) {
	if filter != (params.StorageFilter{}) {
		// StorageFilter has no fields at the time of writing, but
		// check that no fields are set in case we forget to update
		// this code.
		return nil, errors.NotSupportedf("storage filters")
	}
	stateInstances, err := api.storage.AllStorageInstances()
	if err != nil {
		return nil, common.ServerError(err)
	}
	results := make([]params.StorageDetails, len(stateInstances))
	for i, stateInstance := range stateInstances {
		details, err := createStorageDetails(api.storage, stateInstance)
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

func createStorageDetails(st storageAccess, si state.StorageInstance) (*params.StorageDetails, error) {
	// Get information from underlying volume or filesystem.
	var persistent bool
	var statusEntity status.StatusGetter
	if si.Kind() != state.StorageKindBlock {
		// TODO(axw) when we support persistent filesystems,
		// e.g. CephFS, we'll need to do set "persistent"
		// here too.
		filesystem, err := st.StorageInstanceFilesystem(si.StorageTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		statusEntity = filesystem
	} else {
		volume, err := st.StorageInstanceVolume(si.StorageTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if info, err := volume.Info(); err == nil {
			persistent = info.Persistent
		}
		statusEntity = volume
	}
	status, err := statusEntity.Status()
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
			machineTag, location, err := storageAttachmentInfo(st, a)
			if err != nil {
				return nil, errors.Trace(err)
			}
			details := params.StorageAttachmentDetails{
				a.StorageInstance().String(),
				a.Unit().String(),
				machineTag.String(),
				location,
			}
			storageAttachmentDetails[a.Unit().String()] = details
		}
	}

	return &params.StorageDetails{
		StorageTag:  si.Tag().String(),
		OwnerTag:    si.Owner().String(),
		Kind:        params.StorageKind(si.Kind()),
		Status:      common.EntityStatusFromState(status),
		Persistent:  persistent,
		Attachments: storageAttachmentDetails,
	}, nil
}

func storageAttachmentInfo(st storageAccess, a state.StorageAttachment) (_ names.MachineTag, location string, _ error) {
	machineTag, err := st.UnitAssignedMachine(a.Unit())
	if errors.IsNotAssigned(err) {
		return names.MachineTag{}, "", nil
	} else if err != nil {
		return names.MachineTag{}, "", errors.Trace(err)
	}
	info, err := storagecommon.StorageAttachmentInfo(st, a, machineTag)
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
func (a *API) ListPools(
	filters params.StoragePoolFilters,
) (params.StoragePoolsResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.StoragePoolsResults{}, errors.Trace(err)
	}

	results := params.StoragePoolsResults{
		Results: make([]params.StoragePoolsResult, len(filters.Filters)),
	}
	for i, filter := range filters.Filters {
		pools, err := a.listPools(filter)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = pools
	}
	return results, nil
}

func (a *API) listPools(filter params.StoragePoolFilter) ([]params.StoragePool, error) {
	if err := a.validatePoolListFilter(filter); err != nil {
		return nil, errors.Trace(err)
	}
	pools, err := a.poolManager.List()
	if err != nil {
		return nil, errors.Trace(err)
	}
	providers := a.registry.StorageProviderTypes()
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
			return nameSet.Contains(n) && providerSet.Contains(string(p))
		}
		// Otherwise, if only names or types are supplied, use OR to match
		return nameSet.Contains(n) || providerSet.Contains(string(p))
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

func (a *API) validatePoolListFilter(filter params.StoragePoolFilter) error {
	if err := a.validateProviderCriteria(filter.Providers); err != nil {
		return errors.Trace(err)
	}
	if err := a.validateNameCriteria(filter.Names); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (a *API) validateNameCriteria(names []string) error {
	for _, n := range names {
		if !storage.IsValidPoolName(n) {
			return errors.NotValidf("pool name %q", n)
		}
	}
	return nil
}

func (a *API) validateProviderCriteria(providers []string) error {
	for _, p := range providers {
		_, err := a.registry.StorageProvider(storage.ProviderType(p))
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// CreatePool creates a new pool with specified parameters.
func (a *API) CreatePool(p params.StoragePool) error {
	_, err := a.poolManager.Create(
		p.Name,
		storage.ProviderType(p.Provider),
		p.Attrs)
	return err
}

// ListVolumes lists volumes with the given filters. Each filter produces
// an independent list of volumes, or an error if the filter is invalid
// or the volumes could not be listed.
func (a *API) ListVolumes(filters params.VolumeFilters) (params.VolumeDetailsListResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.VolumeDetailsListResults{}, errors.Trace(err)
	}
	results := params.VolumeDetailsListResults{
		Results: make([]params.VolumeDetailsListResult, len(filters.Filters)),
	}
	for i, filter := range filters.Filters {
		volumes, volumeAttachments, err := filterVolumes(a.storage, filter)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		details, err := createVolumeDetailsList(
			a.storage, volumes, volumeAttachments,
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
	st storageAccess,
	f params.VolumeFilter,
) ([]state.Volume, map[names.VolumeTag][]state.VolumeAttachment, error) {
	if f.IsEmpty() {
		// No filter was specified: get all volumes, and all attachments.
		volumes, err := st.AllVolumes()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		volumeAttachments := make(map[names.VolumeTag][]state.VolumeAttachment)
		for _, v := range volumes {
			attachments, err := st.VolumeAttachments(v.VolumeTag())
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
		attachments, err := st.MachineVolumeAttachments(machineTag)
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
		volume, err := st.Volume(volumeTag)
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
	st storageAccess,
	volumes []state.Volume,
	attachments map[names.VolumeTag][]state.VolumeAttachment,
) ([]params.VolumeDetails, error) {

	if len(volumes) == 0 {
		return nil, nil
	}
	results := make([]params.VolumeDetails, len(volumes))
	for i, v := range volumes {
		details, err := createVolumeDetails(st, v, attachments[v.VolumeTag()])
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
	st storageAccess, v state.Volume, attachments []state.VolumeAttachment,
) (*params.VolumeDetails, error) {

	details := &params.VolumeDetails{
		VolumeTag: v.VolumeTag().String(),
	}

	if info, err := v.Info(); err == nil {
		details.Info = storagecommon.VolumeInfoFromState(info)
	}

	if len(attachments) > 0 {
		details.MachineAttachments = make(map[string]params.VolumeAttachmentInfo, len(attachments))
		for _, attachment := range attachments {
			stateInfo, err := attachment.Info()
			var info params.VolumeAttachmentInfo
			if err == nil {
				info = storagecommon.VolumeAttachmentInfoFromState(stateInfo)
			}
			details.MachineAttachments[attachment.Machine().String()] = info
		}
	}

	status, err := v.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}
	details.Status = common.EntityStatusFromState(status)

	if storageTag, err := v.StorageInstance(); err == nil {
		storageInstance, err := st.StorageInstance(storageTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		storageDetails, err := createStorageDetails(st, storageInstance)
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
func (a *API) ListFilesystems(filters params.FilesystemFilters) (params.FilesystemDetailsListResults, error) {
	results := params.FilesystemDetailsListResults{
		Results: make([]params.FilesystemDetailsListResult, len(filters.Filters)),
	}
	if err := a.checkCanRead(); err != nil {
		return results, errors.Trace(err)
	}

	for i, filter := range filters.Filters {
		filesystems, filesystemAttachments, err := filterFilesystems(a.storage, filter)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		details, err := createFilesystemDetailsList(
			a.storage, filesystems, filesystemAttachments,
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
	st storageAccess,
	f params.FilesystemFilter,
) ([]state.Filesystem, map[names.FilesystemTag][]state.FilesystemAttachment, error) {
	if f.IsEmpty() {
		// No filter was specified: get all filesystems, and all attachments.
		filesystems, err := st.AllFilesystems()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		filesystemAttachments := make(map[names.FilesystemTag][]state.FilesystemAttachment)
		for _, f := range filesystems {
			attachments, err := st.FilesystemAttachments(f.FilesystemTag())
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
		attachments, err := st.MachineFilesystemAttachments(machineTag)
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
		filesystem, err := st.Filesystem(filesystemTag)
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
	st storageAccess,
	filesystems []state.Filesystem,
	attachments map[names.FilesystemTag][]state.FilesystemAttachment,
) ([]params.FilesystemDetails, error) {

	if len(filesystems) == 0 {
		return nil, nil
	}
	results := make([]params.FilesystemDetails, len(filesystems))
	for i, f := range filesystems {
		details, err := createFilesystemDetails(st, f, attachments[f.FilesystemTag()])
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
	st storageAccess, f state.Filesystem, attachments []state.FilesystemAttachment,
) (*params.FilesystemDetails, error) {

	details := &params.FilesystemDetails{
		FilesystemTag: f.FilesystemTag().String(),
	}

	if volumeTag, err := f.Volume(); err == nil {
		details.VolumeTag = volumeTag.String()
	}

	if info, err := f.Info(); err == nil {
		details.Info = storagecommon.FilesystemInfoFromState(info)
	}

	if len(attachments) > 0 {
		details.MachineAttachments = make(map[string]params.FilesystemAttachmentInfo, len(attachments))
		for _, attachment := range attachments {
			stateInfo, err := attachment.Info()
			var info params.FilesystemAttachmentInfo
			if err == nil {
				info = storagecommon.FilesystemAttachmentInfoFromState(stateInfo)
			}
			details.MachineAttachments[attachment.Machine().String()] = info
		}
	}

	status, err := f.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}
	details.Status = common.EntityStatusFromState(status)

	if storageTag, err := f.Storage(); err == nil {
		storageInstance, err := st.StorageInstance(storageTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		storageDetails, err := createStorageDetails(st, storageInstance)
		if err != nil {
			return nil, errors.Trace(err)
		}
		details.Storage = storageDetails
	}

	return details, nil
}

// AddToUnit validates and creates additional storage instances for units.
// This method handles bulk add operations and
// a failure on one individual storage instance does not block remaining
// instances from being processed.
// A "CHANGE" block can block this operation.
func (a *API) AddToUnit(args params.StoragesAddParams) (params.ErrorResults, error) {
	if err := a.checkCanWrite(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	// Check if changes are allowed and the operation may proceed.
	blockChecker := common.NewBlockChecker(a.storage)
	if err := blockChecker.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	if len(args.Storages) == 0 {
		return params.ErrorResults{}, nil
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

	result := make([]params.ErrorResult, len(args.Storages))
	for i, one := range args.Storages {
		u, err := names.ParseUnitTag(one.UnitTag)
		if err != nil {
			result[i] = params.ErrorResult{Error: common.ServerError(err)}
			continue
		}

		err = a.storage.AddStorageForUnit(u, one.StorageName, paramsToState(one.Constraints))
		if err != nil {
			result[i] = params.ErrorResult{Error: common.ServerError(err)}
		}
	}
	return params.ErrorResults{Results: result}, nil
}
