// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package storage provides an API server facade for managing
// storage entities.
package storage

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider/registry"
)

var logger = loggo.GetLogger("juju.apiserver.storage")

func init() {
	common.RegisterStandardFacade("Storage", 1, NewAPI)
}

// API implements the storage interface and is the concrete
// implementation of the api end point.
type API struct {
	storage     storageAccess
	poolManager poolmanager.PoolManager
	authorizer  common.Authorizer
}

// createAPI returns a new storage API facade.
func createAPI(
	st storageAccess,
	pm poolmanager.PoolManager,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		storage:     st,
		poolManager: pm,
		authorizer:  authorizer,
	}, nil
}

// NewAPI returns a new storage API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	return createAPI(getState(st), poolManager(st), resources, authorizer)
}

func poolManager(st *state.State) poolmanager.PoolManager {
	return poolmanager.New(state.NewStateSettings(st))
}

// Show retrieves and returns detailed information about desired storage
// identified by supplied tags. If specified storage cannot be retrieved,
// individual error is returned instead of storage information.
func (api *API) Show(entities params.Entities) (params.StorageDetailsResults, error) {
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
		results[i] = api.createStorageDetailsResult(storageInstance)
	}
	return params.StorageDetailsResults{Results: results}, nil
}

// List returns all currently known storage. Unlike Show(),
// if errors encountered while retrieving a particular
// storage, this error is treated as part of the returned storage detail.
func (api *API) List() (params.StorageDetailsResults, error) {
	stateInstances, err := api.storage.AllStorageInstances()
	if err != nil {
		return params.StorageDetailsResults{}, common.ServerError(err)
	}
	results := make([]params.StorageDetailsResult, len(stateInstances))
	for i, stateInstance := range stateInstances {
		results[i] = api.createStorageDetailsResult(stateInstance)
	}
	return params.StorageDetailsResults{Results: results}, nil
}

func (api *API) createStorageDetailsResult(si state.StorageInstance) params.StorageDetailsResult {
	details, err := api.createStorageDetails(si)
	if err != nil {
		return params.StorageDetailsResult{Error: common.ServerError(err)}
	}

	legacy := params.LegacyStorageDetails{
		details.StorageTag,
		details.OwnerTag,
		details.Kind,
		string(details.Status.Status),
		"", // unit tag set below
		"", // location set below
		details.Persistent,
	}
	if len(details.Attachments) == 1 {
		for unitTag, attachmentDetails := range details.Attachments {
			legacy.UnitTag = unitTag
			legacy.Location = attachmentDetails.Location
		}
	}

	return params.StorageDetailsResult{Result: details, Legacy: legacy}
}

func (api *API) createStorageDetails(si state.StorageInstance) (*params.StorageDetails, error) {
	// Get information from underlying volume or filesystem.
	var persistent bool
	var statusEntity state.StatusGetter
	if si.Kind() != state.StorageKindBlock {
		// TODO(axw) when we support persistent filesystems,
		// e.g. CephFS, we'll need to do set "persistent"
		// here too.
		filesystem, err := api.storage.StorageInstanceFilesystem(si.StorageTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		statusEntity = filesystem
	} else {
		volume, err := api.storage.StorageInstanceVolume(si.StorageTag())
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
	storageAttachments, err := api.storage.StorageAttachments(si.StorageTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(storageAttachments) > 0 {
		storageAttachmentDetails = make(map[string]params.StorageAttachmentDetails)
		for _, a := range storageAttachments {
			machineTag, location, err := api.storageAttachmentInfo(a)
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

func (api *API) storageAttachmentInfo(a state.StorageAttachment) (_ names.MachineTag, location string, _ error) {
	machineTag, err := api.storage.UnitAssignedMachine(a.Unit())
	if errors.IsNotAssigned(err) {
		return names.MachineTag{}, "", nil
	} else if err != nil {
		return names.MachineTag{}, "", errors.Trace(err)
	}
	info, err := storagecommon.StorageAttachmentInfo(api.storage, a, machineTag)
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
	filter params.StoragePoolFilter,
) (params.StoragePoolsResult, error) {

	if ok, err := a.isValidPoolListFilter(filter); !ok {
		return params.StoragePoolsResult{}, err
	}

	pools, err := a.poolManager.List()
	if err != nil {
		return params.StoragePoolsResult{}, err
	}
	providers, err := a.allProviders()
	if err != nil {
		return params.StoragePoolsResult{}, err
	}
	matches := buildFilter(filter)
	results := append(
		filterPools(pools, matches),
		filterProviders(providers, matches)...,
	)
	return params.StoragePoolsResult{results}, nil
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

func (a *API) allProviders() ([]storage.ProviderType, error) {
	envName, err := a.storage.EnvName()
	if err != nil {
		return nil, errors.Annotate(err, "getting env name")
	}
	if providers, ok := registry.EnvironStorageProviders(envName); ok {
		return providers, nil
	}
	return nil, nil
}

func (a *API) isValidPoolListFilter(
	filter params.StoragePoolFilter,
) (bool, error) {
	if len(filter.Providers) != 0 {
		if valid, err := a.isValidProviderCriteria(filter.Providers); !valid {
			return false, errors.Trace(err)
		}
	}
	if len(filter.Names) != 0 {
		if valid, err := a.isValidNameCriteria(filter.Names); !valid {
			return false, errors.Trace(err)
		}
	}
	return true, nil
}

func (a *API) isValidNameCriteria(names []string) (bool, error) {
	for _, n := range names {
		if !storage.IsValidPoolName(n) {
			return false, errors.NotValidf("pool name %q", n)
		}
	}
	return true, nil
}

func (a *API) isValidProviderCriteria(providers []string) (bool, error) {
	envName, err := a.storage.EnvName()
	if err != nil {
		return false, errors.Annotate(err, "getting env name")
	}
	for _, p := range providers {
		if !registry.IsProviderSupported(envName, storage.ProviderType(p)) {
			return false, errors.NotSupportedf("%q for environment %q", p, envName)
		}
	}
	return true, nil
}

// CreatePool creates a new pool with specified parameters.
func (a *API) CreatePool(p params.StoragePool) error {
	_, err := a.poolManager.Create(
		p.Name,
		storage.ProviderType(p.Provider),
		p.Attrs)
	return err
}

func (a *API) ListVolumes(filter params.VolumeFilter) (params.VolumeDetailsResults, error) {
	volumes, volumeAttachments, err := filterVolumes(a.storage, filter)
	if err != nil {
		return params.VolumeDetailsResults{}, common.ServerError(err)
	}
	results := createVolumeDetailsResults(a.storage, volumes, volumeAttachments)
	return params.VolumeDetailsResults{Results: results}, nil
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

func createVolumeDetailsResults(
	st storageAccess,
	volumes []state.Volume,
	attachments map[names.VolumeTag][]state.VolumeAttachment,
) []params.VolumeDetailsResult {

	if len(volumes) == 0 {
		return nil
	}

	results := make([]params.VolumeDetailsResult, len(volumes))
	for i, v := range volumes {
		details, err := createVolumeDetails(st, v, attachments[v.VolumeTag()])
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		result := params.VolumeDetailsResult{
			Details: details,
		}

		// We need to populate the legacy fields for old clients.
		if len(details.MachineAttachments) > 0 {
			result.LegacyAttachments = make([]params.VolumeAttachment, 0, len(details.MachineAttachments))
			for machineTag, attachmentInfo := range details.MachineAttachments {
				result.LegacyAttachments = append(result.LegacyAttachments, params.VolumeAttachment{
					VolumeTag:  details.VolumeTag,
					MachineTag: machineTag,
					Info:       attachmentInfo,
				})
			}
		}
		result.LegacyVolume = &params.LegacyVolumeDetails{
			VolumeTag:  details.VolumeTag,
			StorageTag: details.StorageTag,
			VolumeId:   details.Info.VolumeId,
			HardwareId: details.Info.HardwareId,
			Size:       details.Info.Size,
			Persistent: details.Info.Persistent,
			Status:     details.Status,
		}
		if details.StorageOwnerTag != "" {
			kind, err := names.TagKind(details.StorageOwnerTag)
			if err != nil {
				results[i].Error = common.ServerError(err)
				continue
			}
			if kind == names.UnitTagKind {
				result.LegacyVolume.UnitTag = details.StorageOwnerTag
			}
		}
		results[i] = result
	}
	return results
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
		details.StorageTag = storageTag.String()
		storageInstance, err := st.StorageInstance(storageTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		details.StorageOwnerTag = storageInstance.Owner().String()
	}

	return details, nil
}

// ListFilesystems returns a list of filesystems in the environment matching
// the provided filter. Each result describes a filesystem in detail, including
// the filesystem's attachments.
func (a *API) ListFilesystems(filter params.FilesystemFilter) (params.FilesystemDetailsResults, error) {
	filesystems, filesystemAttachments, err := filterFilesystems(a.storage, filter)
	if err != nil {
		return params.FilesystemDetailsResults{}, common.ServerError(err)
	}
	results := createFilesystemDetailsResults(a.storage, filesystems, filesystemAttachments)
	return params.FilesystemDetailsResults{Results: results}, nil
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

func createFilesystemDetailsResults(
	st storageAccess,
	filesystems []state.Filesystem,
	attachments map[names.FilesystemTag][]state.FilesystemAttachment,
) []params.FilesystemDetailsResult {

	if len(filesystems) == 0 {
		return nil
	}

	results := make([]params.FilesystemDetailsResult, len(filesystems))
	for i, f := range filesystems {
		details, err := createFilesystemDetails(st, f, attachments[f.FilesystemTag()])
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Result = details
	}
	return results
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
		details.StorageTag = storageTag.String()
		storageInstance, err := st.StorageInstance(storageTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		details.StorageOwnerTag = storageInstance.Owner().String()
	}

	return details, nil
}

// AddToUnit validates and creates additional storage instances for units.
// This method handles bulk add operations and
// a failure on one individual storage instance does not block remaining
// instances from being processed.
// A "CHANGE" block can block this operation.
func (a *API) AddToUnit(args params.StoragesAddParams) (params.ErrorResults, error) {
	// Check if changes are allowed and the operation may proceed.
	blockChecker := common.NewBlockChecker(a.storage)
	if err := blockChecker.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	if len(args.Storages) == 0 {
		return params.ErrorResults{}, nil
	}

	serverErr := func(err error) params.ErrorResult {
		if errors.IsNotFound(err) {
			err = common.ErrPerm
		}
		return params.ErrorResult{Error: common.ServerError(err)}
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
			result[i] = serverErr(
				errors.Annotatef(err, "parsing unit tag %v", one.UnitTag))
			continue
		}

		err = a.storage.AddStorageForUnit(u,
			one.StorageName,
			paramsToState(one.Constraints))
		if err != nil {
			result[i] = serverErr(
				errors.Annotatef(err, "adding storage %v for %v", one.StorageName, one.UnitTag))
		}
	}
	return params.ErrorResults{Results: result}, nil
}
