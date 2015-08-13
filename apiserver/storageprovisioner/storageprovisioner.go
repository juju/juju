// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

var logger = loggo.GetLogger("juju.apiserver.storageprovisioner")

func init() {
	common.RegisterStandardFacade("StorageProvisioner", 1, NewStorageProvisionerAPI)
}

// StorageProvisionerAPI provides access to the Provisioner API facade.
type StorageProvisionerAPI struct {
	*common.LifeGetter
	*common.DeadEnsurer
	*common.EnvironWatcher
	*common.InstanceIdGetter
	*common.StatusSetter

	st                       provisionerState
	settings                 poolmanager.SettingsManager
	resources                *common.Resources
	authorizer               common.Authorizer
	getScopeAuthFunc         common.GetAuthFunc
	getStorageEntityAuthFunc common.GetAuthFunc
	getMachineAuthFunc       common.GetAuthFunc
	getBlockDevicesAuthFunc  common.GetAuthFunc
	getAttachmentAuthFunc    func() (func(names.MachineTag, names.Tag) bool, error)
}

var getState = func(st *state.State) provisionerState {
	return stateShim{st}
}

var getSettingsManager = func(st *state.State) poolmanager.SettingsManager {
	return state.NewStateSettings(st)
}

// NewStorageProvisionerAPI creates a new server-side StorageProvisionerAPI facade.
func NewStorageProvisionerAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*StorageProvisionerAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	canAccessStorageMachine := func(tag names.MachineTag, allowEnvironManager bool) bool {
		authEntityTag := authorizer.GetAuthTag()
		if tag == authEntityTag {
			// Machine agents can access volumes
			// scoped to their own machine.
			return true
		}
		parentId := state.ParentId(tag.Id())
		if parentId == "" {
			return allowEnvironManager && authorizer.AuthEnvironManager()
		}
		// All containers with the authenticated
		// machine as a parent are accessible by it.
		return names.NewMachineTag(parentId) == authEntityTag
	}
	getScopeAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			switch tag := tag.(type) {
			case names.EnvironTag:
				// Environment managers can access all volumes
				// and filesystems scoped to the environment.
				isEnvironManager := authorizer.AuthEnvironManager()
				return isEnvironManager && tag == st.EnvironTag()
			case names.MachineTag:
				return canAccessStorageMachine(tag, false)
			default:
				return false
			}
		}, nil
	}
	canAccessStorageEntity := func(tag names.Tag, allowMachines bool) bool {
		switch tag := tag.(type) {
		case names.VolumeTag:
			machineTag, ok := names.VolumeMachine(tag)
			if ok {
				return canAccessStorageMachine(machineTag, false)
			}
			return authorizer.AuthEnvironManager()
		case names.FilesystemTag:
			machineTag, ok := names.FilesystemMachine(tag)
			if ok {
				return canAccessStorageMachine(machineTag, false)
			}
			return authorizer.AuthEnvironManager()
		case names.MachineTag:
			return allowMachines && canAccessStorageMachine(tag, true)
		default:
			return false
		}
	}
	getStorageEntityAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return canAccessStorageEntity(tag, false)
		}, nil
	}
	getLifeAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return canAccessStorageEntity(tag, true)
		}, nil
	}
	getAttachmentAuthFunc := func() (func(names.MachineTag, names.Tag) bool, error) {
		// getAttachmentAuthFunc returns a function that validates
		// access by the authenticated user to an attachment.
		return func(machineTag names.MachineTag, attachmentTag names.Tag) bool {
			// Machine agents can access their own machine, and
			// machines contained. Environment managers can access
			// top-level machines.
			if !canAccessStorageMachine(machineTag, true) {
				return false
			}
			// Environment managers can access environment-scoped
			// volumes and volumes scoped to their own machines.
			// Other machine agents can access volumes regardless
			// of their scope.
			if !authorizer.AuthEnvironManager() {
				return true
			}
			var machineScope names.MachineTag
			var hasMachineScope bool
			switch attachmentTag := attachmentTag.(type) {
			case names.VolumeTag:
				machineScope, hasMachineScope = names.VolumeMachine(attachmentTag)
			case names.FilesystemTag:
				machineScope, hasMachineScope = names.FilesystemMachine(attachmentTag)
			}
			return !hasMachineScope || machineScope == authorizer.GetAuthTag()
		}, nil
	}
	getMachineAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag, ok := tag.(names.MachineTag); ok {
				return canAccessStorageMachine(tag, true)
			}
			return false
		}, nil
	}
	getBlockDevicesAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag, ok := tag.(names.MachineTag); ok {
				return canAccessStorageMachine(tag, false)
			}
			return false
		}, nil
	}
	stateInterface := getState(st)
	settings := getSettingsManager(st)
	return &StorageProvisionerAPI{
		LifeGetter:       common.NewLifeGetter(stateInterface, getLifeAuthFunc),
		DeadEnsurer:      common.NewDeadEnsurer(stateInterface, getStorageEntityAuthFunc),
		EnvironWatcher:   common.NewEnvironWatcher(stateInterface, resources, authorizer),
		InstanceIdGetter: common.NewInstanceIdGetter(st, getMachineAuthFunc),
		StatusSetter:     common.NewStatusSetter(st, getStorageEntityAuthFunc),

		st:                       stateInterface,
		settings:                 settings,
		resources:                resources,
		authorizer:               authorizer,
		getScopeAuthFunc:         getScopeAuthFunc,
		getStorageEntityAuthFunc: getStorageEntityAuthFunc,
		getAttachmentAuthFunc:    getAttachmentAuthFunc,
		getMachineAuthFunc:       getMachineAuthFunc,
		getBlockDevicesAuthFunc:  getBlockDevicesAuthFunc,
	}, nil
}

// WatchBlockDevices watches for changes to the specified machines' block devices.
func (s *StorageProvisionerAPI) WatchBlockDevices(args params.Entities) (params.NotifyWatchResults, error) {
	canAccess, err := s.getBlockDevicesAuthFunc()
	if err != nil {
		return params.NotifyWatchResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (string, error) {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			return "", err
		}
		if !canAccess(machineTag) {
			return "", common.ErrPerm
		}
		w := s.st.WatchBlockDevices(machineTag)
		if _, ok := <-w.Changes(); ok {
			return s.resources.Register(w), nil
		}
		return "", watcher.EnsureErr(w)
	}
	for i, arg := range args.Entities {
		var result params.NotifyWatchResult
		id, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.NotifyWatcherId = id
		}
		results.Results[i] = result
	}
	return results, nil
}

// WatchMachines watches for changes to the specified machines.
func (s *StorageProvisionerAPI) WatchMachines(args params.Entities) (params.NotifyWatchResults, error) {
	canAccess, err := s.getMachineAuthFunc()
	if err != nil {
		return params.NotifyWatchResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (string, error) {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			return "", err
		}
		if !canAccess(machineTag) {
			return "", common.ErrPerm
		}
		w, err := s.st.WatchMachine(machineTag)
		if err != nil {
			return "", errors.Trace(err)
		}
		if _, ok := <-w.Changes(); ok {
			return s.resources.Register(w), nil
		}
		return "", watcher.EnsureErr(w)
	}
	for i, arg := range args.Entities {
		var result params.NotifyWatchResult
		id, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.NotifyWatcherId = id
		}
		results.Results[i] = result
	}
	return results, nil
}

// WatchVolumes watches for changes to volumes scoped to the
// entity with the tag passed to NewState.
func (s *StorageProvisionerAPI) WatchVolumes(args params.Entities) (params.StringsWatchResults, error) {
	return s.watchStorageEntities(args, s.st.WatchEnvironVolumes, s.st.WatchMachineVolumes)
}

// WatchFilesystems watches for changes to filesystems scoped
// to the entity with the tag passed to NewState.
func (s *StorageProvisionerAPI) WatchFilesystems(args params.Entities) (params.StringsWatchResults, error) {
	return s.watchStorageEntities(args, s.st.WatchEnvironFilesystems, s.st.WatchMachineFilesystems)
}

func (s *StorageProvisionerAPI) watchStorageEntities(
	args params.Entities,
	watchEnvironStorage func() state.StringsWatcher,
	watchMachineStorage func(names.MachineTag) state.StringsWatcher,
) (params.StringsWatchResults, error) {
	canAccess, err := s.getScopeAuthFunc()
	if err != nil {
		return params.StringsWatchResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (string, []string, error) {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return "", nil, common.ErrPerm
		}
		var w state.StringsWatcher
		if tag, ok := tag.(names.MachineTag); ok {
			w = watchMachineStorage(tag)
		} else {
			w = watchEnvironStorage()
		}
		if changes, ok := <-w.Changes(); ok {
			return s.resources.Register(w), changes, nil
		}
		return "", nil, watcher.EnsureErr(w)
	}
	for i, arg := range args.Entities {
		var result params.StringsWatchResult
		id, changes, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.StringsWatcherId = id
			result.Changes = changes
		}
		results.Results[i] = result
	}
	return results, nil
}

// WatchVolumeAttachments watches for changes to volume attachments scoped to
// the entity with the tag passed to NewState.
func (s *StorageProvisionerAPI) WatchVolumeAttachments(args params.Entities) (params.MachineStorageIdsWatchResults, error) {
	return s.watchAttachments(
		args,
		s.st.WatchEnvironVolumeAttachments,
		s.st.WatchMachineVolumeAttachments,
		common.ParseVolumeAttachmentIds,
	)
}

// WatchFilesystemAttachments watches for changes to filesystem attachments
// scoped to the entity with the tag passed to NewState.
func (s *StorageProvisionerAPI) WatchFilesystemAttachments(args params.Entities) (params.MachineStorageIdsWatchResults, error) {
	return s.watchAttachments(
		args,
		s.st.WatchEnvironFilesystemAttachments,
		s.st.WatchMachineFilesystemAttachments,
		common.ParseFilesystemAttachmentIds,
	)
}

func (s *StorageProvisionerAPI) watchAttachments(
	args params.Entities,
	watchEnvironAttachments func() state.StringsWatcher,
	watchMachineAttachments func(names.MachineTag) state.StringsWatcher,
	parseAttachmentIds func([]string) ([]params.MachineStorageId, error),
) (params.MachineStorageIdsWatchResults, error) {
	canAccess, err := s.getScopeAuthFunc()
	if err != nil {
		return params.MachineStorageIdsWatchResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.MachineStorageIdsWatchResults{
		Results: make([]params.MachineStorageIdsWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (string, []params.MachineStorageId, error) {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return "", nil, common.ErrPerm
		}
		var w state.StringsWatcher
		if tag, ok := tag.(names.MachineTag); ok {
			w = watchMachineAttachments(tag)
		} else {
			w = watchEnvironAttachments()
		}
		if stringChanges, ok := <-w.Changes(); ok {
			changes, err := parseAttachmentIds(stringChanges)
			if err != nil {
				w.Stop()
				return "", nil, err
			}
			return s.resources.Register(w), changes, nil
		}
		return "", nil, watcher.EnsureErr(w)
	}
	for i, arg := range args.Entities {
		var result params.MachineStorageIdsWatchResult
		id, changes, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.MachineStorageIdsWatcherId = id
			result.Changes = changes
		}
		results.Results[i] = result
	}
	return results, nil
}

// Volumes returns details of volumes with the specified tags.
func (s *StorageProvisionerAPI) Volumes(args params.Entities) (params.VolumeResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc()
	if err != nil {
		return params.VolumeResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.VolumeResults{
		Results: make([]params.VolumeResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (params.Volume, error) {
		tag, err := names.ParseVolumeTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.Volume{}, common.ErrPerm
		}
		volume, err := s.st.Volume(tag)
		if errors.IsNotFound(err) {
			return params.Volume{}, common.ErrPerm
		} else if err != nil {
			return params.Volume{}, err
		}
		return common.VolumeFromState(volume)
	}
	for i, arg := range args.Entities {
		var result params.VolumeResult
		volume, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.Result = volume
		}
		results.Results[i] = result
	}
	return results, nil
}

// Filesystems returns details of filesystems with the specified tags.
func (s *StorageProvisionerAPI) Filesystems(args params.Entities) (params.FilesystemResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc()
	if err != nil {
		return params.FilesystemResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.FilesystemResults{
		Results: make([]params.FilesystemResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (params.Filesystem, error) {
		tag, err := names.ParseFilesystemTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.Filesystem{}, common.ErrPerm
		}
		filesystem, err := s.st.Filesystem(tag)
		if errors.IsNotFound(err) {
			return params.Filesystem{}, common.ErrPerm
		} else if err != nil {
			return params.Filesystem{}, err
		}
		return common.FilesystemFromState(filesystem)
	}
	for i, arg := range args.Entities {
		var result params.FilesystemResult
		filesystem, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.Result = filesystem
		}
		results.Results[i] = result
	}
	return results, nil
}

// VolumeAttachments returns details of volume attachments with the specified IDs.
func (s *StorageProvisionerAPI) VolumeAttachments(args params.MachineStorageIds) (params.VolumeAttachmentResults, error) {
	canAccess, err := s.getAttachmentAuthFunc()
	if err != nil {
		return params.VolumeAttachmentResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.VolumeAttachmentResults{
		Results: make([]params.VolumeAttachmentResult, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (params.VolumeAttachment, error) {
		volumeAttachment, err := s.oneVolumeAttachment(arg, canAccess)
		if err != nil {
			return params.VolumeAttachment{}, err
		}
		return common.VolumeAttachmentFromState(volumeAttachment)
	}
	for i, arg := range args.Ids {
		var result params.VolumeAttachmentResult
		volumeAttachment, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.Result = volumeAttachment
		}
		results.Results[i] = result
	}
	return results, nil
}

// VolumeBlockDevices returns details of the block devices corresponding to the
// volume attachments with the specified IDs.
func (s *StorageProvisionerAPI) VolumeBlockDevices(args params.MachineStorageIds) (params.BlockDeviceResults, error) {
	canAccess, err := s.getAttachmentAuthFunc()
	if err != nil {
		return params.BlockDeviceResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.BlockDeviceResults{
		Results: make([]params.BlockDeviceResult, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (storage.BlockDevice, error) {
		stateBlockDevice, err := s.oneVolumeBlockDevice(arg, canAccess)
		if err != nil {
			return storage.BlockDevice{}, err
		}
		return common.BlockDeviceFromState(stateBlockDevice), nil
	}
	for i, arg := range args.Ids {
		var result params.BlockDeviceResult
		blockDevice, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.Result = blockDevice
		}
		results.Results[i] = result
	}
	return results, nil
}

// FilesystemAttachments returns details of filesystem attachments with the specified IDs.
func (s *StorageProvisionerAPI) FilesystemAttachments(args params.MachineStorageIds) (params.FilesystemAttachmentResults, error) {
	canAccess, err := s.getAttachmentAuthFunc()
	if err != nil {
		return params.FilesystemAttachmentResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.FilesystemAttachmentResults{
		Results: make([]params.FilesystemAttachmentResult, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (params.FilesystemAttachment, error) {
		filesystemAttachment, err := s.oneFilesystemAttachment(arg, canAccess)
		if err != nil {
			return params.FilesystemAttachment{}, err
		}
		return common.FilesystemAttachmentFromState(filesystemAttachment)
	}
	for i, arg := range args.Ids {
		var result params.FilesystemAttachmentResult
		filesystemAttachment, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.Result = filesystemAttachment
		}
		results.Results[i] = result
	}
	return results, nil
}

// VolumeParams returns the parameters for creating or destroying
// the volumes with the specified tags.
func (s *StorageProvisionerAPI) VolumeParams(args params.Entities) (params.VolumeParamsResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc()
	if err != nil {
		return params.VolumeParamsResults{}, err
	}
	envConfig, err := s.st.EnvironConfig()
	if err != nil {
		return params.VolumeParamsResults{}, err
	}
	results := params.VolumeParamsResults{
		Results: make([]params.VolumeParamsResult, len(args.Entities)),
	}
	poolManager := poolmanager.New(s.settings)
	one := func(arg params.Entity) (params.VolumeParams, error) {
		tag, err := names.ParseVolumeTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.VolumeParams{}, common.ErrPerm
		}
		volume, err := s.st.Volume(tag)
		if errors.IsNotFound(err) {
			return params.VolumeParams{}, common.ErrPerm
		} else if err != nil {
			return params.VolumeParams{}, err
		}
		volumeAttachments, err := s.st.VolumeAttachments(tag)
		if err != nil {
			return params.VolumeParams{}, err
		}
		storageInstance, err := common.MaybeAssignedStorageInstance(
			volume.StorageInstance,
			s.st.StorageInstance,
		)
		if err != nil {
			return params.VolumeParams{}, err
		}
		volumeParams, err := common.VolumeParams(volume, storageInstance, envConfig, poolManager)
		if err != nil {
			return params.VolumeParams{}, err
		}
		if len(volumeAttachments) == 1 {
			// There is exactly one attachment to be made, so make
			// it immediately. Otherwise we will defer attachments
			// until later.
			volumeAttachment := volumeAttachments[0]
			volumeAttachmentParams, ok := volumeAttachment.Params()
			if !ok {
				return params.VolumeParams{}, errors.Errorf(
					"volume %q is already attached to machine %q",
					volumeAttachment.Volume().Id(),
					volumeAttachment.Machine().Id(),
				)
			}
			machineTag := volumeAttachment.Machine()
			instanceId, err := s.st.MachineInstanceId(machineTag)
			if errors.IsNotProvisioned(err) {
				// Leave the attachment until later.
				instanceId = ""
			} else if err != nil {
				return params.VolumeParams{}, err
			}
			volumeParams.Attachment = &params.VolumeAttachmentParams{
				tag.String(),
				machineTag.String(),
				"", // volume ID
				string(instanceId),
				volumeParams.Provider,
				volumeAttachmentParams.ReadOnly,
			}
		}
		return volumeParams, nil
	}
	for i, arg := range args.Entities {
		var result params.VolumeParamsResult
		volumeParams, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.Result = volumeParams
		}
		results.Results[i] = result
	}
	return results, nil
}

// FilesystemParams returns the parameters for creating the filesystems
// with the specified tags.
func (s *StorageProvisionerAPI) FilesystemParams(args params.Entities) (params.FilesystemParamsResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc()
	if err != nil {
		return params.FilesystemParamsResults{}, err
	}
	envConfig, err := s.st.EnvironConfig()
	if err != nil {
		return params.FilesystemParamsResults{}, err
	}
	results := params.FilesystemParamsResults{
		Results: make([]params.FilesystemParamsResult, len(args.Entities)),
	}
	poolManager := poolmanager.New(s.settings)
	one := func(arg params.Entity) (params.FilesystemParams, error) {
		tag, err := names.ParseFilesystemTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.FilesystemParams{}, common.ErrPerm
		}
		filesystem, err := s.st.Filesystem(tag)
		if errors.IsNotFound(err) {
			return params.FilesystemParams{}, common.ErrPerm
		} else if err != nil {
			return params.FilesystemParams{}, err
		}
		storageInstance, err := common.MaybeAssignedStorageInstance(
			filesystem.Storage,
			s.st.StorageInstance,
		)
		if err != nil {
			return params.FilesystemParams{}, err
		}
		filesystemParams, err := common.FilesystemParams(
			filesystem, storageInstance, envConfig, poolManager,
		)
		if err != nil {
			return params.FilesystemParams{}, err
		}
		return filesystemParams, nil
	}
	for i, arg := range args.Entities {
		var result params.FilesystemParamsResult
		filesystemParams, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.Result = filesystemParams
		}
		results.Results[i] = result
	}
	return results, nil
}

// VolumeAttachmentParams returns the parameters for creating the volume
// attachments with the specified IDs.
func (s *StorageProvisionerAPI) VolumeAttachmentParams(
	args params.MachineStorageIds,
) (params.VolumeAttachmentParamsResults, error) {
	canAccess, err := s.getAttachmentAuthFunc()
	if err != nil {
		return params.VolumeAttachmentParamsResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.VolumeAttachmentParamsResults{
		Results: make([]params.VolumeAttachmentParamsResult, len(args.Ids)),
	}
	poolManager := poolmanager.New(s.settings)
	one := func(arg params.MachineStorageId) (params.VolumeAttachmentParams, error) {
		volumeAttachment, err := s.oneVolumeAttachment(arg, canAccess)
		if err != nil {
			return params.VolumeAttachmentParams{}, err
		}
		instanceId, err := s.st.MachineInstanceId(volumeAttachment.Machine())
		if errors.IsNotProvisioned(err) {
			// The worker must watch for machine provisioning events.
			instanceId = ""
		} else if err != nil {
			return params.VolumeAttachmentParams{}, err
		}
		volume, err := s.st.Volume(volumeAttachment.Volume())
		if err != nil {
			return params.VolumeAttachmentParams{}, err
		}
		var volumeId string
		var pool string
		if volumeParams, ok := volume.Params(); ok {
			pool = volumeParams.Pool
		} else {
			volumeInfo, err := volume.Info()
			if err != nil {
				return params.VolumeAttachmentParams{}, err
			}
			volumeId = volumeInfo.VolumeId
			pool = volumeInfo.Pool
		}
		providerType, _, err := common.StoragePoolConfig(pool, poolManager)
		if err != nil {
			return params.VolumeAttachmentParams{}, errors.Trace(err)
		}
		var readOnly bool
		if volumeAttachmentParams, ok := volumeAttachment.Params(); ok {
			readOnly = volumeAttachmentParams.ReadOnly
		} else {
			// Attachment parameters may be requested even if the
			// attachment exists; i.e. for reattachment.
			volumeAttachmentInfo, err := volumeAttachment.Info()
			if err != nil {
				return params.VolumeAttachmentParams{}, errors.Trace(err)
			}
			readOnly = volumeAttachmentInfo.ReadOnly
		}
		return params.VolumeAttachmentParams{
			volumeAttachment.Volume().String(),
			volumeAttachment.Machine().String(),
			volumeId,
			string(instanceId),
			string(providerType),
			readOnly,
		}, nil
	}
	for i, arg := range args.Ids {
		var result params.VolumeAttachmentParamsResult
		volumeAttachment, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.Result = volumeAttachment
		}
		results.Results[i] = result
	}
	return results, nil
}

// FilesystemAttachmentParams returns the parameters for creating the filesystem
// attachments with the specified IDs.
func (s *StorageProvisionerAPI) FilesystemAttachmentParams(
	args params.MachineStorageIds,
) (params.FilesystemAttachmentParamsResults, error) {
	canAccess, err := s.getAttachmentAuthFunc()
	if err != nil {
		return params.FilesystemAttachmentParamsResults{}, common.ServerError(common.ErrPerm)
	}
	results := params.FilesystemAttachmentParamsResults{
		Results: make([]params.FilesystemAttachmentParamsResult, len(args.Ids)),
	}
	poolManager := poolmanager.New(s.settings)
	one := func(arg params.MachineStorageId) (params.FilesystemAttachmentParams, error) {
		filesystemAttachment, err := s.oneFilesystemAttachment(arg, canAccess)
		if err != nil {
			return params.FilesystemAttachmentParams{}, err
		}
		instanceId, err := s.st.MachineInstanceId(filesystemAttachment.Machine())
		if errors.IsNotProvisioned(err) {
			// The worker must watch for machine provisioning events.
			instanceId = ""
		} else if err != nil {
			return params.FilesystemAttachmentParams{}, err
		}
		filesystem, err := s.st.Filesystem(filesystemAttachment.Filesystem())
		if err != nil {
			return params.FilesystemAttachmentParams{}, err
		}
		var filesystemId string
		var pool string
		if filesystemParams, ok := filesystem.Params(); ok {
			pool = filesystemParams.Pool
		} else {
			filesystemInfo, err := filesystem.Info()
			if err != nil {
				return params.FilesystemAttachmentParams{}, err
			}
			filesystemId = filesystemInfo.FilesystemId
			pool = filesystemInfo.Pool
		}
		providerType, _, err := common.StoragePoolConfig(pool, poolManager)
		if err != nil {
			return params.FilesystemAttachmentParams{}, errors.Trace(err)
		}
		var location string
		var readOnly bool
		if filesystemAttachmentParams, ok := filesystemAttachment.Params(); ok {
			location = filesystemAttachmentParams.Location
			readOnly = filesystemAttachmentParams.ReadOnly
		} else {
			// Attachment parameters may be requested even if the
			// attachment exists; i.e. for reattachment.
			filesystemAttachmentInfo, err := filesystemAttachment.Info()
			if err != nil {
				return params.FilesystemAttachmentParams{}, errors.Trace(err)
			}
			location = filesystemAttachmentInfo.MountPoint
			readOnly = filesystemAttachmentInfo.ReadOnly
		}
		return params.FilesystemAttachmentParams{
			filesystemAttachment.Filesystem().String(),
			filesystemAttachment.Machine().String(),
			filesystemId,
			string(instanceId),
			string(providerType),
			// TODO(axw) dealias MountPoint. We now have
			// Path, MountPoint and Location in different
			// parts of the codebase.
			location,
			readOnly,
		}, nil
	}
	for i, arg := range args.Ids {
		var result params.FilesystemAttachmentParamsResult
		filesystemAttachment, err := one(arg)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.Result = filesystemAttachment
		}
		results.Results[i] = result
	}
	return results, nil
}

func (s *StorageProvisionerAPI) oneVolumeAttachment(
	id params.MachineStorageId, canAccess func(names.MachineTag, names.Tag) bool,
) (state.VolumeAttachment, error) {
	machineTag, err := names.ParseMachineTag(id.MachineTag)
	if err != nil {
		return nil, err
	}
	volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
	if err != nil {
		return nil, err
	}
	if !canAccess(machineTag, volumeTag) {
		return nil, common.ErrPerm
	}
	volumeAttachment, err := s.st.VolumeAttachment(machineTag, volumeTag)
	if errors.IsNotFound(err) {
		return nil, common.ErrPerm
	} else if err != nil {
		return nil, err
	}
	return volumeAttachment, nil
}

func (s *StorageProvisionerAPI) oneVolumeBlockDevice(
	id params.MachineStorageId, canAccess func(names.MachineTag, names.Tag) bool,
) (state.BlockDeviceInfo, error) {
	volumeAttachment, err := s.oneVolumeAttachment(id, canAccess)
	if err != nil {
		return state.BlockDeviceInfo{}, err
	}
	volume, err := s.st.Volume(volumeAttachment.Volume())
	if err != nil {
		return state.BlockDeviceInfo{}, err
	}
	volumeInfo, err := volume.Info()
	if err != nil {
		return state.BlockDeviceInfo{}, err
	}
	volumeAttachmentInfo, err := volumeAttachment.Info()
	if err != nil {
		return state.BlockDeviceInfo{}, err
	}
	blockDevices, err := s.st.BlockDevices(volumeAttachment.Machine())
	if err != nil {
		return state.BlockDeviceInfo{}, err
	}
	blockDevice, ok := common.MatchingBlockDevice(
		blockDevices,
		volumeInfo,
		volumeAttachmentInfo,
	)
	if !ok {
		return state.BlockDeviceInfo{}, errors.NotFoundf(
			"block device for volume %v on machine %v",
			volumeAttachment.Volume().Id(),
			volumeAttachment.Machine().Id(),
		)
	}
	return *blockDevice, nil
}

func (s *StorageProvisionerAPI) oneFilesystemAttachment(
	id params.MachineStorageId, canAccess func(names.MachineTag, names.Tag) bool,
) (state.FilesystemAttachment, error) {
	machineTag, err := names.ParseMachineTag(id.MachineTag)
	if err != nil {
		return nil, err
	}
	filesystemTag, err := names.ParseFilesystemTag(id.AttachmentTag)
	if err != nil {
		return nil, err
	}
	if !canAccess(machineTag, filesystemTag) {
		return nil, common.ErrPerm
	}
	filesystemAttachment, err := s.st.FilesystemAttachment(machineTag, filesystemTag)
	if errors.IsNotFound(err) {
		return nil, common.ErrPerm
	} else if err != nil {
		return nil, err
	}
	return filesystemAttachment, nil
}

// SetVolumeInfo records the details of newly provisioned volumes.
func (s *StorageProvisionerAPI) SetVolumeInfo(args params.Volumes) (params.ErrorResults, error) {
	canAccessVolume, err := s.getStorageEntityAuthFunc()
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Volumes)),
	}
	one := func(arg params.Volume) error {
		volumeTag, volumeInfo, err := common.VolumeToState(arg)
		if err != nil {
			return errors.Trace(err)
		} else if !canAccessVolume(volumeTag) {
			return common.ErrPerm
		}
		err = s.st.SetVolumeInfo(volumeTag, volumeInfo)
		if errors.IsNotFound(err) {
			return common.ErrPerm
		}
		return errors.Trace(err)
	}
	for i, arg := range args.Volumes {
		err := one(arg)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// SetFilesystemInfo records the details of newly provisioned filesystems.
func (s *StorageProvisionerAPI) SetFilesystemInfo(args params.Filesystems) (params.ErrorResults, error) {
	canAccessFilesystem, err := s.getStorageEntityAuthFunc()
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Filesystems)),
	}
	one := func(arg params.Filesystem) error {
		filesystemTag, filesystemInfo, err := common.FilesystemToState(arg)
		if err != nil {
			return errors.Trace(err)
		} else if !canAccessFilesystem(filesystemTag) {
			return common.ErrPerm
		}
		err = s.st.SetFilesystemInfo(filesystemTag, filesystemInfo)
		if errors.IsNotFound(err) {
			return common.ErrPerm
		}
		return errors.Trace(err)
	}
	for i, arg := range args.Filesystems {
		err := one(arg)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// SetVolumeAttachmentInfo records the details of newly provisioned volume
// attachments.
func (s *StorageProvisionerAPI) SetVolumeAttachmentInfo(
	args params.VolumeAttachments,
) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc()
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.VolumeAttachments)),
	}
	one := func(arg params.VolumeAttachment) error {
		machineTag, volumeTag, volumeAttachmentInfo, err := common.VolumeAttachmentToState(arg)
		if err != nil {
			return errors.Trace(err)
		}
		if !canAccess(machineTag, volumeTag) {
			return common.ErrPerm
		}
		err = s.st.SetVolumeAttachmentInfo(machineTag, volumeTag, volumeAttachmentInfo)
		if errors.IsNotFound(err) {
			return common.ErrPerm
		}
		return errors.Trace(err)
	}
	for i, arg := range args.VolumeAttachments {
		err := one(arg)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// SetFilesystemAttachmentInfo records the details of newly provisioned filesystem
// attachments.
func (s *StorageProvisionerAPI) SetFilesystemAttachmentInfo(
	args params.FilesystemAttachments,
) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc()
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.FilesystemAttachments)),
	}
	one := func(arg params.FilesystemAttachment) error {
		machineTag, filesystemTag, filesystemAttachmentInfo, err := common.FilesystemAttachmentToState(arg)
		if err != nil {
			return errors.Trace(err)
		}
		if !canAccess(machineTag, filesystemTag) {
			return common.ErrPerm
		}
		err = s.st.SetFilesystemAttachmentInfo(machineTag, filesystemTag, filesystemAttachmentInfo)
		if errors.IsNotFound(err) {
			return common.ErrPerm
		}
		return errors.Trace(err)
	}
	for i, arg := range args.FilesystemAttachments {
		err := one(arg)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// AttachmentLife returns the lifecycle state of each specified machine
// storage attachment.
func (s *StorageProvisionerAPI) AttachmentLife(args params.MachineStorageIds) (params.LifeResults, error) {
	canAccess, err := s.getAttachmentAuthFunc()
	if err != nil {
		return params.LifeResults{}, err
	}
	results := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (params.Life, error) {
		machineTag, err := names.ParseMachineTag(arg.MachineTag)
		if err != nil {
			return "", err
		}
		attachmentTag, err := names.ParseTag(arg.AttachmentTag)
		if err != nil {
			return "", err
		}
		if !canAccess(machineTag, attachmentTag) {
			return "", common.ErrPerm
		}
		var lifer state.Lifer
		switch attachmentTag := attachmentTag.(type) {
		case names.VolumeTag:
			lifer, err = s.st.VolumeAttachment(machineTag, attachmentTag)
		case names.FilesystemTag:
			lifer, err = s.st.FilesystemAttachment(machineTag, attachmentTag)
		}
		if errors.IsNotFound(err) {
			return "", common.ErrPerm
		} else if err != nil {
			return "", errors.Trace(err)
		}
		return params.Life(lifer.Life().String()), nil
	}
	for i, arg := range args.Ids {
		life, err := one(arg)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		} else {
			results.Results[i].Life = life
		}
	}
	return results, nil
}

// Remove removes volumes and filesystems from state.
func (s *StorageProvisionerAPI) Remove(args params.Entities) (params.ErrorResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc()
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	one := func(arg params.Entity) error {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		if !canAccess(tag) {
			return common.ErrPerm
		}
		switch tag := tag.(type) {
		case names.FilesystemTag:
			return s.st.RemoveFilesystem(tag)
		case names.VolumeTag:
			return s.st.RemoveVolume(tag)
		default:
			// should have been picked up by canAccess
			logger.Debugf("unexpected %v tag", tag.Kind())
			return common.ErrPerm
		}
	}
	for i, arg := range args.Entities {
		err := one(arg)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// RemoveAttachments removes the specified machine storage attachments
// from state.
func (s *StorageProvisionerAPI) RemoveAttachment(args params.MachineStorageIds) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc()
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Ids)),
	}
	removeAttachment := func(arg params.MachineStorageId) error {
		machineTag, err := names.ParseMachineTag(arg.MachineTag)
		if err != nil {
			return err
		}
		attachmentTag, err := names.ParseTag(arg.AttachmentTag)
		if err != nil {
			return err
		}
		if !canAccess(machineTag, attachmentTag) {
			return common.ErrPerm
		}
		switch attachmentTag := attachmentTag.(type) {
		case names.VolumeTag:
			return s.st.RemoveVolumeAttachment(machineTag, attachmentTag)
		case names.FilesystemTag:
			return s.st.RemoveFilesystemAttachment(machineTag, attachmentTag)
		default:
			return common.ErrPerm
		}
	}
	for i, arg := range args.Ids {
		if err := removeAttachment(arg); err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}
