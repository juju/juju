// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider/registry"
)

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
	var all []params.StorageDetailsResult
	for _, entity := range entities.Entities {
		storageTag, err := names.ParseStorageTag(entity.Tag)
		if err != nil {
			all = append(all, params.StorageDetailsResult{
				Error: common.ServerError(err),
			})
			continue
		}
		found, instance, serverErr := api.getStorageInstance(storageTag)
		if err != nil {
			all = append(all, params.StorageDetailsResult{Error: serverErr})
			continue
		}
		if found {
			results := api.createStorageDetailsResult(storageTag, instance)
			all = append(all, results...)
		}
	}
	return params.StorageDetailsResults{Results: all}, nil
}

// List returns all currently known storage. Unlike Show(),
// if errors encountered while retrieving a particular
// storage, this error is treated as part of the returned storage detail.
func (api *API) List() (params.StorageInfosResult, error) {
	stateInstances, err := api.storage.AllStorageInstances()
	if err != nil {
		return params.StorageInfosResult{}, common.ServerError(err)
	}
	var infos []params.StorageInfo
	for _, stateInstance := range stateInstances {
		storageTag := stateInstance.StorageTag()
		persistent, err := api.isPersistent(stateInstance)
		if err != nil {
			return params.StorageInfosResult{}, err
		}
		instance := createParamsStorageInstance(stateInstance, persistent)

		// It is possible to encounter errors here related to getting individual
		// storage details such as getting attachments, getting machine from the unit,
		// etc.
		// Current approach is to do what status command does - treat error
		// as another valid property, i.e. augment storage details.
		attachments := api.createStorageDetailsResult(storageTag, instance)
		for _, one := range attachments {
			aParam := params.StorageInfo{one.Result, one.Error}
			infos = append(infos, aParam)
		}
	}
	return params.StorageInfosResult{Results: infos}, nil
}

func (api *API) createStorageDetailsResult(
	storageTag names.StorageTag,
	instance params.StorageDetails,
) []params.StorageDetailsResult {
	attachments, err := api.getStorageAttachments(storageTag, instance)
	if err != nil {
		return []params.StorageDetailsResult{params.StorageDetailsResult{Result: instance, Error: err}}
	}
	if len(attachments) > 0 {
		// If any attachments were found for this storage instance,
		// return them instead.
		result := make([]params.StorageDetailsResult, len(attachments))
		for i, attachment := range attachments {
			result[i] = params.StorageDetailsResult{Result: attachment}
		}
		return result
	}
	// If we are here then this storage instance is unattached.
	return []params.StorageDetailsResult{params.StorageDetailsResult{Result: instance}}
}

func (api *API) getStorageAttachments(
	storageTag names.StorageTag,
	instance params.StorageDetails,
) ([]params.StorageDetails, *params.Error) {
	serverError := func(err error) *params.Error {
		return common.ServerError(errors.Annotatef(err, "getting attachments for storage %v", storageTag.Id()))
	}
	stateAttachments, err := api.storage.StorageAttachments(storageTag)
	if err != nil {
		return nil, serverError(err)
	}
	result := make([]params.StorageDetails, len(stateAttachments))
	for i, one := range stateAttachments {
		paramsStorageAttachment, err := api.createParamsStorageAttachment(instance, one)
		if err != nil {
			return nil, serverError(err)
		}
		result[i] = paramsStorageAttachment
	}
	return result, nil
}

func (api *API) createParamsStorageAttachment(si params.StorageDetails, sa state.StorageAttachment) (params.StorageDetails, error) {
	result := params.StorageDetails{Status: "pending"}
	result.StorageTag = sa.StorageInstance().String()
	if result.StorageTag != si.StorageTag {
		panic("attachment does not belong to storage instance")
	}
	result.UnitTag = sa.Unit().String()
	result.OwnerTag = si.OwnerTag
	result.Kind = si.Kind
	result.Persistent = si.Persistent
	// TODO(axw) set status according to whether storage has been provisioned.

	// This is only for provisioned attachments
	machineTag, err := api.storage.UnitAssignedMachine(sa.Unit())
	if err != nil {
		return params.StorageDetails{}, errors.Annotate(err, "getting unit for storage attachment")
	}
	info, err := common.StorageAttachmentInfo(api.storage, sa, machineTag)
	if err != nil {
		if errors.IsNotProvisioned(err) {
			// If Info returns an error, then the storage has not yet been provisioned.
			return result, nil
		}
		return params.StorageDetails{}, errors.Annotate(err, "getting storage attachment info")
	}
	result.Location = info.Location
	if result.Location != "" {
		result.Status = "attached"
	}
	return result, nil
}

func (api *API) getStorageInstance(tag names.StorageTag) (bool, params.StorageDetails, *params.Error) {
	nothing := params.StorageDetails{}
	serverError := func(err error) *params.Error {
		return common.ServerError(errors.Annotatef(err, "getting %v", tag))
	}
	stateInstance, err := api.storage.StorageInstance(tag)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nothing, nil
		}
		return false, nothing, serverError(err)
	}
	persistent, err := api.isPersistent(stateInstance)
	if err != nil {
		return false, nothing, serverError(err)
	}
	return true, createParamsStorageInstance(stateInstance, persistent), nil
}

func createParamsStorageInstance(si state.StorageInstance, persistent bool) params.StorageDetails {
	result := params.StorageDetails{
		OwnerTag:   si.Owner().String(),
		StorageTag: si.Tag().String(),
		Kind:       params.StorageKind(si.Kind()),
		Status:     "pending",
		Persistent: persistent,
	}
	return result
}

// TODO(axw) move this and createParamsStorageInstance to
// apiserver/common/storage.go, alongside StorageAttachmentInfo.
func (api *API) isPersistent(si state.StorageInstance) (bool, error) {
	if si.Kind() != state.StorageKindBlock {
		// TODO(axw) when we support persistent filesystems,
		// e.g. CephFS, we'll need to do the same thing as
		// we do for volumes for filesystems.
		return false, nil
	}
	volume, err := api.storage.StorageInstanceVolume(si.StorageTag())
	if err != nil {
		return false, err
	}
	// If the volume is not provisioned, we read its config attributes.
	if params, ok := volume.Params(); ok {
		_, cfg, err := common.StoragePoolConfig(params.Pool, api.poolManager)
		if err != nil {
			return false, err
		}
		return cfg.IsPersistent(), nil
	}
	// If the volume is provisioned, we look at its provisioning info.
	info, err := volume.Info()
	if err != nil {
		return false, err
	}
	return info.Persistent, nil
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

func (a *API) ListVolumes(filter params.VolumeFilter) (params.VolumeItemsResult, error) {
	if !filter.IsEmpty() {
		return params.VolumeItemsResult{Results: a.filterVolumes(filter)}, nil
	}
	volumes, err := a.listVolumeAttachments()
	if err != nil {
		return params.VolumeItemsResult{}, common.ServerError(err)
	}
	return params.VolumeItemsResult{Results: volumes}, nil
}

func (a *API) listVolumeAttachments() ([]params.VolumeItem, error) {
	all, err := a.storage.AllVolumes()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return a.volumeAttachments(all), nil
}

func (a *API) volumeAttachments(all []state.Volume) []params.VolumeItem {
	if all == nil || len(all) == 0 {
		return nil
	}

	result := make([]params.VolumeItem, len(all))
	for i, v := range all {
		volume, err := a.convertStateVolumeToParams(v)
		if err != nil {
			result[i] = params.VolumeItem{
				Error: common.ServerError(errors.Trace(err)),
			}
			continue
		}
		result[i] = params.VolumeItem{Volume: volume}
		atts, err := a.storage.VolumeAttachments(v.VolumeTag())
		if err != nil {
			result[i].Error = common.ServerError(errors.Annotatef(
				err, "attachments for volume %v", v.VolumeTag()))
			continue
		}
		result[i].Attachments = convertStateVolumeAttachmentsToParams(atts)
	}
	return result
}

func (a *API) filterVolumes(f params.VolumeFilter) []params.VolumeItem {
	var attachments []state.VolumeAttachment
	var errs []params.VolumeItem

	addErr := func(err error) {
		errs = append(errs,
			params.VolumeItem{Error: common.ServerError(err)})
	}

	for _, machine := range f.Machines {
		tag, err := names.ParseMachineTag(machine)
		if err != nil {
			addErr(errors.Annotatef(err, "parsing machine tag %v", machine))
		}
		machineAttachments, err := a.storage.MachineVolumeAttachments(tag)
		if err != nil {
			addErr(errors.Annotatef(err,
				"getting volume attachments for machine %v",
				machine))
		}
		attachments = append(attachments, machineAttachments...)
	}
	return append(errs, a.getVolumeItems(attachments)...)
}

func (a *API) convertStateVolumeToParams(st state.Volume) (params.VolumeInstance, error) {
	volume := params.VolumeInstance{VolumeTag: st.VolumeTag().String()}

	if storage, err := st.StorageInstance(); err == nil {
		volume.StorageTag = storage.String()
		storageInstance, err := a.storage.StorageInstance(storage)
		if err != nil {
			err = errors.Annotatef(err,
				"getting storage instance %v for volume %v",
				storage, volume.VolumeTag)
			return params.VolumeInstance{}, err
		}
		owner := storageInstance.Owner()
		// only interested in Unit for now
		if unitTag, ok := owner.(names.UnitTag); ok {
			volume.UnitTag = unitTag.String()
		}
	}
	if info, err := st.Info(); err == nil {
		volume.HardwareId = info.HardwareId
		volume.Size = info.Size
		volume.Persistent = info.Persistent
		volume.VolumeId = info.VolumeId
	}
	status, err := st.Status()
	if err != nil {
		return params.VolumeInstance{}, errors.Trace(err)
	}
	volume.Status = common.EntityStatusFromState(status)
	return volume, nil
}

func convertStateVolumeAttachmentsToParams(all []state.VolumeAttachment) []params.VolumeAttachment {
	if len(all) == 0 {
		return nil
	}
	result := make([]params.VolumeAttachment, len(all))
	for i, one := range all {
		result[i] = convertStateVolumeAttachmentToParams(one)
	}
	return result
}

func convertStateVolumeAttachmentToParams(attachment state.VolumeAttachment) params.VolumeAttachment {
	result := params.VolumeAttachment{
		VolumeTag:  attachment.Volume().String(),
		MachineTag: attachment.Machine().String()}
	if info, err := attachment.Info(); err == nil {
		result.Info = params.VolumeAttachmentInfo{
			info.DeviceName,
			info.BusAddress,
			info.ReadOnly,
		}
	}
	return result
}

func (a *API) getVolumeItems(all []state.VolumeAttachment) []params.VolumeItem {
	group := groupAttachmentsByVolume(all)

	if len(group) == 0 {
		return nil
	}

	result := make([]params.VolumeItem, len(group))
	i := 0
	for volumeTag, attachments := range group {
		result[i] = a.createVolumeItem(volumeTag, attachments)
		i++
	}
	return result
}

func (a *API) createVolumeItem(volumeTag string, attachments []params.VolumeAttachment) params.VolumeItem {
	result := params.VolumeItem{Attachments: attachments}

	tag, err := names.ParseVolumeTag(volumeTag)
	if err != nil {
		result.Error = common.ServerError(errors.Annotatef(err, "parsing volume tag %v", volumeTag))
		return result
	}
	st, err := a.storage.Volume(tag)
	if err != nil {
		result.Error = common.ServerError(errors.Annotatef(err, "getting volume for tag %v", tag))
		return result
	}
	volume, err := a.convertStateVolumeToParams(st)
	if err != nil {
		result.Error = common.ServerError(errors.Trace(err))
		return result
	}
	result.Volume = volume
	return result
}

// groupAttachmentsByVolume constructs map of attachments grouped by volumeTag
func groupAttachmentsByVolume(all []state.VolumeAttachment) map[string][]params.VolumeAttachment {
	if len(all) == 0 {
		return nil
	}
	group := make(map[string][]params.VolumeAttachment)
	for _, one := range all {
		attachment := convertStateVolumeAttachmentToParams(one)
		group[attachment.VolumeTag] = append(
			group[attachment.VolumeTag],
			attachment)
	}
	return group
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
