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

	st                    provisionerState
	settings              poolmanager.SettingsManager
	resources             *common.Resources
	authorizer            common.Authorizer
	getScopeAuthFunc      common.GetAuthFunc
	getVolumeAuthFunc     common.GetAuthFunc
	getAttachmentAuthFunc func() (func(names.MachineTag, names.Tag) bool, error)
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
	canAccessVolumeMachine := func(tag names.MachineTag, allowEnvironManager bool) bool {
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
				// scoped to the environment.
				//
				// TODO(axw) allow watching volumes in alternative
				// environments? Need to check with thumper.
				isEnvironManager := authorizer.AuthEnvironManager()
				return isEnvironManager && tag == st.EnvironTag()
			case names.MachineTag:
				return canAccessVolumeMachine(tag, false)
			default:
				return false
			}
		}, nil
	}
	getVolumeAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			switch tag := tag.(type) {
			case names.VolumeTag:
				machineTag, ok := names.VolumeMachine(tag)
				if ok {
					return canAccessVolumeMachine(machineTag, false)
				}
				return authorizer.AuthEnvironManager()
			default:
				return false
			}
		}, nil
	}
	getAttachmentAuthFunc := func() (func(names.MachineTag, names.Tag) bool, error) {
		// getAttachmentAuthFunc returns a function that validates
		// access by the authenticated user to an attachment.
		return func(machineTag names.MachineTag, attachmentTag names.Tag) bool {
			// Machine agents can access their own machine, and
			// machines contained. Environment managers can access
			// top-level machines.
			if !canAccessVolumeMachine(machineTag, true) {
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
	stateInterface := getState(st)
	settings := getSettingsManager(st)
	return &StorageProvisionerAPI{
		LifeGetter:            common.NewLifeGetter(stateInterface, getVolumeAuthFunc),
		DeadEnsurer:           common.NewDeadEnsurer(stateInterface, getVolumeAuthFunc),
		st:                    stateInterface,
		settings:              settings,
		resources:             resources,
		authorizer:            authorizer,
		getScopeAuthFunc:      getScopeAuthFunc,
		getVolumeAuthFunc:     getVolumeAuthFunc,
		getAttachmentAuthFunc: getAttachmentAuthFunc,
	}, nil
}

// WatchVolumes watches for changes to volumes scoped to the
// entity with the tag passed to NewState.
func (s *StorageProvisionerAPI) WatchVolumes(args params.Entities) (params.StringsWatchResults, error) {
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
			w = s.st.WatchMachineVolumes(tag)
		} else {
			w = s.st.WatchEnvironVolumes()
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
			w = s.st.WatchMachineVolumeAttachments(tag)
		} else {
			w = s.st.WatchEnvironVolumeAttachments()
		}
		if stringChanges, ok := <-w.Changes(); ok {
			changes, err := common.ParseVolumeAttachmentIds(stringChanges)
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
	canAccess, err := s.getVolumeAuthFunc()
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

// VolumeParams returns the parameters for creating the volumes
// with the specified tags.
func (s *StorageProvisionerAPI) VolumeParams(args params.Entities) (params.VolumeParamsResults, error) {
	canAccess, err := s.getVolumeAuthFunc()
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
		volumeParams, err := common.VolumeParams(volume, poolManager)
		if err != nil {
			return params.VolumeParams{}, err
		}
		if len(volumeAttachments) == 1 {
			machineTag := volumeAttachments[0].Machine()
			instanceId, err := s.st.MachineInstanceId(machineTag)
			if errors.IsNotProvisioned(err) {
				// Leave the attachment until later.
			} else if err != nil {
				return params.VolumeParams{}, err
			} else {
				volumeParams.Attachment = &params.VolumeAttachmentParams{
					MachineTag: volumeAttachments[0].Machine().String(),
					VolumeTag:  tag.String(),
					InstanceId: string(instanceId),
					Provider:   volumeParams.Provider,
					// TODO(axw) other attachment params (e.g. ReadOnly)
				}
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
		if err != nil {
			// Can't attach a volume to a machine that
			// hasn't been provisioned yet.
			return params.VolumeAttachmentParams{}, err
		}
		volume, err := s.st.Volume(volumeAttachment.Volume())
		if err != nil && !errors.IsNotProvisioned(err) {
			return params.VolumeAttachmentParams{}, err
		}
		volumeInfo, err := volume.Info()
		if err != nil {
			// Can't attach a volume that hasn't been
			// provisioned yet.
			return params.VolumeAttachmentParams{}, err
		}
		providerType, _, err := common.StoragePoolConfig(volumeInfo.Pool, poolManager)
		if err != nil {
			return params.VolumeAttachmentParams{}, errors.Trace(err)
		}
		return params.VolumeAttachmentParams{
			volumeAttachment.Volume().String(),
			volumeAttachment.Machine().String(),
			string(instanceId),
			volumeInfo.VolumeId,
			string(providerType),
			// TODO(axw) other attachment params (e.g. ReadOnly)
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

func (s *StorageProvisionerAPI) oneVolumeAttachment(
	id params.MachineStorageId, canAccessMachine func(names.MachineTag, names.Tag) bool,
) (state.VolumeAttachment, error) {
	machineTag, err := names.ParseMachineTag(id.MachineTag)
	if err != nil {
		return nil, err
	}
	volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
	if err != nil {
		return nil, err
	}
	if !canAccessMachine(machineTag, volumeTag) {
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

// SetVolumeInfo records the details of newly provisioned volumes.
func (s *StorageProvisionerAPI) SetVolumeInfo(args params.Volumes) (params.ErrorResults, error) {
	canAccessVolume, err := s.getVolumeAuthFunc()
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
